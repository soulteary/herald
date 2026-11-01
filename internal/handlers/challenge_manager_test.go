package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"
)

func activationFixture(t *testing.T, cfg challengekit.Config) (*atomicChallengeManager, *redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = rc.Close() })
	return newAtomicChallengeManager(rc, cfg), rc, mr
}
func createActivationOTP(t *testing.T, m challengekit.ManagerInterface, user string) (*challengekit.Challenge, string) {
	t.Helper()
	ch, code, err := m.Create(context.Background(), challengekit.CreateRequest{UserID: user, Channel: challengekit.ChannelEmail, Destination: "a@example.com", Purpose: "login"})
	require.NoError(t, err)
	return ch, code
}

func TestAtomicActivationRetainsLegacyIndexEncoding(t *testing.T) {
	for _, prefix := range []string{"", "custom:"} {
		t.Run(prefix, func(t *testing.T) {
			cfg := challengekit.Config{Expiry: 5 * time.Minute, CodeLength: 6, MaxAttempts: 5}
			if prefix != "" {
				cfg.ChallengeKeyPrefix = prefix + "ch:"
				cfg.ActiveIndexPrefix = prefix + "active:"
			}
			m, rc, mr := activationFixture(t, cfg)
			legacy := challengekit.NewManager(rc, cfg)
			old, oldCode := createActivationOTP(t, legacy, " User ")
			_, err := legacy.SwapActive(context.Background(), old)
			require.NoError(t, err)
			replacement, newCode := createActivationOTP(t, m, " User ")
			key := m.config.ChallengeKeyPrefix + replacement.ID
			before, err := rc.Get(context.Background(), key).Result()
			require.NoError(t, err)
			ttl := mr.TTL(key)
			previous, err := m.SwapActive(context.Background(), replacement)
			require.NoError(t, err)
			require.Equal(t, old.ID, previous)
			require.False(t, mr.Exists(m.config.ChallengeKeyPrefix+old.ID))
			after, err := rc.Get(context.Background(), key).Result()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, ttl, mr.TTL(key))
			require.Equal(t, cfg.Expiry, mr.TTL(m.activeIndexKey(replacement)))
			result, _ := m.Verify(context.Background(), old.ID, oldCode, "")
			require.NotNil(t, result)
			require.False(t, result.OK)
			result, err = m.VerifyWithOptions(context.Background(), replacement.ID, newCode, "", challengekit.VerifyOptions{ExpectedUserID: " User ", ExpectedPurpose: "login", ExpectedChannel: challengekit.ChannelEmail})
			require.NoError(t, err)
			require.True(t, result.OK)
		})
	}
}

func TestAtomicActivationPreservesOtherIdentitiesAndSameCandidate(t *testing.T) {
	m, _, mr := activationFixture(t, challengekit.Config{Expiry: time.Minute, CodeLength: 6, MaxAttempts: 5})
	other, _ := createActivationOTP(t, m, "other-user")
	_, err := m.SwapActive(context.Background(), other)
	require.NoError(t, err)
	ch, code := createActivationOTP(t, m, "user")
	_, err = m.SwapActive(context.Background(), ch)
	require.NoError(t, err)
	previous, err := m.SwapActive(context.Background(), ch)
	require.NoError(t, err)
	require.Equal(t, ch.ID, previous)
	require.True(t, mr.Exists(m.config.ChallengeKeyPrefix+other.ID))
	result, err := m.Verify(context.Background(), ch.ID, code, "")
	require.NoError(t, err)
	require.True(t, result.OK)
}

func TestAtomicActivationFailureDoesNotPublishIndex(t *testing.T) {
	m, _, mr := activationFixture(t, challengekit.Config{Expiry: time.Minute, CodeLength: 6})
	old, _ := createActivationOTP(t, m, "user")
	_, err := m.SwapActive(context.Background(), old)
	require.NoError(t, err)
	missing := *old
	missing.ID = "missing-pending"
	_, err = m.SwapActive(context.Background(), &missing)
	require.ErrorIs(t, err, challengekit.ErrBackendUnavailable)
	require.True(t, mr.Exists(m.config.ChallengeKeyPrefix+old.ID))
	active, err := mr.Get(m.activeIndexKey(old))
	require.NoError(t, err)
	require.Equal(t, old.ID, active)
	_, err = m.SwapActive(context.Background(), nil)
	require.Error(t, err)
	nilClient := newAtomicChallengeManager(nil, challengekit.Config{Expiry: time.Minute})
	_, err = nilClient.SwapActive(context.Background(), old)
	require.ErrorIs(t, err, challengekit.ErrBackendUnavailable)
	m.config.Expiry = 0
	_, err = m.SwapActive(context.Background(), old)
	require.ErrorIs(t, err, challengekit.ErrBackendUnavailable)
}

func TestConcurrentActivationsLeaveOnlyIndexedCandidate(t *testing.T) {
	m, rc, mr := activationFixture(t, challengekit.Config{Expiry: time.Minute, CodeLength: 6, MaxAttempts: 5})
	var candidates []*challengekit.Challenge
	for i := 0; i < 4; i++ {
		ch, _ := createActivationOTP(t, m, "user")
		candidates = append(candidates, ch)
	}
	errs := make(chan error, len(candidates))
	var wg sync.WaitGroup
	for _, ch := range candidates {
		wg.Go(func() { _, err := m.SwapActive(context.Background(), ch); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	active, err := rc.Get(context.Background(), m.activeIndexKey(candidates[0])).Result()
	require.NoError(t, err)
	for _, ch := range candidates {
		require.Equal(t, ch.ID == active, mr.Exists(m.config.ChallengeKeyPrefix+ch.ID), "candidate %s", ch.ID)
	}
}
