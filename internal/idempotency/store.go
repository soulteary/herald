package idempotency

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// State is the lifecycle state of an idempotency record.
type State string

const (
	// StatePending means a request with this key is in-flight; a concurrent
	// duplicate must not proceed.
	StatePending State = "pending"
	// StateSucceeded means the original request completed and the stored
	// response should be replayed.
	StateSucceeded State = "succeeded"
	// StateFailed means the original request failed terminally; a retry with the
	// same key may proceed (the record is cleared on failure).
	StateFailed State = "failed"
)

// Errors returned by the store.
var (
	// ErrInFlight indicates a concurrent request holds the pending slot.
	ErrInFlight = errors.New("idempotency: request in flight")
	// ErrConflict indicates the same key was used with a different request
	// fingerprint (different body). This maps to HTTP 409.
	ErrConflict = errors.New("idempotency: key reused with a different request")
	// ErrBackendUnavailable indicates the Redis backend failed; callers must
	// fail closed rather than proceeding without idempotency protection.
	ErrBackendUnavailable = errors.New("idempotency: backend unavailable")
)

// Record is the persisted idempotency record. The Response is opaque JSON that
// the caller stores on success and replays on a duplicate.
type Record struct {
	State       State           `json:"state"`
	Fingerprint string          `json:"fingerprint"`
	Response    json.RawMessage `json:"response,omitempty"`
	CreatedAt   int64           `json:"created_at"`
}

// Store implements an atomic, principal-namespaced idempotency store backed by
// Redis. Keys are HMAC(principal + client-key) so a raw client-supplied key is
// never used directly as a Redis key and one principal cannot probe or collide
// with another principal's keys.
type Store struct {
	client *redis.Client
	prefix string
	secret []byte
	ttl    time.Duration
}

// NewStore creates a Store. secret must be non-empty (used to derive keys and
// prevent cross-principal collisions / key guessing).
func NewStore(client *redis.Client, prefix string, secret []byte, ttl time.Duration) *Store {
	if prefix == "" {
		prefix = "otp:idem:"
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Store{client: client, prefix: prefix, secret: secret, ttl: ttl}
}

// redisKey derives the opaque Redis key from principal + client key. Even if
// secret is empty (misconfiguration), we still hash so the raw key never lands
// in Redis verbatim.
func (s *Store) redisKey(principal, clientKey string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(principal))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(clientKey))
	return s.prefix + hex.EncodeToString(mac.Sum(nil))
}

// Fingerprint computes a stable fingerprint over the normalized request fields
// so the same key reused with a different body is detected as a conflict.
func Fingerprint(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Begin attempts to claim the pending slot for (principal, clientKey,
// fingerprint). Outcomes:
//   - nil, nil, nil            -> caller owns the slot and must proceed, then
//     call Succeed or Fail.
//   - replayResp, nil, nil     -> a prior success exists; caller replays it
//     (replayResp non-nil).
//   - nil, nil, ErrInFlight    -> a concurrent request holds the slot.
//   - nil, nil, ErrConflict    -> key reused with a different fingerprint.
//   - nil, nil, ErrBackendUnavailable -> Redis failure; caller fails closed.
func (s *Store) Begin(ctx context.Context, principal, clientKey, fingerprint string) (replay json.RawMessage, owned bool, err error) {
	key := s.redisKey(principal, clientKey)

	pending := Record{State: StatePending, Fingerprint: fingerprint, CreatedAt: time.Now().Unix()}
	pendingJSON, _ := json.Marshal(pending)

	// Atomic claim: only the first caller sets the pending placeholder.
	set, serr := s.client.SetNX(ctx, key, pendingJSON, s.ttl).Result()
	if serr != nil {
		return nil, false, ErrBackendUnavailable
	}
	if set {
		return nil, true, nil
	}

	// Slot already exists: read and classify.
	raw, gerr := s.client.Get(ctx, key).Bytes()
	if gerr != nil {
		if errors.Is(gerr, redis.Nil) {
			// Race: it expired between SetNX and Get. Treat as in-flight; caller
			// should retry rather than double-send.
			return nil, false, ErrInFlight
		}
		return nil, false, ErrBackendUnavailable
	}
	var existing Record
	if json.Unmarshal(raw, &existing) != nil {
		return nil, false, ErrBackendUnavailable
	}
	if existing.Fingerprint != fingerprint {
		return nil, false, ErrConflict
	}
	switch existing.State {
	case StateSucceeded:
		return existing.Response, false, nil
	case StateFailed:
		// Allow a retry: clear and re-claim.
		if delErr := s.client.Del(ctx, key).Err(); delErr != nil {
			return nil, false, ErrBackendUnavailable
		}
		set2, serr2 := s.client.SetNX(ctx, key, pendingJSON, s.ttl).Result()
		if serr2 != nil {
			return nil, false, ErrBackendUnavailable
		}
		if set2 {
			return nil, true, nil
		}
		return nil, false, ErrInFlight
	default:
		return nil, false, ErrInFlight
	}
}

// Succeed stores the terminal success response so future duplicates replay it.
func (s *Store) Succeed(ctx context.Context, principal, clientKey, fingerprint string, response json.RawMessage) error {
	key := s.redisKey(principal, clientKey)
	rec := Record{State: StateSucceeded, Fingerprint: fingerprint, Response: response, CreatedAt: time.Now().Unix()}
	recJSON, _ := json.Marshal(rec)
	if err := s.client.Set(ctx, key, recJSON, s.ttl).Err(); err != nil {
		return ErrBackendUnavailable
	}
	return nil
}

// Fail clears the pending slot so a subsequent retry with the same key can
// proceed. A terminal failure does not cache a poisoned response.
func (s *Store) Fail(ctx context.Context, principal, clientKey string) error {
	key := s.redisKey(principal, clientKey)
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return ErrBackendUnavailable
	}
	return nil
}
