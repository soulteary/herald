package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	logger "github.com/soulteary/logger-kit/v3"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/security"
)

// failedAttemptCounterPrefix holds the per-user count of wrong codes. It sits
// next to the other user lock key spaces and, like them, carries a peppered
// digest in place of the user ID.
const failedAttemptCounterPrefix = "otp:lock:failed:"

// userLockValue is what the challenge manager stores under a user lock key.
// Only the key's existence is ever checked.
const userLockValue = `"1"`

// failedAttemptScript counts one wrong code for a user and locks the user
// when the count reaches the threshold.
//
// The counter is a fixed window: its lifetime is set by the first wrong code
// and never extended, and a counter that lost its lifetime is given one. When
// the threshold is reached the counter is cleared, so the next count starts
// from zero, as the exhausted-challenge record does. A lock that is already
// in effect is left alone, so its deadline is never pushed back.
//
// KEYS[1] is the counter, KEYS[2] the user lock key. ARGV is the window (ms),
// the threshold, the lock duration (ms) and the lock value. The reply is
// {count, 1 when this call set the lock}.
const failedAttemptScript = `
local count = redis.call("incr", KEYS[1])
if count == 1 or redis.call("pttl", KEYS[1]) < 0 then
	redis.call("pexpire", KEYS[1], ARGV[1])
end
if count < tonumber(ARGV[2]) then
	return {count, 0}
end
redis.call("del", KEYS[1])
if redis.call("exists", KEYS[2]) == 1 then
	return {count, 0}
end
redis.call("set", KEYS[2], ARGV[4], "px", ARGV[3])
return {count, 1}
`

// failedAttemptGuard locks a user after too many wrong codes within a window,
// whichever challenges they were submitted against.
//
// The challenge manager locks a user only after whole challenges have been
// exhausted. Wrong codes against a challenge that is replaced before its last
// attempt are counted nowhere, so a caller who requests a new code after
// MAX_ATTEMPTS-1 wrong ones can keep guessing without ever being locked. This
// guard counts every wrong code per user and sets the same lock key the
// manager checks, so the lock is enforced by the existing paths: verification
// answers user_locked and creation answers 403/user_locked.
type failedAttemptGuard struct {
	client   *redis.Client
	digester *security.Digester
	// lookupUser returns the user a challenge belongs to, or "" when the
	// challenge does not exist.
	lookupUser func(ctx context.Context, challengeID string) (string, error)
	log        *logger.Logger
}

func newFailedAttemptGuard(client *redis.Client, digester *security.Digester, manager challengekit.ManagerInterface, log *logger.Logger) *failedAttemptGuard {
	return &failedAttemptGuard{
		client:   client,
		digester: digester,
		log:      log,
		lookupUser: func(ctx context.Context, challengeID string) (string, error) {
			ch, err := manager.Get(ctx, challengeID)
			if err != nil {
				if errors.Is(err, challengekit.ErrNotFound) || errors.Is(err, redis.Nil) {
					return "", nil
				}
				return "", err
			}
			return ch.UserID, nil
		},
	}
}

func (g *failedAttemptGuard) enabled() bool {
	return g != nil && config.FailedAttemptLockThreshold > 0
}

// subject returns the user behind a challenge before it is verified, for the
// v1 route, whose request does not name the user. It is read first because a
// wrong code can be followed at once by a replacement challenge that deletes
// this one. Errors are not reported here: record looks the user up again when
// it has a wrong code to attribute.
func (g *failedAttemptGuard) subject(ctx context.Context, challengeID string) string {
	if !g.enabled() {
		return ""
	}
	userID, _ := g.lookupUser(ctx, challengeID)
	return userID
}

// countsAsWrongCode reports whether a verification result is a wrong code
// that used up an attempt. Those are the results that carry the remaining
// attempts: "invalid", and "locked" for the attempt that exhausts the
// challenge. A later probe of an exhausted challenge, an expired or unknown
// challenge, a context mismatch and every backend or contention failure
// compare no code and are not counted.
func countsAsWrongCode(result *challengekit.VerifyResult) bool {
	if result == nil || result.OK || result.RemainingAttempts == nil {
		return false
	}
	return result.Reason == challengekit.ReasonInvalid || result.Reason == challengekit.ReasonLocked
}

// record counts a wrong code for the user and locks the user at the
// threshold. It returns an error when the count could not be stored; the
// caller then answers 503 instead of the verification result, as the other
// gates do when Redis fails.
func (g *failedAttemptGuard) record(ctx context.Context, userID, challengeID string, result *challengekit.VerifyResult) error {
	if !g.enabled() || !countsAsWrongCode(result) {
		return nil
	}
	if userID == "" {
		found, err := g.lookupUser(ctx, challengeID)
		if err != nil {
			return fmt.Errorf("failed-attempt count: look up challenge user: %w", err)
		}
		if found == "" {
			// The challenge is gone and was unreadable before verification
			// too. There is no user to count against.
			return nil
		}
		userID = found
	}
	if g.client == nil {
		return errors.New("failed-attempt count: redis client is nil")
	}
	identity := g.digester.DigestParts("user-lock", userID)
	reply, err := g.client.Eval(ctx, failedAttemptScript,
		[]string{failedAttemptCounterPrefix + identity, userLockKeyPrefix + identity},
		config.FailedAttemptLockWindow.Milliseconds(), config.FailedAttemptLockThreshold,
		config.LockoutDuration.Milliseconds(), userLockValue,
	).Int64Slice()
	if err != nil {
		return fmt.Errorf("failed-attempt count: %w", err)
	}
	if len(reply) != 2 {
		return errors.New("failed-attempt count: unexpected reply")
	}
	if reply[1] == 1 {
		metrics.RecordFailedAttemptLock()
		g.log.Warn().
			Str("user_id_digest", g.digester.Digest(userID)).
			Int64("failed_attempts", reply[0]).
			Int("threshold", config.FailedAttemptLockThreshold).
			Dur("window", config.FailedAttemptLockWindow).
			Dur("lockout", config.LockoutDuration).
			Msg("user locked after repeated wrong verification codes")
	}
	return nil
}
