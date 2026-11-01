package handlers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"
)

// Run against a dedicated Redis instance, not an application database:
// HERALD_ACTIVATION_REDIS_ADDR=127.0.0.1:6379 go test -race -run TestAtomicActivationWithRedisServer ./internal/handlers
func TestAtomicActivationWithRedisServer(t *testing.T) {
	addr := os.Getenv("HERALD_ACTIVATION_REDIS_ADDR")
	if addr == "" {
		t.Skip("dedicated Redis integration instance not configured")
	}
	ctx := context.Background()
	rc := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	defer func() { _ = rc.Close() }()
	require.NoError(t, rc.Ping(ctx).Err())
	prefix := fmt.Sprintf("activation-test:%d:", time.Now().UnixNano())
	cfg := challengekit.Config{Expiry: time.Minute, CodeLength: 6, MaxAttempts: 5, ChallengeKeyPrefix: prefix + "ch:", ActiveIndexPrefix: prefix + "active:", LockKeyPrefix: prefix + "lock:"}
	m := newAtomicChallengeManager(rc, cfg)
	legacy := challengekit.NewManager(rc, cfg)
	old, oldCode := createActivationOTP(t, legacy, "user")
	_, err := legacy.SwapActive(ctx, old)
	require.NoError(t, err)
	replacement, newCode := createActivationOTP(t, m, "user")
	previous, err := m.SwapActive(ctx, replacement)
	require.NoError(t, err)
	require.Equal(t, old.ID, previous)
	oldResult, _ := m.Verify(ctx, old.ID, oldCode, "")
	require.NotNil(t, oldResult)
	require.False(t, oldResult.OK)
	newResult, err := m.Verify(ctx, replacement.ID, newCode, "")
	require.NoError(t, err)
	require.True(t, newResult.OK)
	again, _ := m.Verify(ctx, replacement.ID, newCode, "")
	require.NotNil(t, again)
	require.False(t, again.OK)
	// A missing candidate must not discard an existing live predecessor.
	live, _ := createActivationOTP(t, m, "another-user")
	_, err = m.SwapActive(ctx, live)
	require.NoError(t, err)
	missing := *live
	missing.ID = "missing"
	_, err = m.SwapActive(ctx, &missing)
	require.ErrorIs(t, err, challengekit.ErrBackendUnavailable)
	active, err := rc.Get(ctx, m.activeIndexKey(live)).Result()
	require.NoError(t, err)
	require.Equal(t, live.ID, active)
	// A legacy index shared by differently cased users must not revoke either identity.
	victim, victimCode := createActivationOTP(t, legacy, "Alice")
	_, err = legacy.SwapActive(ctx, victim)
	require.NoError(t, err)
	other, _ := createActivationOTP(t, m, "alice")
	_, err = m.SwapActive(ctx, other)
	require.NoError(t, err)
	require.Equal(t, victim.ID, rc.Get(ctx, m.legacyActiveIndexKey(victim)).Val())
	victimResult, err := m.VerifyWithOptions(ctx, victim.ID, victimCode, "", challengekit.VerifyOptions{ExpectedUserID: "Alice", ExpectedPurpose: "login", ExpectedChannel: challengekit.ChannelEmail})
	require.NoError(t, err)
	require.True(t, victimResult.OK)

}
