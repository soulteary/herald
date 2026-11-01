package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestVerifiedHMACIdentityUsesResolvedKeyWithoutChangingWireHeader(t *testing.T) {
	for _, keyID := range []string{"", "primary"} {
		t.Run(keyID, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			now := time.Now()
			cfg := Config{Mode: ModeHMACV2, DefaultKeyID: "primary", KeyProvider: func(string) string { return "secret" }, NonceStore: NewNonceStore(rc, "nonce:", []byte("secret"), time.Minute), FailClosed: true}
			app := fiber.New()
			app.Use(New(cfg))
			app.Post("/resource", func(c fiber.Ctx) error {
				service, resolved, ok := VerifiedHMACIdentity(c)
				require.True(t, ok)
				require.Equal(t, "caller", service)
				require.Equal(t, "primary", resolved)
				require.Equal(t, keyID, c.Get(HeaderKeyID))
				return c.SendStatus(204)
			})
			req := httptest.NewRequest("POST", "/resource?scope=write", strings.NewReader(middlewareTestBody))
			for k, v := range signedHeaders(now, "nonce", keyID, "secret") {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, 204, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
		})
	}
}
func TestNonHMACModesHaveNoVerifiedHMACIdentity(t *testing.T) {
	for _, mode := range []Mode{ModeNone, ModeAPIKey} {
		app := fiber.New()
		app.Use(New(Config{Mode: mode, APIKey: "secret"}))
		app.Post("/resource", func(c fiber.Ctx) error {
			_, _, ok := VerifiedHMACIdentity(c)
			require.False(t, ok)
			return c.SendStatus(204)
		})
		req := httptest.NewRequest("POST", "/resource", nil)
		req.Header.Set("X-API-Key", "secret")
		req.Header.Set(HeaderService, "caller")
		req.Header.Set(HeaderKeyID, "primary")
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, 204, resp.StatusCode)
		require.NoError(t, resp.Body.Close())
	}
}
func TestReservedDefaultKeyIDRejectsBeforeNonceMutation(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	cfg := Config{Mode: ModeHMACV2, DefaultKeyID: "primary|kid:other", KeyProvider: func(string) string { t.Error("invalid default key must not reach lookup"); return "secret" }, NonceStore: NewNonceStore(rc, "nonce:", []byte("secret"), time.Minute)}
	status, reason := runMiddlewareRequest(t, cfg, signedHeaders(time.Now(), "nonce", "", "secret"))
	require.Equal(t, http.StatusUnauthorized, status)
	require.Equal(t, "invalid_auth_headers", reason)
	require.Empty(t, mr.Keys())
}
