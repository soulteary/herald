package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/security"
)

// Separate record and mutex namespaces prevent older verifiers, which ignore
// activation_state, from redeeming newly pending challenges.
const (
	challengeRecordPrefix      = "otp2:ch:"
	challengeVerifyLockPrefix  = "otp2:vlock:"
	challengeActiveIndexPrefix = "otp2:active:"
)

// The compatibility manager swaps the index and asks its caller to revoke the old
// challenge separately. Herald requires revocation to complete before a
// successful activation, including when its process exits or loses a reply.
type atomicChallengeManager struct {
	challengekit.ManagerInterface
	client *redis.Client
	config challengekit.Config
}

// User lock key spaces. v1.6.0 and earlier wrote "otp:lock:<user_id>"; the
// version segment keeps those raw-identifier keys apart from the digested ones.
const (
	userLockKeyPrefix     = "otp:lock:v2:"
	userLockCounterPrefix = "otp:lock:exhausted:"
)

// withUserLockPolicy sets the user lock policy on a challenge configuration: a
// user is locked after LOCKOUT_CHALLENGE_THRESHOLD exhausted challenges within
// LockoutDuration, and lock keys carry a peppered digest in place of the user
// ID, as the rate-limit keys do.
func withUserLockPolicy(cfg challengekit.Config, digester *security.Digester) challengekit.Config {
	cfg.LockKeyPrefix = userLockKeyPrefix
	cfg.LockCounterPrefix = userLockCounterPrefix
	cfg.LockoutThreshold = config.LockoutChallengeThreshold
	cfg.LockIdentity = func(userID string) string {
		return digester.DigestParts("user-lock", userID)
	}
	return cfg
}

// IsUserLockedE forwards to the wrapped manager so the handlers can tell a
// locked user from a Redis outage.
func (m *atomicChallengeManager) IsUserLockedE(ctx context.Context, userID string) (bool, error) {
	if checker, ok := m.ManagerInterface.(challengekit.UserLockChecker); ok {
		return checker.IsUserLockedE(ctx, userID)
	}
	return m.IsUserLocked(ctx, userID), nil
}

func newAtomicChallengeManager(client *redis.Client, cfg challengekit.Config) *atomicChallengeManager {
	if cfg.ChallengeKeyPrefix == "" {
		cfg.ChallengeKeyPrefix = "otp:ch:"
	}
	if cfg.ActiveIndexPrefix == "" {
		cfg.ActiveIndexPrefix = "otp:active:"
	}
	return &atomicChallengeManager{ManagerInterface: challengekit.NewManager(client, cfg), client: client, config: cfg}
}

// Delete the superseded challenge before publishing the new index. Lua keeps
// other clients from observing a half-completed activation. Redis script errors
// do not roll back earlier writes; this ordering also ensures a failed delete
// cannot publish an index while leaving its predecessor redeemable.
const activateAndRevokeScript = `
local previous = redis.call('GET', KEYS[1])
if redis.call('EXISTS', ARGV[2] .. ARGV[1]) == 0 then
    return redis.error_reply('pending challenge is missing')
end
local legacy = redis.call('GET', KEYS[2])
local ownedLegacy = false
if legacy then
    local record = redis.call('GET', ARGV[2] .. legacy)
    if record then
        local data = cjson.decode(record)
        ownedLegacy = data.user_id == ARGV[4] and data.purpose == ARGV[5]
            and data.channel == ARGV[6] and data.destination == ARGV[7]
    end
end
if ownedLegacy and legacy ~= ARGV[1] then
    redis.call('DEL', ARGV[2] .. legacy)
end
if previous and previous ~= ARGV[1] then
    redis.call('DEL', ARGV[2] .. previous)
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
if ownedLegacy then
    redis.call('DEL', KEYS[2])
end
return previous or (ownedLegacy and legacy) or ''
`

func (m *atomicChallengeManager) SwapActive(ctx context.Context, ch *challengekit.Challenge) (string, error) {
	if m.config.RequireActiveVerification {
		return m.ManagerInterface.SwapActive(ctx, ch)
	}
	if ch == nil {
		return "", fmt.Errorf("nil challenge")
	}
	ttlMs := m.config.Expiry.Milliseconds()
	if m.client == nil || ttlMs <= 0 {
		return "", fmt.Errorf("%w: invalid activation backend or expiry", challengekit.ErrBackendUnavailable)
	}
	previous, err := m.client.Eval(ctx, activateAndRevokeScript, []string{m.activeIndexKey(ch), m.legacyActiveIndexKey(ch)}, ch.ID, m.config.ChallengeKeyPrefix, ttlMs, ch.UserID, ch.Purpose, string(ch.Channel), ch.Destination).Text()
	if err != nil {
		return "", fmt.Errorf("%w: activate and revoke: %v", challengekit.ErrBackendUnavailable, err)
	}
	return previous, nil
}

// Use an unambiguous encoding without changing case or trimming opaque identities.
func (m *atomicChallengeManager) activeIndexKey(ch *challengekit.Challenge) string {
	identity, _ := json.Marshal([]string{ch.UserID, ch.Purpose, string(ch.Channel), ch.Destination})
	sum := sha256.Sum256(identity)
	return m.config.ActiveIndexPrefix + "v2:" + hex.EncodeToString(sum[:])
}

// Preserve challenge-kit v1.10.3's index encoding so the first post-upgrade
// resend can revoke the legacy code after checking its exact identity. Tests
// seed this index with the dependency's unwrapped manager.
func (m *atomicChallengeManager) legacyActiveIndexKey(ch *challengekit.Challenge) string {
	identity := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(ch.UserID)),
		strings.ToLower(strings.TrimSpace(ch.Purpose)),
		strings.ToLower(strings.TrimSpace(string(ch.Channel))),
		strings.ToLower(strings.TrimSpace(ch.Destination)),
	}, "\x1f")
	sum := sha256.Sum256([]byte(identity))
	return m.config.ActiveIndexPrefix + hex.EncodeToString(sum[:])
}
