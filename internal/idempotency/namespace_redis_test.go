package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// Use a dedicated Redis test instance, never an application database.
func TestGuardedNamespaceWithRedisServer(t *testing.T) {
	addr := os.Getenv("HERALD_IDEMPOTENCY_REDIS_ADDR")
	if addr == "" {
		t.Skip("dedicated Redis integration instance not configured")
	}
	rc := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() { _ = rc.Close() })
	ctx := context.Background()
	require.NoError(t, rc.Ping(ctx).Err())
	s := NewStore(rc, fmt.Sprintf("namespace-test:%d:", time.Now().UnixNano()), []byte("test-secret"), time.Minute)
	_, owner, err := s.BeginWithLegacyGuard(ctx, "canonical", "legacy", "key", "fp")
	require.NoError(t, err)
	require.NotEmpty(t, owner)
	require.NoError(t, s.MarkSending(ctx, "canonical", "key", "fp", owner, "ch"))
	_, _, err = s.BeginWithLegacyGuard(ctx, "canonical", "legacy", "key", "fp")
	require.ErrorIs(t, err, ErrDeliveryUnknown)
	require.Equal(t, time.Duration(-1), rc.PTTL(ctx, s.redisKey("canonical", "key")).Val())
	require.NoError(t, s.Succeed(ctx, "canonical", "key", "fp", owner, json.RawMessage(`{"challenge_id":"ch"}`)))
	replay, _, err := s.BeginWithLegacyGuard(ctx, "canonical", "legacy", "key", "fp")
	require.NoError(t, err)
	require.JSONEq(t, `{"challenge_id":"ch"}`, string(replay))
	legacyKey := s.redisKey("legacy", "ambiguous")
	raw := `{"state":"sending","owner":"old","fingerprint":"fp","challenge_id":"private"}`
	require.NoError(t, rc.Set(ctx, legacyKey, raw, 0).Err())
	for _, principal := range []string{"primary", "replacement"} {
		replay, owner, err = s.BeginWithLegacyGuard(ctx, principal, "legacy", "ambiguous", "fp")
		require.ErrorIs(t, err, ErrNamespaceConflict)
		require.Empty(t, replay)
		require.Empty(t, owner)
		require.Equal(t, raw, rc.Get(ctx, legacyKey).Val())
		require.Equal(t, time.Duration(-1), rc.PTTL(ctx, legacyKey).Val())
		require.Zero(t, rc.Exists(ctx, s.redisKey(principal, "ambiguous")).Val())
	}
	require.NoError(t, rc.Del(ctx, legacyKey, s.redisKey("canonical", "key")).Err())
}
