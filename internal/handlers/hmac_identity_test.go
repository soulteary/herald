package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"encoding/json"
	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/idempotency"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
	"sync"
)

func defaultKeyRequest(t *testing.T, app *fiber.App, keyID, nonce string) *fiberTestResp {
	return identityRequest(t, app, keyID, "identity-secret", nonce)
}
func identityRequest(t *testing.T, app *fiber.App, keyID, secret, nonce string) *fiberTestResp {
	t.Helper()
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	canonical := auth.CanonicalRequest{Method: "POST", Path: "/v1/otp/challenges", Timestamp: timestamp, Nonce: nonce, Service: "identity-service", KeyID: keyID, IdempotencyKey: "same-client-key", Body: []byte(faultBody)}
	req := httptest.NewRequest("POST", canonical.Path, bytes.NewBufferString(faultBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(auth.HeaderSignatureVersion, auth.SignatureVersionV3)
	req.Header.Set(auth.HeaderSignature, auth.SignV3(secret, canonical))
	req.Header.Set(auth.HeaderTimestamp, timestamp)
	req.Header.Set(auth.HeaderNonce, nonce)
	req.Header.Set(auth.HeaderService, canonical.Service)
	if keyID != "" {
		req.Header.Set(auth.HeaderKeyID, keyID)
	}
	req.Header.Set("Idempotency-Key", canonical.IdempotencyKey)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return &fiberTestResp{status: resp.StatusCode, body: body}
}
func identityApp(t *testing.T, rc *redis.Client) (*fiber.App, *countingProvider) {
	p := &countingProvider{channel: provider.ChannelEmail}
	return identityProviderApp(t, rc, p), p
}
func identityProviderApp(t *testing.T, rc *redis.Client, p provider.Provider) *fiber.App {
	return identityDefaultApp(t, rc, p, "primary")
}
func identityDefaultApp(t *testing.T, rc *redis.Client, p provider.Provider, defaultID string) *fiber.App {
	h := newIdemHandlers(t, rc)
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Use(auth.New(auth.Config{Mode: auth.ModeHMACV2, RequireV3: true, DefaultKeyID: defaultID, KeyProvider: func(id string) string {
		if id == "primary" {
			return "identity-secret"
		}
		if id == "replacement" {
			return "replacement-secret"
		}
		return ""
	}, NonceStore: auth.NewNonceStore(rc, "identity:nonce:", []byte("nonce-secret"), time.Minute), FailClosed: true}))
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app
}
func TestHMACDefaultKeyIdentityCannotSplitIdempotency(t *testing.T) {
	for _, firstKey := range []string{"", "primary"} {
		t.Run("first="+firstKey, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			hook := &rejectStateHook{}
			rc.AddHook(hook)
			app, p := identityApp(t, rc)
			first := defaultKeyRequest(t, app, firstKey, "first")
			require.Equal(t, 503, first.status)
			require.Positive(t, hook.rejected.Load())
			require.EqualValues(t, 1, p.calls)
			mr.FastForward(2 * time.Minute)
			secondKey := "primary"
			if firstKey != "" {
				secondKey = ""
			}
			retry := defaultKeyRequest(t, app, secondKey, "retry")
			t.Logf("first key ID=%q retry key ID=%q first status=%d retry status=%d provider calls=%d", firstKey, secondKey, first.status, retry.status, p.calls)
			require.EqualValues(t, 1, p.calls, "same authenticated key and idempotency key must not send twice")
			require.Equal(t, 409, retry.status)
			require.Contains(t, string(retry.body), "delivery_unknown")
			require.NoError(t, rc.Ping(context.Background()).Err())
		})
	}
}

func TestHMACLegacyImplicitRecordsBlockWithoutReplayOrMutation(t *testing.T) {
	for _, state := range []idempotency.State{idempotency.StatePending, idempotency.StateSending, idempotency.StateSucceeded} {
		t.Run(string(state), func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			app, p := identityApp(t, rc)
			store := idempotency.NewStore(rc, "otp:idem2:", []byte(config.IdempotencySecret), time.Minute)
			principal := "svc:identity-service|kid:"
			fp := idempotency.Fingerprint("u1", "email", "a@b.com", "login", "")
			_, owner, err := store.Begin(context.Background(), principal, "same-client-key", fp)
			require.NoError(t, err)
			if state != idempotency.StatePending {
				require.NoError(t, store.MarkSending(context.Background(), principal, "same-client-key", fp, owner, "legacy-challenge"))
			}
			if state == idempotency.StateSucceeded {
				require.NoError(t, store.Succeed(context.Background(), principal, "same-client-key", fp, owner, json.RawMessage(`{"challenge_id":"private-legacy","debug_code":"private-code"}`)))
			}
			keys := rc.Keys(context.Background(), "otp:idem2:*").Val()
			require.Len(t, keys, 1)
			raw, err := mr.Get(keys[0])
			require.NoError(t, err)
			ttl := mr.TTL(keys[0])
			for i, keyID := range []string{"", "primary"} {
				resp := defaultKeyRequest(t, app, keyID, "legacy-"+strconv.Itoa(i))
				require.Equal(t, 409, resp.status)
				require.Contains(t, string(resp.body), "idempotency_namespace_conflict")
				require.NotContains(t, string(resp.body), "private-")
			}

			rotated := identityDefaultApp(t, rc, p, "replacement")
			response := identityRequest(t, rotated, "", "replacement-secret", "rotated-default")
			require.Equal(t, 409, response.status)
			require.Contains(t, string(response.body), "idempotency_namespace_conflict")
			require.NotContains(t, string(response.body), "private-")
			require.EqualValues(t, 0, p.calls)
			after, err := mr.Get(keys[0])
			require.NoError(t, err)
			require.Equal(t, raw, after)
			require.Equal(t, ttl, mr.TTL(keys[0]))
			require.Len(t, rc.Keys(context.Background(), "otp:idem2:*").Val(), 1)
		})
	}
}
func TestHMACMixedKeyFormsShareConcurrentAndCompletedRequests(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	app, p := identityApp(t, rc)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			keyID := ""
			if i%2 != 0 {
				keyID = "primary"
			}
			resp := defaultKeyRequest(t, app, keyID, "mixed-"+strconv.Itoa(i))
			if resp.status != 200 && resp.status != 409 {
				t.Errorf("unexpected status %d: %s", resp.status, resp.body)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, p.calls)
	replay := defaultKeyRequest(t, app, "", "completed")
	require.Equal(t, 200, replay.status)
	require.EqualValues(t, 1, p.calls)
}
func TestHMACKeyFormRetryAfterRestartDoesNotResend(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	rc.AddHook(&rejectStateHook{})
	first := identityProviderApp(t, rc, &faultProvider{rc: rc})
	require.Equal(t, 503, defaultKeyRequest(t, first, "", "initial").status)
	restarted := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = restarted.Close() })
	app := identityProviderApp(t, restarted, &faultProvider{rc: restarted})
	mr.FastForward(24 * time.Hour)
	retry := defaultKeyRequest(t, app, "primary", "restarted")
	require.Equal(t, 409, retry.status)
	require.Contains(t, string(retry.body), "delivery_unknown")
	count, err := restarted.Get(context.Background(), "test:delivered").Int()
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestHMACDefaultRotationCannotReplayAnotherCredentialResponse(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	firstProvider := &countingProvider{channel: provider.ChannelEmail}
	firstApp := identityDefaultApp(t, rc, firstProvider, "primary")
	original := defaultKeyRequest(t, firstApp, "", "before-rotation")
	require.Equal(t, 200, original.status)
	mr.FastForward(61 * time.Second)
	secondProvider := &countingProvider{channel: provider.ChannelEmail}
	rotated := identityDefaultApp(t, rc, secondProvider, "replacement")
	response := identityRequest(t, rotated, "", "replacement-secret", "after-rotation")
	require.Equal(t, 200, response.status)
	t.Logf("primary credential calls=%d replacement credential calls=%d replayed previous response=%v", firstProvider.calls, secondProvider.calls, bytes.Equal(original.body, response.body))
	require.NotEqual(t, string(original.body), string(response.body), "another verified key must not replay the previous credential's response")
	require.EqualValues(t, 1, firstProvider.calls)
	require.EqualValues(t, 1, secondProvider.calls)
}
