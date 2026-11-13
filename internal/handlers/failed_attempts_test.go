package handlers

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
	logger "github.com/soulteary/logger-kit/v3"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
)

const failedAttemptUser = "failed-attempt-user@tenant.example"

// newFailedAttemptFixture is the user lock fixture with the wrong-code count
// switched on. The exhausted-challenge threshold is set high enough that only
// the count can lock the user.
func newFailedAttemptFixture(t *testing.T, threshold int, window time.Duration) *userLockFixture {
	t.Helper()
	f := newUserLockFixture(t, 100) // restores the failed-attempt settings on cleanup
	config.FailedAttemptLockThreshold, config.FailedAttemptLockWindow = threshold, window
	return f
}

func (f *userLockFixture) failedKey(userID string) string {
	return failedAttemptCounterPrefix + f.h.digester.DigestParts("user-lock", userID)
}

func (f *userLockFixture) failedCount(t *testing.T, userID string) string {
	t.Helper()
	if !f.mr.Exists(f.failedKey(userID)) {
		return ""
	}
	value, err := f.mr.Get(f.failedKey(userID))
	require.NoError(t, err)
	return value
}

func (f *userLockFixture) verifyV2(t *testing.T, challengeID, code, userID string) (int, map[string]any) {
	t.Helper()
	return f.post(t, "/v2/otp/verifications", fiber.Map{
		"challenge_id": challengeID, "code": code,
		"expected_user_id": userID, "expected_purpose": "login", "expected_channel": "email",
	})
}

func failedAttemptLocks(t *testing.T) float64 {
	t.Helper()
	families, err := metrics.Registry.Gatherer().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == "herald_failed_attempt_lock_total" {
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}

// Both verification routes count against one per-user counter, and the lock
// is the manager's own lock key with LOCKOUT_DURATION.
func TestFailedAttempts_BothRoutesShareOneCounterAndSetTheUserLock(t *testing.T) {
	f := newFailedAttemptFixture(t, 4, time.Hour)
	var logs bytes.Buffer
	f.h.failedAttempts.log = logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})
	locksBefore := failedAttemptLocks(t)

	first, firstCode := f.newChallenge(t, failedAttemptUser)
	for i := 0; i < 2; i++ {
		status, body := f.verify(t, first.ID, wrongCode(firstCode))
		require.Equal(t, fiber.StatusUnauthorized, status, body)
		require.Equal(t, "invalid", body["reason"])
	}
	require.Equal(t, "2", f.failedCount(t, failedAttemptUser))
	require.Equal(t, time.Hour, f.mr.TTL(f.failedKey(failedAttemptUser)))

	// The window is not extended by later wrong codes.
	f.mr.FastForward(10 * time.Minute)
	second, secondCode := f.newChallenge(t, failedAttemptUser)
	status, body := f.verifyV2(t, second.ID, wrongCode(secondCode), failedAttemptUser)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "3", f.failedCount(t, failedAttemptUser))
	require.Equal(t, 50*time.Minute, f.mr.TTL(f.failedKey(failedAttemptUser)))
	require.False(t, f.mr.Exists(f.lockKey(failedAttemptUser)))

	// The fourth wrong code is still answered as a wrong code, and locks.
	status, body = f.verifyV2(t, second.ID, wrongCode(secondCode), failedAttemptUser)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "invalid", body["reason"])
	require.True(t, f.mr.Exists(f.lockKey(failedAttemptUser)))
	require.Equal(t, config.LockoutDuration, f.mr.TTL(f.lockKey(failedAttemptUser)))
	require.Empty(t, f.failedCount(t, failedAttemptUser), "the count starts again from zero after a lock")
	require.Equal(t, 0, f.exhaustedCount(t, failedAttemptUser))

	// Locked on both routes, with the right code.
	fresh, freshCode := f.newChallenge(t, failedAttemptUser)
	status, body = f.verify(t, fresh.ID, freshCode)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "user_locked", body["reason"])
	status, body = f.verifyV2(t, second.ID, secondCode, failedAttemptUser)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "user_locked", body["reason"])
	require.Empty(t, f.failedCount(t, failedAttemptUser), "refusals while locked compare no code and are not counted")

	// One lock, reported with the digest only.
	require.InDelta(t, locksBefore+1, failedAttemptLocks(t), 0)
	require.Contains(t, logs.String(), "user locked after repeated wrong verification codes")
	require.Contains(t, logs.String(), f.h.digester.Digest(failedAttemptUser))
	require.NotContains(t, logs.String(), failedAttemptUser)
	f.requireNoRawUserIDInKeys(t, failedAttemptUser)

	// The lock ends after LOCKOUT_DURATION and the user can verify again.
	f.mr.FastForward(config.LockoutDuration + time.Second)
	third, thirdCode := f.newChallenge(t, failedAttemptUser)
	status, body = f.verify(t, third.ID, thirdCode)
	require.Equal(t, fiber.StatusOK, status, body)
}

// Outcomes that compare no code leave the counter alone.
func TestFailedAttempts_OnlyWrongCodesAreCounted(t *testing.T) {
	f := newFailedAttemptFixture(t, 50, time.Hour)

	// Unknown challenge.
	status, body := f.verify(t, "ch_AbCdEfGhIjKlMnOpQrStUv", "123456")
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "expired", body["reason"])

	// Context mismatch on the bound route, named after another user.
	ch, code := f.newChallenge(t, failedAttemptUser)
	status, body = f.verifyV2(t, ch.ID, wrongCode(code), "another-user")
	require.Equal(t, fiber.StatusConflict, status, body)
	require.Equal(t, "context_mismatch", body["reason"])
	require.Empty(t, f.failedCount(t, failedAttemptUser))
	require.Empty(t, f.failedCount(t, "another-user"))

	// Malformed code.
	status, _ = f.verify(t, ch.ID, "12")
	require.Equal(t, fiber.StatusBadRequest, status)
	require.Empty(t, f.failedCount(t, failedAttemptUser))

	// MAX_ATTEMPTS wrong codes are counted, including the exhausting one.
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		status, _ = f.verify(t, ch.ID, wrongCode(code))
		require.Equal(t, fiber.StatusUnauthorized, status)
	}
	require.Equal(t, "5", f.failedCount(t, failedAttemptUser))

	// Probing the exhausted challenge again spends no attempt and counts nothing.
	for i := 0; i < 3; i++ {
		status, body = f.verify(t, ch.ID, wrongCode(code))
		require.Equal(t, fiber.StatusUnauthorized, status, body)
		require.Equal(t, "locked", body["reason"])
	}
	require.Equal(t, "5", f.failedCount(t, failedAttemptUser))

	// A successful verification does not reset the count.
	next, nextCode := f.newChallenge(t, failedAttemptUser)
	status, body = f.verify(t, next.ID, nextCode)
	require.Equal(t, fiber.StatusOK, status, body)
	require.Equal(t, "5", f.failedCount(t, failedAttemptUser))
}

func TestFailedAttempts_WrongCodesOutsideTheWindowDoNotLock(t *testing.T) {
	f := newFailedAttemptFixture(t, 3, 30*time.Minute)
	for round := 0; round < 3; round++ {
		ch, code := f.newChallenge(t, failedAttemptUser)
		for i := 0; i < 2; i++ {
			status, body := f.verify(t, ch.ID, wrongCode(code))
			require.Equal(t, fiber.StatusUnauthorized, status, body)
			require.Equal(t, "invalid", body["reason"])
		}
		require.Equal(t, "2", f.failedCount(t, failedAttemptUser))
		f.mr.FastForward(30*time.Minute + time.Second)
		require.Empty(t, f.failedCount(t, failedAttemptUser), "the counter must expire with its window")
	}
	require.False(t, f.mr.Exists(f.lockKey(failedAttemptUser)))
}

func TestFailedAttempts_ZeroThresholdDisablesTheCount(t *testing.T) {
	f := newFailedAttemptFixture(t, 0, time.Hour)
	for round := 0; round < 5; round++ {
		ch, code := f.newChallenge(t, failedAttemptUser)
		for attempt := 1; attempt < config.MaxAttempts; attempt++ {
			status, _ := f.verify(t, ch.ID, wrongCode(code))
			require.Equal(t, fiber.StatusUnauthorized, status)
		}
	}
	require.Empty(t, f.failedCount(t, failedAttemptUser))
	require.False(t, f.mr.Exists(f.lockKey(failedAttemptUser)))
	for _, key := range f.mr.Keys() {
		require.False(t, strings.HasPrefix(key, failedAttemptCounterPrefix), key)
	}
}

// A lock that is already in effect keeps its deadline when the count reaches
// the threshold.
func TestFailedAttempts_ExistingLockIsNotExtended(t *testing.T) {
	f := newFailedAttemptFixture(t, 2, time.Hour)
	ctx := context.Background()
	guard := f.h.failedAttempts
	wrong := func() *challengekit.VerifyResult {
		remaining := 3
		return &challengekit.VerifyResult{Reason: challengekit.ReasonInvalid, RemainingAttempts: &remaining}
	}
	require.NoError(t, f.rc.Set(ctx, f.lockKey(failedAttemptUser), userLockValue, 3*time.Minute).Err())
	require.NoError(t, guard.record(ctx, failedAttemptUser, "", wrong()))
	require.NoError(t, guard.record(ctx, failedAttemptUser, "", wrong()))
	require.Equal(t, 3*time.Minute, f.mr.TTL(f.lockKey(failedAttemptUser)))
	require.Empty(t, f.failedCount(t, failedAttemptUser))
}

// If the count cannot be stored the wrong-code answer is withheld and the
// request fails closed, like the other gates.
func TestFailedAttempts_CounterOutageFailsClosed(t *testing.T) {
	f := newFailedAttemptFixture(t, 3, time.Hour)
	ch, code := f.newChallenge(t, failedAttemptUser)
	f.rc.AddHook(&rejectRateKey{prefix: failedAttemptCounterPrefix})

	status, body := f.verify(t, ch.ID, wrongCode(code))
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"])
	require.NotContains(t, body, "remaining_attempts")

	status, body = f.verifyV2(t, ch.ID, wrongCode(code), failedAttemptUser)
	require.Equal(t, fiber.StatusServiceUnavailable, status, body)
	require.Equal(t, "backend_unavailable", body["reason"])
}

func TestCountsAsWrongCode(t *testing.T) {
	remaining := 0
	for name, tc := range map[string]struct {
		result *challengekit.VerifyResult
		want   bool
	}{
		"nil":                {nil, false},
		"success":            {&challengekit.VerifyResult{OK: true}, false},
		"invalid":            {&challengekit.VerifyResult{Reason: challengekit.ReasonInvalid, RemainingAttempts: &remaining}, true},
		"exhausting attempt": {&challengekit.VerifyResult{Reason: challengekit.ReasonLocked, RemainingAttempts: &remaining}, true},
		"exhausted probe":    {&challengekit.VerifyResult{Reason: challengekit.ReasonLocked}, false},
		"expired":            {&challengekit.VerifyResult{Reason: challengekit.ReasonExpired}, false},
		"user locked":        {&challengekit.VerifyResult{Reason: challengekit.ReasonUserLocked}, false},
		"context mismatch":   {&challengekit.VerifyResult{Reason: challengekit.ReasonContextMismatch}, false},
		"contention":         {&challengekit.VerifyResult{Reason: challengekit.ReasonLockContention, RemainingAttempts: &remaining}, false},
		"backend":            {&challengekit.VerifyResult{Reason: challengekit.ReasonBackendUnavailable}, false},
	} {
		require.Equal(t, tc.want, countsAsWrongCode(tc.result), name)
	}
}

// The v1 route learns the user from the challenge. These cases cover a
// challenge that cannot be attributed.
func TestFailedAttempts_UserLookup(t *testing.T) {
	f := newFailedAttemptFixture(t, 2, time.Hour)
	ctx := context.Background()
	remaining := 1
	wrong := &challengekit.VerifyResult{Reason: challengekit.ReasonInvalid, RemainingAttempts: &remaining}

	// A nil guard and a disabled guard do nothing.
	var none *failedAttemptGuard
	require.Empty(t, none.subject(ctx, "ch_x"))
	require.NoError(t, none.record(ctx, "u", "ch_x", wrong))

	guard := f.h.failedAttempts
	ch, _ := f.newChallenge(t, failedAttemptUser)
	require.Equal(t, failedAttemptUser, guard.subject(ctx, ch.ID))
	require.Empty(t, guard.subject(ctx, "ch_missing"))

	// Unknown before verification: the user is looked up again.
	require.NoError(t, guard.record(ctx, "", ch.ID, wrong))
	require.Equal(t, "1", f.failedCount(t, failedAttemptUser))

	// The challenge is gone: nobody to count against.
	require.NoError(t, guard.record(ctx, "", "ch_missing", wrong))
	require.Equal(t, "1", f.failedCount(t, failedAttemptUser))

	// The lookup itself fails: fail closed.
	broken := *guard
	broken.lookupUser = func(context.Context, string) (string, error) { return "", errors.New("redis down") }
	require.Empty(t, broken.subject(ctx, ch.ID))
	require.ErrorContains(t, broken.record(ctx, "", ch.ID, wrong), "redis down")

	// No client: fail closed.
	broken = *guard
	broken.client = nil
	require.Error(t, broken.record(ctx, failedAttemptUser, ch.ID, wrong))
}
