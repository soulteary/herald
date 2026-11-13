package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

const lockTestUser = "lock-user@tenant.example"

type userLockFixture struct {
	*creationLimitFixture
	rc       *redis.Client
	sequence int
}

// newUserLockFixture builds create and verify endpoints whose handlers lock a
// user after `threshold` exhausted challenges.
func newUserLockFixture(t *testing.T, threshold int) *userLockFixture {
	t.Helper()
	oldThreshold, oldLockout, oldPepper := config.LockoutChallengeThreshold, config.LockoutDuration, config.PIIPepper
	t.Cleanup(func() {
		config.LockoutChallengeThreshold, config.LockoutDuration, config.PIIPepper = oldThreshold, oldLockout, oldPepper
	})
	config.LockoutChallengeThreshold, config.LockoutDuration, config.PIIPepper = threshold, 10*time.Minute, "user-lock-test-pepper"
	f := &userLockFixture{creationLimitFixture: newCreationLimitFixture(t)}
	f.rc = f.h.redis
	f.app.Post("/v1/otp/verifications", f.h.VerifyChallenge)
	f.app.Post("/v2/otp/verifications", f.h.VerifyChallengeV2)
	return f
}

// newChallenge stores an active challenge for the user without going through
// the create endpoint, so the test knows its code. Each call uses a new
// destination: the lock is per user, across destinations.
func (f *userLockFixture) newChallenge(t *testing.T, userID string) (*challengekit.Challenge, string) {
	t.Helper()
	f.sequence++
	ch, code, err := f.h.challengeManager.Create(context.Background(), challengekit.CreateRequest{
		UserID: userID, Channel: challengekit.ChannelEmail, Purpose: "login",
		Destination: fmt.Sprintf("inbox-%d@example.com", f.sequence),
	})
	require.NoError(t, err)
	_, err = f.h.challengeManager.SwapActive(context.Background(), ch)
	require.NoError(t, err)
	return ch, code
}

func wrongCode(code string) string {
	if code == "000000" {
		return "111111"
	}
	return "000000"
}

func (f *userLockFixture) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.app.Test(req, fiber.TestConfig{Timeout: 30 * time.Second})
	require.NoError(t, err)
	raw, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded), string(raw))
	return resp.StatusCode, decoded
}

func (f *userLockFixture) verify(t *testing.T, challengeID, code string) (int, map[string]any) {
	t.Helper()
	return f.post(t, "/v1/otp/verifications", fiber.Map{"challenge_id": challengeID, "code": code})
}

// exhaust submits MAX_ATTEMPTS wrong codes for a new challenge and checks that
// the last one reports the challenge, not the user, as locked.
func (f *userLockFixture) exhaust(t *testing.T, userID string) *challengekit.Challenge {
	t.Helper()
	ch, code := f.newChallenge(t, userID)
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		status, body := f.verify(t, ch.ID, wrongCode(code))
		require.Equal(t, fiber.StatusUnauthorized, status, body)
		if attempt < config.MaxAttempts {
			require.Equal(t, "invalid", body["reason"], "attempt %d", attempt)
		} else {
			require.Equal(t, "locked", body["reason"])
		}
	}
	return ch
}

func (f *userLockFixture) lockKey(userID string) string {
	return userLockKeyPrefix + f.h.digester.DigestParts("user-lock", userID)
}

func (f *userLockFixture) exhaustedKey(userID string) string {
	return userLockCounterPrefix + f.h.digester.DigestParts("user-lock", userID)
}

func (f *userLockFixture) exhaustedCount(t *testing.T, userID string) int {
	t.Helper()
	if !f.mr.Exists(f.exhaustedKey(userID)) {
		return 0
	}
	members, err := f.mr.SMembers(f.exhaustedKey(userID))
	require.NoError(t, err)
	return len(members)
}

func (f *userLockFixture) requireNoRawUserIDInKeys(t *testing.T, userID string) {
	t.Helper()
	for _, key := range f.mr.Keys() {
		require.NotContains(t, key, userID, "Redis key contains the raw user ID")
	}
}

func TestUserLock_OneExhaustedChallengeDoesNotLockUser(t *testing.T) {
	f := newUserLockFixture(t, 3)
	exhausted := f.exhaust(t, lockTestUser)

	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)), "one exhausted challenge must not lock the user")
	require.Equal(t, 1, f.exhaustedCount(t, lockTestUser))
	f.requireNoRawUserIDInKeys(t, lockTestUser)

	// The exhausted challenge stays unusable.
	status, body := f.verify(t, exhausted.ID, "123456")
	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, "locked", body["reason"])

	// A new challenge can be created through the endpoint ...
	resp := f.create(t, lockTestUser, "fresh@example.com")
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))

	// ... and one can be verified, on both routes, with the stored user ID.
	ch, code := f.newChallenge(t, lockTestUser)
	status, body = f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusOK, status, body)
	require.Equal(t, lockTestUser, body["user_id"])

	ch, code = f.newChallenge(t, lockTestUser)
	status, body = f.post(t, "/v2/otp/verifications", fiber.Map{
		"challenge_id": ch.ID, "code": code,
		"expected_user_id": lockTestUser, "expected_purpose": "login", "expected_channel": "email",
	})
	require.Equal(t, fiber.StatusOK, status, body)
	require.Equal(t, lockTestUser, body["user_id"])
}

func TestUserLock_ThresholdExhaustedChallengesLockUserForLockoutDuration(t *testing.T) {
	f := newUserLockFixture(t, 3)
	f.exhaust(t, lockTestUser)
	f.exhaust(t, lockTestUser)
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)), "two exhausted challenges must not lock the user")
	require.Equal(t, 2, f.exhaustedCount(t, lockTestUser))

	f.exhaust(t, lockTestUser)
	require.True(t, f.mr.Exists(f.lockKey(lockTestUser)), "the third exhausted challenge must lock the user")
	require.Equal(t, config.LockoutDuration, f.mr.TTL(f.lockKey(lockTestUser)))
	require.False(t, f.mr.Exists(f.exhaustedKey(lockTestUser)), "the counter is consumed by the lock")
	f.requireNoRawUserIDInKeys(t, lockTestUser)

	// Locked: no new challenge, and no verification even with the right code.
	resp := f.create(t, lockTestUser, "locked@example.com")
	require.Equal(t, fiber.StatusForbidden, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"user_locked"`)

	ch, code := f.newChallenge(t, lockTestUser)
	status, body := f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, "user_locked", body["reason"])
	status, body = f.post(t, "/v2/otp/verifications", fiber.Map{
		"challenge_id": ch.ID, "code": code,
		"expected_user_id": lockTestUser, "expected_purpose": "login", "expected_channel": "email",
	})
	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, "user_locked", body["reason"])

	// Another user is not affected.
	other, otherCode := f.newChallenge(t, "someone-else")
	status, body = f.verify(t, other.ID, otherCode)
	require.Equal(t, fiber.StatusOK, status, body)

	// The lock ends after LOCKOUT_DURATION.
	f.mr.FastForward(config.LockoutDuration - time.Second)
	require.True(t, f.mr.Exists(f.lockKey(lockTestUser)))
	f.mr.FastForward(2 * time.Second)
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)))
	resp = f.create(t, lockTestUser, "unlocked@example.com")
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))
	ch, code = f.newChallenge(t, lockTestUser)
	status, body = f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusOK, status, body)
}

func TestUserLock_ExhaustedCounterExpiresWithItsWindow(t *testing.T) {
	f := newUserLockFixture(t, 3)
	f.exhaust(t, lockTestUser)
	require.Equal(t, config.LockoutDuration, f.mr.TTL(f.exhaustedKey(lockTestUser)))

	// The window starts at the first exhausted challenge and is not extended
	// by later ones.
	f.mr.FastForward(6 * time.Minute)
	f.exhaust(t, lockTestUser)
	require.Equal(t, 2, f.exhaustedCount(t, lockTestUser))
	require.Equal(t, config.LockoutDuration-6*time.Minute, f.mr.TTL(f.exhaustedKey(lockTestUser)))

	f.mr.FastForward(4*time.Minute + time.Second)
	require.False(t, f.mr.Exists(f.exhaustedKey(lockTestUser)), "the counter must expire")

	// Two more exhausted challenges in a new window still do not lock.
	f.exhaust(t, lockTestUser)
	f.exhaust(t, lockTestUser)
	require.Equal(t, 2, f.exhaustedCount(t, lockTestUser))
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)))
}

func TestUserLock_ThresholdOneLocksOnFirstExhaustedChallenge(t *testing.T) {
	f := newUserLockFixture(t, 1)
	f.exhaust(t, lockTestUser)

	require.True(t, f.mr.Exists(f.lockKey(lockTestUser)))
	require.Equal(t, config.LockoutDuration, f.mr.TTL(f.lockKey(lockTestUser)))
	require.False(t, f.mr.Exists(f.exhaustedKey(lockTestUser)))
	f.requireNoRawUserIDInKeys(t, lockTestUser)

	resp := f.create(t, lockTestUser, "locked@example.com")
	require.Equal(t, fiber.StatusForbidden, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"user_locked"`)
	ch, code := f.newChallenge(t, lockTestUser)
	status, body := f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, "user_locked", body["reason"])
}

// A successful verification leaves the counter as it is: a success on one
// challenge says nothing about who is guessing the others.
func TestUserLock_SuccessfulVerificationDoesNotResetCounter(t *testing.T) {
	f := newUserLockFixture(t, 3)
	f.exhaust(t, lockTestUser)
	f.exhaust(t, lockTestUser)

	ch, code := f.newChallenge(t, lockTestUser)
	status, body := f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusOK, status, body)
	require.Equal(t, 2, f.exhaustedCount(t, lockTestUser))

	f.exhaust(t, lockTestUser)
	require.True(t, f.mr.Exists(f.lockKey(lockTestUser)))
}

// Probing a challenge that is already exhausted must not count it again.
func TestUserLock_ExhaustedChallengeIsCountedOnce(t *testing.T) {
	f := newUserLockFixture(t, 3)
	ch := f.exhaust(t, lockTestUser)
	for i := 0; i < 4; i++ {
		status, body := f.verify(t, ch.ID, "123456")
		require.Equal(t, fiber.StatusUnauthorized, status)
		require.Equal(t, "locked", body["reason"])
	}
	require.Equal(t, 1, f.exhaustedCount(t, lockTestUser))
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)))
}

// failEval rejects EVAL while armed; every other command reaches Redis.
type failEval struct{ armed atomic.Bool }

func (h *failEval) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *failEval) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *failEval) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.armed.Load() && strings.EqualFold(cmd.Name(), "eval") && strings.Contains(strings.ToLower(fmt.Sprint(cmd.Args()...)), "scard") {
			err := errors.New("injected lock write failure")
			cmd.SetErr(err)
			return err
		}
		return next(ctx, cmd)
	}
}

// When the exhausted challenge cannot be recorded the request fails closed,
// and the next request for that challenge records it exactly once.
func TestUserLock_FailedLockWriteFailsClosedAndIsRetried(t *testing.T) {
	// A threshold above one makes the kit record the exhausted challenge.
	f := newUserLockFixture(t, 2)
	hook := &failEval{}
	f.rc.AddHook(hook)

	ch, code := f.newChallenge(t, lockTestUser)
	for attempt := 1; attempt < config.MaxAttempts; attempt++ {
		status, _ := f.verify(t, ch.ID, wrongCode(code))
		require.Equal(t, fiber.StatusUnauthorized, status)
	}
	hook.armed.Store(true)
	status, body := f.verify(t, ch.ID, wrongCode(code))
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"])
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)))

	// Still failing: the right code is not accepted for the exhausted challenge.
	status, body = f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)

	hook.armed.Store(false)
	status, body = f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status)
	require.Equal(t, "locked", body["reason"])
	require.Equal(t, 1, f.exhaustedCount(t, lockTestUser), "the exhausted challenge is recorded on the retry")
	require.False(t, f.mr.Exists(f.lockKey(lockTestUser)), "one exhausted challenge is below the threshold")
}

func TestUserLock_RedisOutageFailsClosed(t *testing.T) {
	f := newUserLockFixture(t, 3)
	ch, code := f.newChallenge(t, lockTestUser)
	f.mr.Close()

	resp := f.create(t, lockTestUser, "outage@example.com")
	require.Equal(t, fiber.StatusServiceUnavailable, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"backend_unavailable"`)
	require.NotContains(t, string(resp.body), "user_locked")

	status, body := f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"])
	status, body = f.post(t, "/v2/otp/verifications", fiber.Map{
		"challenge_id": ch.ID, "code": code,
		"expected_user_id": lockTestUser, "expected_purpose": "login", "expected_channel": "email",
	})
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"])
}

// Parallel wrong guesses consume at most MAX_ATTEMPTS per challenge, and the
// lock set by them is not bypassed by a correct code on another challenge.
func TestUserLock_ConcurrentWrongGuesses(t *testing.T) {
	const threshold, workers = 2, 6
	f := newUserLockFixture(t, threshold)
	ctx := context.Background()

	type target struct {
		ch       *challengekit.Challenge
		code     string
		consumed atomic.Int64
	}
	targets := make([]*target, threshold)
	for i := range targets {
		ch, code := f.newChallenge(t, lockTestUser)
		targets[i] = &target{ch: ch, code: code}
	}
	spare, spareCode := f.newChallenge(t, lockTestUser)

	var wg sync.WaitGroup
	var accepted atomic.Int64
	for _, tg := range targets {
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func(tg *target) {
				defer wg.Done()
				for try := 0; try < 200; try++ {
					result, _ := f.h.challengeManager.Verify(ctx, tg.ch.ID, wrongCode(tg.code), "192.0.2.1")
					switch {
					case result.OK:
						accepted.Add(1)
						return
					case result.RemainingAttempts != nil:
						// A wrong code was compared and counted.
						tg.consumed.Add(1)
					case result.Reason == challengekit.ReasonLocked, result.Reason == challengekit.ReasonUserLocked, result.Reason == challengekit.ReasonExpired:
						return
					}
				}
			}(tg)
		}
	}
	wg.Wait()

	require.Zero(t, accepted.Load())
	exhausted := 0
	for _, tg := range targets {
		require.LessOrEqual(t, tg.consumed.Load(), int64(config.MaxAttempts), "more than MAX_ATTEMPTS guesses were compared")
		if tg.consumed.Load() == int64(config.MaxAttempts) {
			exhausted++
		}
	}
	// The lock can land while the last challenge still has attempts left; it
	// is set as soon as `threshold` challenges are exhausted, never later.
	require.Equal(t, exhausted >= threshold, f.mr.Exists(f.lockKey(lockTestUser)))
	require.True(t, f.mr.Exists(f.lockKey(lockTestUser)), "exhausting the threshold number of challenges must lock the user")

	// Parallel correct codes on an untouched challenge are all refused.
	var refused atomic.Int64
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for try := 0; try < 200; try++ {
				result, _ := f.h.challengeManager.Verify(ctx, spare.ID, spareCode, "192.0.2.1")
				if result.OK {
					accepted.Add(1)
					return
				}
				if result.Reason == challengekit.ReasonUserLocked {
					refused.Add(1)
					return
				}
			}
		}()
	}
	wg.Wait()
	require.Zero(t, accepted.Load(), "a locked user's challenge was verified")
	require.EqualValues(t, workers, refused.Load())
}

// The raw user ID never appears in a lock key, and the key of v1.6.0
// ("otp:lock:<user_id>") is not written.
func TestUserLock_LockKeyCarriesDigestNotUserID(t *testing.T) {
	f := newUserLockFixture(t, 1)
	f.exhaust(t, lockTestUser)
	require.False(t, f.mr.Exists("otp:lock:"+lockTestUser))
	lockKeys := 0
	for _, key := range f.mr.Keys() {
		if strings.HasPrefix(key, "otp:lock:") {
			lockKeys++
			require.Equal(t, f.lockKey(lockTestUser), key)
		}
	}
	require.Equal(t, 1, lockKeys)
	require.True(t, f.h.challengeManager.IsUserLocked(context.Background(), lockTestUser))
	require.False(t, f.h.challengeManager.IsUserLocked(context.Background(), "someone-else"))
}
