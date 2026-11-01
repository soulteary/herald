package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
)

// The stock manager swaps the index and asks its caller to revoke the old
// challenge separately. Herald requires revocation to complete before a
// successful activation, including when its process exits or loses a reply.
type atomicChallengeManager struct {
	challengekit.ManagerInterface
	client *redis.Client
	config challengekit.Config
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
if previous and previous ~= ARGV[1] then
    redis.call('DEL', ARGV[2] .. previous)
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[3])
return previous or ''
`

func (m *atomicChallengeManager) SwapActive(ctx context.Context, ch *challengekit.Challenge) (string, error) {
	if ch == nil {
		return "", fmt.Errorf("nil challenge")
	}
	ttlMs := m.config.Expiry.Milliseconds()
	if m.client == nil || ttlMs <= 0 {
		return "", fmt.Errorf("%w: invalid activation backend or expiry", challengekit.ErrBackendUnavailable)
	}
	previous, err := m.client.Eval(ctx, activateAndRevokeScript, []string{m.activeIndexKey(ch)}, ch.ID, m.config.ChallengeKeyPrefix, ttlMs).Text()
	if err != nil {
		return "", fmt.Errorf("%w: activate and revoke: %v", challengekit.ErrBackendUnavailable, err)
	}
	return previous, nil
}

// Preserve challenge-kit v1.10.3's index encoding so the first post-upgrade
// resend also revokes the code referenced by an existing deployment. Tests
// seed this index with the dependency's unwrapped manager.
func (m *atomicChallengeManager) activeIndexKey(ch *challengekit.Challenge) string {
	identity := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(ch.UserID)),
		strings.ToLower(strings.TrimSpace(ch.Purpose)),
		strings.ToLower(strings.TrimSpace(string(ch.Channel))),
		strings.ToLower(strings.TrimSpace(ch.Destination)),
	}, "\x1f")
	sum := sha256.Sum256([]byte(identity))
	return m.config.ActiveIndexPrefix + hex.EncodeToString(sum[:])
}
