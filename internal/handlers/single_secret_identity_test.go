package handlers

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
)

func TestSingleSecretCannotCreateArbitraryIdempotencyNamespaces(t *testing.T) {
	require.False(t, config.HasHMACKeys())
	original := config.HMACSecret
	config.HMACSecret = "identity-secret"
	t.Cleanup(func() { config.HMACSecret = original })
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	hook := &rejectStateHook{}
	rc.AddHook(hook)
	h := newIdemHandlers(t, rc)
	p := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Use(auth.New(auth.Config{Mode: auth.ModeHMACV2, RequireV3: true,
		DefaultKeyID: "default", KeyProvider: config.GetHMACSecret,
		NonceStore: auth.NewNonceStore(rc, "single:nonce:", []byte("nonce-secret"), time.Minute)}))
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	first := defaultKeyRequest(t, app, "default", "first")
	require.Equal(t, 503, first.status)
	require.Positive(t, hook.rejected.Load())
	require.EqualValues(t, 1, p.calls)
	mr.FastForward(2 * time.Minute)
	retry := defaultKeyRequest(t, app, "unconfigured-key", "retry")
	t.Logf("first=%d retry=%d sends=%d", first.status, retry.status, p.calls)
	require.EqualValues(t, 1, p.calls, "an unconfigured alias must not bypass a retained sending record")
	require.Equal(t, 401, retry.status)
	require.Contains(t, string(retry.body), "invalid_key_id")
	for _, keyID := range []string{"", "default"} {
		validRetry := defaultKeyRequest(t, app, keyID, "valid-retry-"+keyID)
		require.Equal(t, 409, validRetry.status)
		require.Contains(t, string(validRetry.body), "delivery_unknown")
	}
	require.EqualValues(t, 1, p.calls)
}
