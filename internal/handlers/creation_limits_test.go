package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/destination"
)

type creationLimitFixture struct {
	app      *fiber.App
	h        *Handlers
	mr       *miniredis.Miniredis
	provider *countingProvider
}

// newCreationLimitFixture builds a create endpoint on a miniredis instance the
// test can inspect. Settings changed after this call are restored on cleanup.
func newCreationLimitFixture(t *testing.T) *creationLimitFixture {
	t.Helper()
	oldUser, oldIP, oldDest, oldCooldown := config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination, config.ResendCooldown
	oldCodeLength, oldAttempts, oldExpiry := config.CodeLength, config.MaxAttempts, config.ChallengeExpiry
	oldPurposes, oldPolicy, oldSecret := config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret
	t.Cleanup(func() {
		config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination, config.ResendCooldown = oldUser, oldIP, oldDest, oldCooldown
		config.CodeLength, config.MaxAttempts, config.ChallengeExpiry = oldCodeLength, oldAttempts, oldExpiry
		config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret = oldPurposes, oldPolicy, oldSecret
	})
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	p := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return &creationLimitFixture{app: app, h: h, mr: mr, provider: p}
}

func emailChallengeBody(userID, dest string) string {
	return fmt.Sprintf(`{"user_id":%q,"channel":"email","destination":%q,"purpose":"login"}`, userID, dest)
}

func (f *creationLimitFixture) create(t *testing.T, userID, dest string) *fiberTestResp {
	t.Helper()
	resp, err := idemRequest(f.app, "", emailChallengeBody(userID, dest))
	require.NoError(t, err)
	return resp
}

func (f *creationLimitFixture) userKey(userID string) string {
	return "ratelimit:user:" + f.h.digester.Digest(userID)
}

func (f *creationLimitFixture) destKey(dest string) string {
	return "ratelimit:dest:" + f.h.digester.Digest(destination.LimitKey("email", dest))
}

func (f *creationLimitFixture) userCooldownKey(userID, dest string) string {
	return "ratelimit:cooldown:" + f.h.digester.DigestParts(userID, destination.LimitKey("email", dest))
}

func (f *creationLimitFixture) destCooldownKey(dest string) string {
	return "ratelimit:cooldown:dest:" + f.h.digester.Digest(destination.LimitKey("email", dest))
}

// limiterState returns every limiter and cooldown key with its value.
func (f *creationLimitFixture) limiterState(t *testing.T) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, key := range f.mr.Keys() {
		if strings.HasPrefix(key, "ratelimit:") {
			value, err := f.mr.Get(key)
			require.NoError(t, err)
			state[key] = value
		}
	}
	return state
}

// ipCounter returns the single per-IP counter; every test request shares one
// peer address.
func (f *creationLimitFixture) ipCounter(t *testing.T) string {
	t.Helper()
	value := ""
	for key, v := range f.limiterState(t) {
		if strings.HasPrefix(key, "ratelimit:ip:") {
			require.Empty(t, value, "more than one IP counter")
			value = v
		}
	}
	return value
}

func requireRateLimited(t *testing.T, resp *fiberTestResp, reason string, max int) {
	t.Helper()
	require.Equal(t, fiber.StatusTooManyRequests, resp.status, string(resp.body))
	var body struct {
		OK           bool   `json:"ok"`
		Reason       string `json:"reason"`
		NextResendIn int    `json:"next_resend_in"`
	}
	require.NoError(t, json.Unmarshal(resp.body, &body))
	require.False(t, body.OK)
	require.Equal(t, reason, body.Reason)
	require.Greater(t, body.NextResendIn, 0)
	require.LessOrEqual(t, body.NextResendIn, max)
}

func TestCreateChallenge_HappyPathConsumesEachGateOnce(t *testing.T) {
	f := newCreationLimitFixture(t)
	resp := f.create(t, "alice", "alice@example.com")
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))

	state := f.limiterState(t)
	require.Len(t, state, 5, "three counters and two cooldown keys: %v", state)
	require.Equal(t, "1", state[f.userKey("alice")])
	require.Equal(t, "1", state[f.destKey("alice@example.com")])
	require.Equal(t, "1", f.ipCounter(t))
	require.Contains(t, state, f.userCooldownKey("alice", "alice@example.com"))
	require.Contains(t, state, f.destCooldownKey("alice@example.com"))

	require.Equal(t, time.Hour, f.mr.TTL(f.userKey("alice")))
	require.Equal(t, time.Hour, f.mr.TTL(f.destKey("alice@example.com")))
	require.Equal(t, config.ResendCooldown, f.mr.TTL(f.userCooldownKey("alice", "alice@example.com")))
	require.Equal(t, config.ResendCooldown, f.mr.TTL(f.destCooldownKey("alice@example.com")))
	require.EqualValues(t, 1, atomic.LoadInt64(&f.provider.calls))
}

func TestCreateChallenge_CooldownDenialSpendsNoQuota(t *testing.T) {
	f := newCreationLimitFixture(t)
	require.Equal(t, fiber.StatusOK, f.create(t, "alice", "alice@example.com").status)
	before := f.limiterState(t)

	for i := 0; i < 3; i++ {
		requireRateLimited(t, f.create(t, "alice", "alice@example.com"), "resend_cooldown", 60)
	}
	require.Equal(t, before, f.limiterState(t), "requests denied by the cooldown must not change any counter")
	require.EqualValues(t, 1, atomic.LoadInt64(&f.provider.calls))
}

type lockedUserManager struct {
	challengekit.ManagerInterface
}

func (lockedUserManager) IsUserLocked(context.Context, string) bool { return true }

func TestCreateChallenge_LockedUserSpendsNoQuotaOrCooldown(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.h.challengeManager = lockedUserManager{f.h.challengeManager}

	resp := f.create(t, "alice", "alice@example.com")
	require.Equal(t, fiber.StatusForbidden, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"user_locked"`)
	require.Empty(t, f.limiterState(t), "a locked user must not consume counters or start a cooldown")
	require.Zero(t, atomic.LoadInt64(&f.provider.calls))
}

// IsUserLocked reports true when Redis is unreachable. With the lock check
// ahead of the limiter, that case must still answer 503.
func TestCreateChallenge_LockCheckOutageIsBackendUnavailable(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.mr.Close()
	resp := f.create(t, "alice", "alice@example.com")
	require.Equal(t, fiber.StatusServiceUnavailable, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"backend_unavailable"`)
	require.Zero(t, atomic.LoadInt64(&f.provider.calls))
}

func TestCreateChallenge_DestinationLimitDenialLeavesUserAndIPCounters(t *testing.T) {
	f := newCreationLimitFixture(t)
	config.RateLimitPerDestination = 1
	require.Equal(t, fiber.StatusOK, f.create(t, "alice", "shared@example.com").status)
	// Clear the cooldowns so the destination limit is the gate under test.
	f.mr.Del(f.userCooldownKey("alice", "shared@example.com"))
	f.mr.Del(f.destCooldownKey("shared@example.com"))
	before := f.limiterState(t)
	require.Equal(t, "1", f.ipCounter(t))

	requireRateLimited(t, f.create(t, "bob", "shared@example.com"), "rate_limit_exceeded", 3600)

	require.Equal(t, before, f.limiterState(t))
	require.False(t, f.mr.Exists(f.userKey("bob")), "the denied user's counter must not be created")
	require.Equal(t, "1", f.ipCounter(t))
	require.EqualValues(t, 1, atomic.LoadInt64(&f.provider.calls))
}

func TestCreateChallenge_IPLimitDenialLeavesUserCounter(t *testing.T) {
	f := newCreationLimitFixture(t)
	config.RateLimitPerIP = 1
	require.Equal(t, fiber.StatusOK, f.create(t, "alice", "alice@example.com").status)
	before := f.limiterState(t)

	requireRateLimited(t, f.create(t, "bob", "bob@example.com"), "rate_limit_exceeded", 60)
	require.Equal(t, before, f.limiterState(t))
}

func TestCreateChallenge_RotatingUserIDDoesNotBypassCooldown(t *testing.T) {
	f := newCreationLimitFixture(t)
	require.Equal(t, fiber.StatusOK, f.create(t, "alice", "victim@example.com").status)
	before := f.limiterState(t)

	for _, user := range []string{"mallory-1", "mallory-2", "mallory-3"} {
		requireRateLimited(t, f.create(t, user, "victim@example.com"), "resend_cooldown", 60)
		require.False(t, f.mr.Exists(f.userKey(user)))
	}
	require.Equal(t, before, f.limiterState(t))
	require.EqualValues(t, 1, atomic.LoadInt64(&f.provider.calls))

	// The destination is available again once the cooldown has elapsed.
	f.mr.FastForward(config.ResendCooldown + time.Second)
	require.Equal(t, fiber.StatusOK, f.create(t, "mallory-1", "victim@example.com").status)
}

// A user+destination cooldown written before the upgrade has no
// destination-only counterpart. It must still be honoured.
func TestCreateChallenge_ExistingUserDestinationCooldownIsHonoured(t *testing.T) {
	f := newCreationLimitFixture(t)
	key := f.userCooldownKey("alice", "alice@example.com")
	require.NoError(t, f.mr.Set(key, "1"))
	f.mr.SetTTL(key, 30*time.Second)

	requireRateLimited(t, f.create(t, "alice", "alice@example.com"), "resend_cooldown", 30)
	require.Zero(t, atomic.LoadInt64(&f.provider.calls))
}

func TestCreateChallenge_ProviderFailureKeepsQuotaSpent(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(&failingProvider{channel: provider.ChannelEmail}))

	resp := f.create(t, "alice", "alice@example.com")
	require.Equal(t, fiber.StatusInternalServerError, resp.status, string(resp.body))
	state := f.limiterState(t)
	require.Equal(t, "1", state[f.userKey("alice")])
	require.Equal(t, "1", state[f.destKey("alice@example.com")])
	require.Equal(t, "1", f.ipCounter(t))
	require.Contains(t, state, f.destCooldownKey("alice@example.com"))
}

func TestCreateChallenge_ConcurrentRequestsAdmitExactlyTheLimit(t *testing.T) {
	const requests, limit = 24, 5
	// The hash budget does not queue; size it so it is not the gate under test.
	oldHash := config.MaxConcurrentHashOperations
	t.Cleanup(func() { config.MaxConcurrentHashOperations = oldHash })
	config.MaxConcurrentHashOperations = requests
	f := newCreationLimitFixture(t)
	config.RateLimitPerIP = limit

	var ok, limited atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, err := idemRequest(f.app, "", emailChallengeBody(fmt.Sprintf("user-%d", i), fmt.Sprintf("user-%d@example.com", i)))
			if err != nil {
				t.Errorf("request %d: %v", i, err)
				return
			}
			switch resp.status {
			case fiber.StatusOK:
				ok.Add(1)
			case fiber.StatusTooManyRequests:
				limited.Add(1)
			default:
				t.Errorf("request %d: status=%d body=%s", i, resp.status, resp.body)
			}
		}(i)
	}
	wg.Wait()

	require.EqualValues(t, limit, ok.Load())
	require.EqualValues(t, requests-limit, limited.Load())
	require.EqualValues(t, limit, atomic.LoadInt64(&f.provider.calls))
	require.Equal(t, fmt.Sprint(limit), f.ipCounter(t))
	users := 0
	for key := range f.limiterState(t) {
		if strings.HasPrefix(key, "ratelimit:user:") {
			users++
		}
	}
	require.Equal(t, limit, users, "denied requests must not leave a user counter")
}
