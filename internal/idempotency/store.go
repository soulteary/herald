package idempotency

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// State is the lifecycle state of an idempotency record.
type State string

const (
	// StatePending is an expiring pre-send claim; a concurrent duplicate must
	// not proceed. MarkSending replaces its short TTL with the unresolved TTL
	// before delivery.
	StatePending State = "pending"
	// StateSending is persisted before delivery. A crash or an uncertain
	// provider result must never turn this key into a fresh send while the
	// record exists. It expires after the unresolved TTL so an interrupted
	// request does not leave a permanent key.
	StateSending State = "sending"
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
	// ErrDeliveryUnknown means delivery may have happened and cannot be retried safely.
	ErrDeliveryUnknown = errors.New("idempotency: delivery outcome unknown")
	// ErrNamespaceConflict indicates an unattributable legacy implicit-key record.
	ErrNamespaceConflict = errors.New("idempotency: legacy namespace conflict")

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
	ChallengeID string          `json:"challenge_id,omitempty"`
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
	// unresolvedTTL bounds records left in StateSending.
	unresolvedTTL time.Duration
}

// DefaultUnresolvedTTL is how long a record stays in StateSending when no
// other value is configured.
const DefaultUnresolvedTTL = 24 * time.Hour

// NewStore creates a Store. secret must be non-empty (used to derive keys and
// prevent cross-principal collisions / key guessing).
func NewStore(client *redis.Client, prefix string, secret []byte, ttl time.Duration) *Store {
	if prefix == "" {
		prefix = "otp:idem:"
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Store{client: client, prefix: prefix, secret: secret, ttl: ttl, unresolvedTTL: DefaultUnresolvedTTL}
}

// WithUnresolvedTTL sets how long a record may stay in StateSending. It must
// exceed the pending TTL; non-positive values keep the current setting.
func (s *Store) WithUnresolvedTTL(ttl time.Duration) *Store {
	if ttl > 0 {
		s.unresolvedTTL = ttl
	}
	return s
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
// Ordinary NUL-free requests retain their existing fingerprint for rolling
// upgrades. Embedded NULs require length framing: delimiters alone let bytes
// move between fields without changing the hash input. The v2 prefix keeps
// framed fingerprints disjoint from legacy hex digests.
func Fingerprint(parts ...string) string {
	for _, p := range parts {
		if strings.ContainsRune(p, '\x00') {
			h := sha256.New()
			var size [8]byte
			binary.BigEndian.PutUint64(size[:], uint64(len(parts)))
			_, _ = h.Write(size[:])
			for _, field := range parts {
				binary.BigEndian.PutUint64(size[:], uint64(len(field)))
				_, _ = h.Write(size[:])
				_, _ = h.Write([]byte(field))
			}
			return "v2:" + hex.EncodeToString(h.Sum(nil))
		}
	}
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
//   - nil, "", ErrDeliveryUnknown -> sending started; never resend automatically.
//   - nil, "", ErrConflict    -> key reused with a different fingerprint.
//   - nil, "", ErrBackendUnavailable -> Redis failure; caller fails closed.
func (s *Store) Begin(ctx context.Context, principal, clientKey, fingerprint string) (replay json.RawMessage, owner string, err error) {
	key := s.redisKey(principal, clientKey)
	pendingJSON, owner, err := newPendingClaim(fingerprint)
	if err != nil {
		return nil, "", err
	}

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
	return classifyExistingRecord(raw, fingerprint)
}

func classifyExistingRecord(raw []byte, fingerprint string) (json.RawMessage, string, error) {
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
	case StateSending:
		return nil, "", ErrDeliveryUnknown
	case StateFailed:
		return nil, "", ErrInFlight
	default:
		return nil, "", ErrInFlight
	}
}

// MarkSending atomically replaces the pending lease before the external side
// effect. Only its owner can complete it; Fail cannot reopen it, and the key
// stays claimed until the unresolved TTL elapses.
// ChallengeID lets operators reconcile an interrupted send without storing codes.
func (s *Store) MarkSending(ctx context.Context, principal, clientKey, fingerprint, owner, challengeID string) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx, current Record) error {
		if current.State != StatePending {
			return ErrDeliveryUnknown
		}
		current.State = StateSending
		current.ChallengeID = challengeID
		payload, err := json.Marshal(current)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, payload, s.unresolvedTTL)
			return nil
		})
		return err
	})
}

// Succeed stores the terminal success response so future duplicates replay it.
func (s *Store) Succeed(ctx context.Context, principal, clientKey, fingerprint, owner string, response json.RawMessage) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx, current Record) error {
		rec := Record{State: StateSucceeded, Fingerprint: fingerprint, ChallengeID: current.ChallengeID, Response: response, CreatedAt: time.Now().Unix()}
		recJSON, _ := json.Marshal(rec)
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, recJSON, s.ttl)
			return nil
		})
		return err
	})
}

// Fail clears a pre-send pending slot so a retry can proceed. Once sending
// starts, even an error can mean delivery happened, so the record is retained.
func (s *Store) Fail(ctx context.Context, principal, clientKey, fingerprint, owner string) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx, current Record) error {
		if current.State != StatePending {
			return ErrDeliveryUnknown
		}
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Del(ctx, key)
			return nil
		})
		return err
	})
}

// Refresh verifies that the caller still owns a pending record and renews its
// TTL before sending, or renews the unresolved TTL of a sending record. Handlers call
// this immediately before activation so an expired, stale
// request cannot mutate challenge state owned by a replacement request.
func (s *Store) Refresh(ctx context.Context, principal, clientKey, fingerprint, owner string) error {
	key := s.redisKey(principal, clientKey)
	return s.mutateOwned(ctx, key, fingerprint, owner, func(tx *redis.Tx, current Record) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			if current.State == StateSending {
				pipe.Expire(ctx, key, s.unresolvedTTL)
			} else {
				pipe.Expire(ctx, key, s.ttl)
			}
			return nil
		})
		return err
	})
}

func (s *Store) mutateOwned(ctx context.Context, key, fingerprint, owner string, mutate func(*redis.Tx, Record) error) error {
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
		if (current.State != StatePending && current.State != StateSending) || current.Fingerprint != fingerprint || current.Owner != owner || owner == "" {
			return ErrOwnershipLost
		}
		return mutate(tx, current)
	}, key)
	if errors.Is(err, ErrDeliveryUnknown) {
		return ErrDeliveryUnknown
	}
	if errors.Is(err, ErrOwnershipLost) || errors.Is(err, redis.TxFailedErr) {
		return ErrOwnershipLost
	}
	if err != nil {
		return ErrBackendUnavailable
	}
	return nil
}
