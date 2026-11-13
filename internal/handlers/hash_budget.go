package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
)

var errHashCapacity = errors.New("hash work capacity unavailable")

// A shared, non-queueing budget for creation and both verification routes.
// Slots cover hashing and required prechecks, never provider network delivery.
type hashBudget struct{ slots chan struct{} }

func newHashBudget(limit int) (*hashBudget, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("HERALD_MAX_CONCURRENT_HASH_OPERATIONS must be positive")
	}
	return &hashBudget{slots: make(chan struct{}, limit)}, nil
}

func (b *hashBudget) acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errHashCapacity
	}
	select {
	case b.slots <- struct{}{}:
		return func() { <-b.slots }, nil
	default:
		return nil, errHashCapacity
	}
}

// Admission precedes rate/cooldown mutation so an overloaded request can retry
// after capacity returns. Completed idempotency responses bypass this stage.
// Order: hash budget, user lock (read-only), then limits and cooldowns as one
// all-or-nothing step, then challenge creation.
func (h *Handlers) prepareChallenge(c fiber.Ctx, ctx context.Context, req CreateChallengeRequest, peerIP string, failIdem func()) (*challengekit.Challenge, string, bool, error) {
	release, err := h.hashBudget.acquire(ctx)
	if err != nil {
		failIdem()
		c.Set("Retry-After", "1")
		return nil, "", true, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
	}
	defer release()
	// The lock check is read-only and runs first: a locked user is refused
	// without spending any quota or starting a cooldown.
	locked, lockErr := h.userLocked(ctx, req.UserID)
	if lockErr != nil {
		// A lock that cannot be read is an outage, not a locked user.
		failIdem()
		return nil, "", true, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
	}
	if locked {
		failIdem()
		return nil, "", true, c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ok": false, "reason": "user_locked"})
	}
	if allowed, err := h.checkCreationLimits(c, ctx, req, peerIP, failIdem); !allowed {
		return nil, "", true, err
	}
	ch, code, err := h.challengeManager.Create(ctx, challengekit.CreateRequest{
		UserID: req.UserID, Channel: challengekit.Channel(req.Channel), Destination: req.Destination, Purpose: req.Purpose, ClientIP: peerIP,
	})
	return ch, code, false, err
}

func (h *Handlers) verifyWithBudget(ctx context.Context, id, code, peerIP string, opts *challengekit.VerifyOptions) (*challengekit.VerifyResult, error) {
	release, err := h.hashBudget.acquire(ctx)
	if err != nil {
		return &challengekit.VerifyResult{OK: false, Reason: challengekit.ReasonBackendUnavailable}, err
	}
	defer release()
	if opts == nil {
		return h.challengeManager.Verify(ctx, id, code, peerIP)
	}
	return h.challengeManager.VerifyWithOptions(ctx, id, code, peerIP, *opts)
}

// userLocked reports whether the user is locked, with the backend error when
// the manager can provide it. A manager that cannot is trusted as is: its
// IsUserLocked already answers true on an outage.
func (h *Handlers) userLocked(ctx context.Context, userID string) (bool, error) {
	if checker, ok := h.challengeManager.(challengekit.UserLockChecker); ok {
		return checker.IsUserLockedE(ctx, userID)
	}
	return h.challengeManager.IsUserLocked(ctx, userID), nil
}
