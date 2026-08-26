package idempotency

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
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
	// ErrOwnershipLost indicates that a pending record expired or was replaced
	// before its original handler attempted to complete it.
	ErrOwnershipLost = errors.New("idempotency: pending ownership lost")
)

// Record is the persisted idempotency record. The Response is opaque JSON that
// the caller stores on success and replays on a duplicate.
type Record struct {
	State       State           `json:"state"`
	Fingerprint string          `json:"fingerprint"`
	Owner       string          `json:"owner,omitempty"`
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
//   - nil, owner, nil          -> caller owns the slot and must proceed, then
//     call Succeed or Fail.
//   - replayResp, "", nil      -> a prior success exists; caller replays it
//     (replayResp non-nil).
//   - nil, "", ErrInFlight    -> a concurrent request holds the slot.
//   - nil, "", ErrConflict    -> key reused with a different fingerprint.
//   - nil, "", ErrBackendUnavailable -> Redis failure; caller fails closed.
func (s *Store) Begin(ctx context.Context, principal, clientKey, fingerprint string) (replay json.RawMessage, owner string, err error) {
	key := s.redisKey(principal, clientKey)
	ownerBytes := make([]byte, 16)
	if _, err := rand.Read(ownerBytes); err != nil {
		return nil, "", ErrBackendUnavailable
	}
	owner = hex.EncodeToString(ownerBytes)

	pending := Record{State: StatePending, Fingerprint: fingerprint, Owner: owner, CreatedAt: time.Now().Unix()}
	pendingJSON, _ := json.Marshal(pending)

	// Atomic claim: only the first caller sets the pending placeholder.
	set, serr := s.client.SetNX(ctx, key, pendingJSON, s.ttl).Result()
	if serr != nil {
		return nil, "", ErrBackendUnavailable
	}
	if set {
		return nil, owner, nil
	}

	// Slot already exists: read and classify.
	raw, gerr := s.client.Get(ctx, key).Bytes()
	if gerr != nil {
		if errors.Is(gerr, redis.Nil) {
			// Race: it expired between SetNX and Get. Treat as in-flight; caller
			// should retry rather than double-send.
			return nil, "", ErrInFlight
		}
		return nil, "", ErrBackendUnavailable
	}
	var existing Record
	if json.Unmarshal(raw, &existing) != nil {
		return nil, "", ErrBackendUnavailable
	}
	if existing.Fingerprint != fingerprint {
		return nil, "", ErrConflict
	}
	switch existing.State {
	case StateSucceeded:
		return existing.Response, "", nil
	case StateFailed:
		return nil, "", ErrInFlight
	default:
		return nil, "", ErrInFlight
	}
}

// Succeed stores the terminal success response so future duplicates replay it.
func (s *Store) Succeed(ctx context.Context, principal, clientKey, fingerprint, owner string, response json.RawMessage) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx) error {
		rec := Record{State: StateSucceeded, Fingerprint: fingerprint, Response: response, CreatedAt: time.Now().Unix()}
		recJSON, _ := json.Marshal(rec)
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, recJSON, s.ttl)
			return nil
		})
		return err
	})
}

// Fail clears the pending slot so a subsequent retry with the same key can
// proceed. A terminal failure does not cache a poisoned response.
func (s *Store) Fail(ctx context.Context, principal, clientKey, fingerprint, owner string) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		return err
	})
}

// Refresh verifies that the caller still owns a pending record and renews its
// TTL. Handlers call this immediately before activation so an expired, stale
// request cannot mutate challenge state owned by a replacement request.
func (s *Store) Refresh(ctx context.Context, principal, clientKey, fingerprint, owner string) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Expire(ctx, key, s.ttl)
			return nil
		})
		return err
	})
}

func (s *Store) mutateOwned(ctx context.Context, key, fingerprint, owner string, mutate func(*redis.Tx) error) error {
	err := s.client.Watch(ctx, func(tx *redis.Tx) error {
		raw, err := tx.Get(ctx, key).Bytes()
		if errors.Is(err, redis.Nil) {
			return ErrOwnershipLost
		}
		if err != nil {
			return err
		}
		var current Record
		if err := json.Unmarshal(raw, &current); err != nil {
			return err
		}
		if current.State != StatePending || current.Fingerprint != fingerprint || current.Owner != owner || owner == "" {
			return ErrOwnershipLost
		}
		return mutate(tx)
	}, key)
	if errors.Is(err, ErrOwnershipLost) || errors.Is(err, redis.TxFailedErr) {
		return ErrOwnershipLost
	}
	if err != nil {
		return ErrBackendUnavailable
	}
	return nil
}
