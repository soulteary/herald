package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

const middlewareTestBody = `{"ok":true}`

func runMiddlewareRequest(t *testing.T, cfg Config, headers map[string]string) (int, string) {
	t.Helper()
	app := fiber.New()
	app.Use(New(cfg))
	app.Post("/resource", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodPost, "/resource?scope=write", strings.NewReader(middlewareTestBody))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var payload struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)
	return resp.StatusCode, payload.Reason
}

func signedHeaders(now time.Time, nonce, keyID, secret string) map[string]string {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	request := CanonicalRequest{
		Method: http.MethodPost, Path: "/resource", Query: "scope=write",
		Timestamp: timestamp, Nonce: nonce, Service: "caller", KeyID: keyID,
		Body: []byte(middlewareTestBody),
	}
	return map[string]string{
		HeaderSignatureVersion: SignatureVersionV2,
		HeaderSignature:        SignV2(secret, request),
		HeaderTimestamp:        timestamp,
		HeaderNonce:            nonce,
		HeaderService:          "caller",
		HeaderKeyID:            keyID,
	}
}

func TestParseMode(t *testing.T) {
	tests := map[string]Mode{
		"api_key": ModeAPIKey, " APIKEY ": ModeAPIKey,
		"none": ModeNone, "off": ModeNone, "DISABLED": ModeNone,
		"hmac_v2": ModeHMACV2, "unknown": ModeHMACV2,
	}
	for input, want := range tests {
		if got := ParseMode(input); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMiddleware_NoneAndAPIKeyModes(t *testing.T) {
	if status, _ := runMiddlewareRequest(t, Config{Mode: ModeNone}, nil); status != http.StatusNoContent {
		t.Fatalf("none mode status = %d, want %d", status, http.StatusNoContent)
	}

	tests := []struct {
		name    string
		cfg     Config
		headers map[string]string
		want    int
	}{
		{"x-api-key", Config{Mode: ModeAPIKey, APIKey: "secret"}, map[string]string{"X-API-Key": "secret"}, http.StatusNoContent},
		{"bearer", Config{Mode: ModeAPIKey, APIKey: "secret"}, map[string]string{"Authorization": "Bearer secret"}, http.StatusNoContent},
		{"missing", Config{Mode: ModeAPIKey, APIKey: "secret"}, nil, http.StatusUnauthorized},
		{"wrong", Config{Mode: ModeAPIKey, APIKey: "secret"}, map[string]string{"X-API-Key": "wrong"}, http.StatusUnauthorized},
		{"empty configured key", Config{Mode: ModeAPIKey}, map[string]string{"X-API-Key": "secret"}, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := runMiddlewareRequest(t, tt.cfg, tt.headers)
			if status != tt.want {
				t.Fatalf("status = %d, want %d", status, tt.want)
			}
		})
	}
}

func TestMiddleware_HMACV2SuccessReplayAndSignatureOrder(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	store := newNonceStore(t)
	cfg := Config{
		Mode: ModeHMACV2, MaxDrift: time.Minute, Clock: func() time.Time { return now },
		KeyProvider: func(id string) string {
			if id == "key-1" {
				return "secret"
			}
			return ""
		},
		NonceStore: store, FailClosed: true,
	}

	bad := signedHeaders(now, "nonce-order", "key-1", "wrong-secret")
	if status, reason := runMiddlewareRequest(t, cfg, bad); status != http.StatusUnauthorized || reason != "invalid_signature" {
		t.Fatalf("bad signature: status=%d reason=%q", status, reason)
	}
	valid := signedHeaders(now, "nonce-order", "key-1", "secret")
	if status, reason := runMiddlewareRequest(t, cfg, valid); status != http.StatusNoContent {
		t.Fatalf("valid request: status=%d reason=%q", status, reason)
	}
	if status, reason := runMiddlewareRequest(t, cfg, valid); status != http.StatusUnauthorized || reason != "replayed_nonce" {
		t.Fatalf("replay: status=%d reason=%q", status, reason)
	}
}

func TestMiddleware_HMACV2DefaultKeyID(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	headers := signedHeaders(now, "nonce-default", "", "secret")
	cfg := Config{
		Mode: ModeHMACV2, Clock: func() time.Time { return now }, DefaultKeyID: "default",
		KeyProvider: func(id string) string {
			if id == "default" {
				return "secret"
			}
			return ""
		},
		NonceStore: newNonceStore(t), FailClosed: true,
	}
	if status, reason := runMiddlewareRequest(t, cfg, headers); status != http.StatusNoContent {
		t.Fatalf("status=%d reason=%q", status, reason)
	}
}

func TestMiddleware_HMACV2Rejections(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	base := Config{
		Mode: ModeHMACV2, Clock: func() time.Time { return now }, MaxDrift: time.Minute,
		KeyProvider: func(id string) string {
			if id == "key-1" {
				return "secret"
			}
			return ""
		},
		NonceStore: newNonceStore(t), FailClosed: true,
	}
	valid := signedHeaders(now, "nonce-base", "key-1", "secret")
	clone := func() map[string]string {
		out := make(map[string]string, len(valid))
		for k, v := range valid {
			out[k] = v
		}
		return out
	}

	tests := []struct {
		name   string
		cfg    Config
		mutate func(map[string]string)
		reason string
	}{
		{"version required", base, func(h map[string]string) { delete(h, HeaderSignatureVersion) }, "signature_version_required"},
		{"missing headers", base, func(h map[string]string) { delete(h, HeaderNonce) }, "missing_auth_headers"},
		{"key id required", func() Config { c := base; c.DefaultKeyID = ""; return c }(), func(h map[string]string) { delete(h, HeaderKeyID) }, "key_id_required"},
		{"timestamp out of range", base, func(h map[string]string) { h[HeaderTimestamp] = "1" }, "timestamp_out_of_range"},
		{"nil provider", func() Config { c := base; c.KeyProvider = nil; return c }(), func(h map[string]string) {}, "unauthorized"},
		{"unknown key", base, func(h map[string]string) { h[HeaderKeyID] = "unknown" }, "invalid_key_id"},
		{"nil nonce store", func() Config { c := base; c.NonceStore = nil; return c }(), func(h map[string]string) {}, "unauthorized"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := clone()
			headers[HeaderNonce] = "nonce-reject-" + strconv.Itoa(i)
			// Re-sign after changing the nonce; individual mutations then target
			// the exact validation branch named by the test.
			headers = signedHeaders(now, headers[HeaderNonce], "key-1", "secret")
			tt.mutate(headers)
			status, reason := runMiddlewareRequest(t, tt.cfg, headers)
			if status != http.StatusUnauthorized || reason != tt.reason {
				t.Fatalf("status=%d reason=%q, want 401/%q", status, reason, tt.reason)
			}
		})
	}
}

func TestMiddleware_HMACV2NonceBackendPolicy(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	headers := signedHeaders(now, "nonce-backend", "key-1", "secret")
	base := Config{
		Mode: ModeHMACV2, Clock: func() time.Time { return now },
		KeyProvider: func(string) string { return "secret" },
		NonceStore:  NewNonceStore(nil, "", nil, 0),
	}

	closed := base
	closed.FailClosed = true
	if status, reason := runMiddlewareRequest(t, closed, headers); status != http.StatusServiceUnavailable || reason != "nonce_store_unavailable" {
		t.Fatalf("fail closed: status=%d reason=%q", status, reason)
	}
	if status, reason := runMiddlewareRequest(t, base, headers); status != http.StatusNoContent {
		t.Fatalf("fail open: status=%d reason=%q", status, reason)
	}
}

func TestMiddleware_HMACV1FallbackIsExplicit(t *testing.T) {
	cfg := Config{
		Mode: ModeHMACV2, V1Enabled: true,
		V1Handler: func(c fiber.Ctx) error { return c.SendStatus(http.StatusAccepted) },
	}
	if status, _ := runMiddlewareRequest(t, cfg, nil); status != http.StatusAccepted {
		t.Fatalf("status=%d, want %d", status, http.StatusAccepted)
	}
}
