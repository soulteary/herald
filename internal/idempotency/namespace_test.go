package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func namespaceStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rc.Close() })
	return NewStore(rc, "otp:idem2:", []byte("secret"), time.Minute), mr
}

func TestLegacyNamespaceGuardPreservesAmbiguousRecords(t *testing.T) {
	for _, state := range []State{StatePending, StateSending, StateSucceeded} {
		t.Run(string(state), func(t *testing.T) {
			s, mr := namespaceStore(t)
			ctx := context.Background()
			legacy := "svc:service|kid:"
			canonical := legacy + "primary"
			record := Record{State: state, Owner: "legacy-owner", Fingerprint: "same", Response: json.RawMessage(`{"challenge_id":"private-legacy"}`)}
			payload, err := json.Marshal(record)
			require.NoError(t, err)
			ttl := time.Minute
			if state == StateSending {
				ttl = 0
			}
			key := s.redisKey(legacy, "key")
			require.NoError(t, s.client.Set(ctx, key, payload, ttl).Err())
			before := mr.TTL(key)
			// Even a matching fingerprint cannot establish the original default key.
			for _, principal := range []string{canonical, legacy + "rotated-key"} {
				replay, owner, err := s.BeginWithLegacyGuard(ctx, principal, legacy, "key", "same")
				require.ErrorIs(t, err, ErrNamespaceConflict)
				require.Empty(t, replay)
				require.Empty(t, owner)
				require.False(t, mr.Exists(s.redisKey(principal, "key")))
				require.Equal(t, string(payload), mrGet(t, mr, key))
				require.Equal(t, before, mr.TTL(key))
			}
		})
	}
}
func mrGet(t *testing.T, mr *miniredis.Miniredis, key string) string {
	t.Helper()
	value, err := mr.Get(key)
	require.NoError(t, err)
	return value
}

func TestGuardedNamespaceRetainsExplicitKeyRecords(t *testing.T) {
	s, mr := namespaceStore(t)
	ctx := context.Background()
	principal := "svc:service|kid:primary"
	legacy := "svc:service|kid:"
	_, owner, err := s.Begin(ctx, principal, "key", "fingerprint")
	require.NoError(t, err)
	_, _, err = s.BeginWithLegacyGuard(ctx, principal, legacy, "key", "fingerprint")
	require.ErrorIs(t, err, ErrInFlight)
	require.NoError(t, s.MarkSending(ctx, principal, "key", "fingerprint", owner, "challenge"))
	_, _, err = s.BeginWithLegacyGuard(ctx, principal, legacy, "key", "fingerprint")
	require.ErrorIs(t, err, ErrDeliveryUnknown)
	response := json.RawMessage(`{"challenge_id":"owned"}`)
	require.NoError(t, s.Succeed(ctx, principal, "key", "fingerprint", owner, response))
	key := s.redisKey(principal, "key")
	before := mr.TTL(key)
	bytes := mrGet(t, mr, key)
	replay, owner, err := s.BeginWithLegacyGuard(ctx, principal, legacy, "key", "fingerprint")
	require.NoError(t, err)
	require.Empty(t, owner)
	require.JSONEq(t, string(response), string(replay))
	require.Equal(t, bytes, mrGet(t, mr, key))
	require.Equal(t, before, mr.TTL(key))
	_, _, err = s.BeginWithLegacyGuard(ctx, principal, legacy, "key", "different")
	require.ErrorIs(t, err, ErrConflict)
}
func TestGuardedNamespaceConcurrentClaimsHaveOneOwner(t *testing.T) {
	s, _ := namespaceStore(t)
	var owners atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Go(func() {
			_, owner, err := s.BeginWithLegacyGuard(context.Background(), "canonical", "legacy", "key", "fingerprint")
			if owner != "" {
				owners.Add(1)
			}
			if err != nil && !errors.Is(err, ErrInFlight) {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, owners.Load())
}

type namespaceReplyHook struct {
	value interface{}
	fail  bool
	after bool
}

func (*namespaceReplyHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*namespaceReplyHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *namespaceReplyHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() != "eval" {
			return next(ctx, cmd)
		}
		if h.after {
			if err := next(ctx, cmd); err != nil {
				return err
			}
		}
		if h.fail {
			return errors.New("injected namespace backend failure")
		}
		cmd.(*redis.Cmd).SetVal(h.value)
		return nil
	}
}
func TestGuardedNamespaceFailsClosedOnBackendErrors(t *testing.T) {
	for _, value := range []interface{}{[]interface{}{}, []interface{}{int64(1), int64(9)}, []interface{}{"bad", ""}, []interface{}{int64(9), ""}} {
		s, _ := namespaceStore(t)
		s.client.AddHook(&namespaceReplyHook{value: value})
		_, _, err := s.BeginWithLegacyGuard(context.Background(), "canonical", "legacy", "key", "fp")
		require.ErrorIs(t, err, ErrBackendUnavailable)
	}
	for _, after := range []bool{false, true} {
		s, mr := namespaceStore(t)
		s.client.AddHook(&namespaceReplyHook{fail: true, after: after})
		_, _, err := s.BeginWithLegacyGuard(context.Background(), "canonical", "legacy", "key", "fp")
		require.ErrorIs(t, err, ErrBackendUnavailable)
		require.Equal(t, after, mr.Exists(s.redisKey("canonical", "key")))
		if after {
			var record Record
			require.NoError(t, json.Unmarshal([]byte(mrGet(t, mr, s.redisKey("canonical", "key"))), &record))
			require.Equal(t, StatePending, record.State)
		}
	}
}
func TestGuardedNamespaceBoundaryCases(t *testing.T) {
	ctx := context.Background()
	s, mr := namespaceStore(t)
	_, owner, err := s.BeginWithLegacyGuard(ctx, "same", "same", "key", "fp")
	require.NoError(t, err)
	require.NotEmpty(t, owner)
	s.ttl = time.Nanosecond
	_, owner, err = s.BeginWithLegacyGuard(ctx, "canonical", "legacy", "tiny", "fp")
	require.NoError(t, err)
	require.NotEmpty(t, owner)
	require.Equal(t, time.Millisecond, mr.TTL(s.redisKey("canonical", "tiny")))
	require.NoError(t, s.client.Set(ctx, s.redisKey("canonical", "corrupt"), "not JSON", time.Minute).Err())
	_, _, err = s.BeginWithLegacyGuard(ctx, "canonical", "legacy", "corrupt", "fp")
	require.ErrorIs(t, err, ErrBackendUnavailable)
	_, _, err = NewStore(nil, "", nil, 0).BeginWithLegacyGuard(ctx, "canonical", "legacy", "key", "fp")
	require.ErrorIs(t, err, ErrBackendUnavailable)
}
