package idempotency

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
)

func newPendingClaim(fingerprint string) ([]byte, string, error) {
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return nil, "", ErrBackendUnavailable
	}
	owner := hex.EncodeToString(ownerBytes)
	payload, _ := json.Marshal(Record{State: StatePending, Fingerprint: fingerprint, Owner: owner, CreatedAt: time.Now().Unix()})
	return payload, owner, nil
}

// An old empty-key-ID record contains no evidence of which default key signed
// it. Never copy or replay it into a newly resolved credential's namespace.
// Check its absence and claim the canonical key in the same Redis operation.
const guardedNamespaceScript = `
if redis.call('EXISTS', KEYS[2]) ~= 0 then
    return {0, ''}
end
local existing = redis.call('GET', KEYS[1])
if existing then
    return {1, existing}
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return {2, ''}
`

// BeginWithLegacyGuard keeps explicit-key record keys unchanged, unifies new
// implicit-key requests with them, and blocks ambiguous records from old code.
func (s *Store) BeginWithLegacyGuard(ctx context.Context, principal, legacyPrincipal, clientKey, fingerprint string) (json.RawMessage, string, error) {
	if s.client == nil {
		return nil, "", ErrBackendUnavailable
	}
	if principal == legacyPrincipal {
		return s.Begin(ctx, principal, clientKey, fingerprint)
	}
	payload, owner, err := newPendingClaim(fingerprint)
	if err != nil {
		return nil, "", err
	}
	ttlMS := s.ttl.Milliseconds()
	if ttlMS < 1 {
		ttlMS = 1
	}
	values, err := s.client.Eval(ctx, guardedNamespaceScript, []string{s.redisKey(principal, clientKey), s.redisKey(legacyPrincipal, clientKey)}, payload, ttlMS).Slice()
	if err != nil || len(values) != 2 {
		return nil, "", ErrBackendUnavailable
	}
	status, ok := values[0].(int64)
	if !ok {
		return nil, "", ErrBackendUnavailable
	}
	switch status {
	case 0:
		return nil, "", ErrNamespaceConflict
	case 1:
		raw, ok := values[1].(string)
		if !ok {
			return nil, "", ErrBackendUnavailable
		}
		return classifyExistingRecord([]byte(raw), fingerprint)
	case 2:
		return nil, owner, nil
	default:
		return nil, "", ErrBackendUnavailable
	}
}
