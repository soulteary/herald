package handlers

import (
	"context"
	"encoding/json"
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
	if err := h.providerRegistry.Register(p); err != nil {
		t.Fatal(err)
	}
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
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		statuses, responses = append(statuses, resp.StatusCode), append(responses, string(data))
	}
	for i, status := range statuses {
		var result struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal([]byte(responses[i]), &result); err != nil {
			t.Fatal(err)
		}
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
			if err := h.providerRegistry.Register(p); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			// Seed the literal namespace used before the header validation fix.
			const principal, key = "svc:caller|kid:key", "existing-key"
			fp := idempotency.Fingerprint("user", "email", "a@example.com", "login", "")
			_, owner, err := h.idempotencyStore.Begin(ctx, principal, key, fp)
			if err != nil || owner == "" {
				t.Fatalf("seed claim: owner=%q err=%v", owner, err)
			}
			if state != idempotency.StatePending {
				if err := h.idempotencyStore.MarkSending(ctx, principal, key, fp, owner, "existing-challenge"); err != nil {
					t.Fatal(err)
				}
			}
			cached := json.RawMessage(`{"challenge_id":"existing-challenge"}`)
			if state == idempotency.StateSucceeded {
				if err := h.idempotencyStore.Succeed(ctx, principal, key, fp, owner, cached); err != nil {
					t.Fatal(err)
				}
			}
			keys := mr.Keys()
			if len(keys) != 1 {
				t.Fatalf("unexpected seed keys: %v", keys)
			}
			before, err := rc.Get(ctx, keys[0]).Result()
			if err != nil {
				t.Fatal(err)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantBody := http.StatusConflict, "idempotency_in_flight"
			if state == idempotency.StateSending {
				wantBody = "delivery_unknown"
			}
			if state == idempotency.StateSucceeded {
				wantStatus, wantBody = http.StatusOK, string(cached)
			}
			if resp.StatusCode != wantStatus || !strings.Contains(string(data), wantBody) || atomic.LoadInt64(&p.calls) != 0 {
				t.Fatalf("legacy namespace changed: status=%d body=%s provider calls=%d", resp.StatusCode, data, atomic.LoadInt64(&p.calls))
			}
			after, err := rc.Get(ctx, keys[0]).Result()
			if err != nil || after != before || mr.TTL(keys[0]) != ttl || len(mr.Keys()) != 1 {
				t.Fatalf("legacy record changed: err=%v ttl=%s keys=%v", err, mr.TTL(keys[0]), mr.Keys())
			}
		})
	}
}
