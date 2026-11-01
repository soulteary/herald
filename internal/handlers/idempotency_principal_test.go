package handlers

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/idempotency"
)

func TestIdempotencyPrincipalCannotAliasDifferentHMACKeys(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	p := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	credentials := []struct{ service, keyID, secret string }{
		{"service|kid:first", "second", "first-test-secret"},
		{"service", "first|kid:second", "second-test-secret"},
	}
	app := fiber.New()
	app.Post("/v1/otp/challenges", auth.New(auth.Config{
		Mode: auth.ModeHMACV2, FailClosed: true,
		KeyProvider: func(keyID string) string {
			for _, credential := range credentials {
				if credential.keyID == keyID {
					return credential.secret
				}
			}
			return ""
		},
		NonceStore: auth.NewNonceStore(rc, "principal-test:nonce:", []byte("nonce-test-secret"), time.Minute),
	}), h.CreateChallenge)
	const body = `{"user_id":"user","channel":"email","destination":"a@example.com","purpose":"login"}`
	statuses := make([]int, 0, 2)
	responses := make([]string, 0, 2)
	for i, credential := range credentials {
		timestamp, nonce := strconv.FormatInt(time.Now().Unix(), 10), "nonce-"+strconv.Itoa(i)
		req := httptest.NewRequest(http.MethodPost, "/v1/otp/challenges", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "shared-key")
		req.Header.Set(auth.HeaderSignatureVersion, auth.SignatureVersionV2)
		req.Header.Set(auth.HeaderTimestamp, timestamp)
		req.Header.Set(auth.HeaderNonce, nonce)
		req.Header.Set(auth.HeaderService, credential.service)
		req.Header.Set(auth.HeaderKeyID, credential.keyID)
		req.Header.Set(auth.HeaderSignature, auth.SignV2(credential.secret, auth.CanonicalRequest{
			Method: http.MethodPost, Path: "/v1/otp/challenges", Timestamp: timestamp,
			Nonce: nonce, Service: credential.service, KeyID: credential.keyID, Body: []byte(body),
		}))
		resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
		require.NoError(t, err)
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		statuses, responses = append(statuses, resp.StatusCode), append(responses, string(data))
	}
	for i, status := range statuses {
		var result struct {
			Reason string `json:"reason"`
		}
		require.NoError(t, json.Unmarshal([]byte(responses[i]), &result))
		if status != http.StatusUnauthorized || result.Reason != "invalid_auth_headers" {
			t.Errorf("ambiguous principal accepted: statuses=%v responses=%v provider calls=%d", statuses, responses, atomic.LoadInt64(&p.calls))
		}
	}
	if atomic.LoadInt64(&p.calls) != 0 || len(mr.Keys()) != 0 {
		t.Fatalf("rejected identity caused side effects: provider calls=%d keys=%v", atomic.LoadInt64(&p.calls), mr.Keys())
	}
}

func TestIdempotencyPrincipalRetainsExistingOrdinaryRecords(t *testing.T) {
	for _, state := range []idempotency.State{idempotency.StatePending, idempotency.StateSending, idempotency.StateSucceeded} {
		t.Run(string(state), func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			h := newIdemHandlers(t, rc)
			p := &countingProvider{channel: provider.ChannelEmail}
			h.providerRegistry = provider.NewRegistry()
			require.NoError(t, h.providerRegistry.Register(p))
			ctx := context.Background()
			// Seed the literal namespace used before the header validation fix.
			const principal, key = "svc:caller|kid:key", "existing-key"
			fp := idempotency.Fingerprint("user", "email", "a@example.com", "login", "")
			_, owner, err := h.idempotencyStore.Begin(ctx, principal, key, fp)
			require.NoError(t, err)
			require.NotEmpty(t, owner)
			if state != idempotency.StatePending {
				require.NoError(t, h.idempotencyStore.MarkSending(ctx, principal, key, fp, owner, "existing-challenge"))
			}
			cached := json.RawMessage(`{"challenge_id":"existing-challenge"}`)
			if state == idempotency.StateSucceeded {
				require.NoError(t, h.idempotencyStore.Succeed(ctx, principal, key, fp, owner, cached))
			}
			keys := mr.Keys()
			if len(keys) != 1 {
				t.Fatalf("unexpected seed keys: %v", keys)
			}
			before, err := rc.Get(ctx, keys[0]).Result()
			require.NoError(t, err)
			ttl := mr.TTL(keys[0])
			app := fiber.New()
			app.Post("/v1/otp/challenges", auth.New(auth.Config{Mode: auth.ModeAPIKey, APIKey: "test-key"}), h.CreateChallenge)
			req := httptest.NewRequest(http.MethodPost, "/v1/otp/challenges", strings.NewReader(`{"user_id":"user","channel":"email","destination":"a@example.com","purpose":"login"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", "test-key")
			req.Header.Set(auth.HeaderService, "caller")
			req.Header.Set(auth.HeaderKeyID, "key")
			req.Header.Set("Idempotency-Key", key)
			resp, err := app.Test(req)
			require.NoError(t, err)
			data, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			wantStatus, wantBody := http.StatusConflict, "idempotency_in_flight"
			if state == idempotency.StateSending {
				wantBody = "delivery_unknown"
			}
			if state == idempotency.StateSucceeded {
				wantStatus, wantBody = http.StatusOK, string(cached)
			}
			require.Equal(t, wantStatus, resp.StatusCode)
			require.Contains(t, string(data), wantBody)
			require.Zero(t, atomic.LoadInt64(&p.calls))

			after, err := rc.Get(ctx, keys[0]).Result()
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, ttl, mr.TTL(keys[0]))
			require.Len(t, mr.Keys(), 1)
		})
	}
}
