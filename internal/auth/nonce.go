package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrNonceStoreUnavailable is returned when the nonce backend fails. Callers
// must fail closed (reject the request) in production rather than allowing a
// replay window.
var ErrNonceStoreUnavailable = errors.New("auth: nonce store unavailable")

// NonceStore consumes single-use nonces via Redis SET NX EX. The nonce is
// stored under a keyed digest so the raw nonce (which may be attacker-chosen)
// never lands verbatim in Redis and one service cannot probe another's nonces.
type NonceStore struct {
	client *redis.Client
	prefix string
	secret []byte
	ttl    time.Duration
}

// NewNonceStore creates a NonceStore. ttl should cover the maximum allowed
// timestamp drift so a nonce cannot be replayed within the acceptance window.
func NewNonceStore(client *redis.Client, prefix string, secret []byte, ttl time.Duration) *NonceStore {
	if prefix == "" {
		prefix = "otp:nonce:"
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &NonceStore{client: client, prefix: prefix, secret: secret, ttl: ttl}
}

func (s *NonceStore) key(service, keyID, nonce string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(service))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(keyID))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(nonce))
	return s.prefix + hex.EncodeToString(mac.Sum(nil))
}

// Consume atomically claims the nonce. It returns:
//   - (true, nil)  when the nonce was fresh and is now consumed.
//   - (false, nil) when the nonce was already used (replay).
//   - (false, ErrNonceStoreUnavailable) on backend failure (fail closed).
//
// IMPORTANT: callers MUST verify the signature BEFORE calling Consume so that an
// invalid request cannot burn a legitimate nonce.
func (s *NonceStore) Consume(ctx context.Context, service, keyID, nonce string) (bool, error) {
	if s.client == nil {
		return false, ErrNonceStoreUnavailable
	}
	ok, err := s.client.SetNX(ctx, s.key(service, keyID, nonce), "1", s.ttl).Result()
	if err != nil {
		return false, ErrNonceStoreUnavailable
	}
	return ok, nil
}
