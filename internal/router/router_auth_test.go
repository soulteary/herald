package router

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// v2Headers signs a request with the HMAC v2 canonical scheme and returns the
// headers to set. nonce must be unique per request.
func v2Sign(secret, keyID, service, method, path, query, nonce string, ts int64, body []byte) map[string]string {
	canon := auth.CanonicalRequest{
		Method:    method,
		Path:      path,
		Query:     query,
		Timestamp: strconv.FormatInt(ts, 10),
		Nonce:     nonce,
		Service:   service,
		KeyID:     keyID,
		Body:      body,
	}
	sig := auth.SignV2(secret, canon)
	return map[string]string{
		auth.HeaderSignatureVersion: auth.SignatureVersionV2,
		auth.HeaderSignature:        sig,
		auth.HeaderTimestamp:        strconv.FormatInt(ts, 10),
		auth.HeaderNonce:            nonce,
		auth.HeaderService:          service,
		auth.HeaderKeyID:            keyID,
	}
}

func withV2Config(t *testing.T) func() {
	t.Helper()
	origMode := config.RequestAuthMode
	origHMAC := config.HMACSecret
	origAPIKey := config.APIKey
	origDefaultKey := config.HMACDefaultKeyID
	origV1 := config.HMACV1Enabled
	config.RequestAuthMode = "hmac_v2"
	config.HMACSecret = "v2-secret"
	config.APIKey = ""
	config.HMACDefaultKeyID = ""
	config.HMACV1Enabled = false
	return func() {
		config.RequestAuthMode = origMode
		config.HMACSecret = origHMAC
		config.APIKey = origAPIKey
		config.HMACDefaultKeyID = origDefaultKey
		config.HMACV1Enabled = origV1
	}
}

func doReq(t *testing.T, app *fiber.App, headers map[string]string, body []byte) *fiberResp {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	b, _ := io.ReadAll(resp.Body)
	return &fiberResp{status: resp.StatusCode, body: b}
}

type fiberResp struct {
	status int
	body   []byte
}

func TestAuthV2_NoHeaders_Returns401(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	r := doReq(t, rw.App, nil, body)
	if r.status != fiber.StatusUnauthorized {
		t.Errorf("no headers status = %d, want 401, body=%s", r.status, string(r.body))
	}
}

func TestAuthV2_ValidSignature_ReachesHandler(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	h := v2Sign("v2-secret", "", "svc", "POST", "/v1/otp/challenges", "", "nonce-1", time.Now().Unix(), body)
	r := doReq(t, rw.App, h, body)
	if r.status == fiber.StatusUnauthorized {
		t.Errorf("valid v2 signature should pass auth, got 401 body=%s", string(r.body))
	}
}

func TestAuthV2_TamperedBody_Returns401(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	h := v2Sign("v2-secret", "", "svc", "POST", "/v1/otp/challenges", "", "nonce-tamper", time.Now().Unix(), body)
	tampered := []byte(`{"user_id":"attacker","channel":"email","destination":"a@b.com","purpose":"login"}`)
	r := doReq(t, rw.App, h, tampered)
	if r.status != fiber.StatusUnauthorized {
		t.Errorf("tampered body status = %d, want 401", r.status)
	}
}

func TestAuthV2_ReplayedNonce_Returns401(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	ts := time.Now().Unix()
	h := v2Sign("v2-secret", "", "svc", "POST", "/v1/otp/challenges", "", "nonce-replay", ts, body)

	first := doReq(t, rw.App, h, body)
	if first.status == fiber.StatusUnauthorized {
		t.Fatalf("first request should pass auth, got 401 body=%s", string(first.body))
	}
	second := doReq(t, rw.App, h, body)
	if second.status != fiber.StatusUnauthorized {
		t.Errorf("replayed nonce status = %d, want 401 body=%s", second.status, string(second.body))
	}
}

func TestAuthV2_TimestampDrift_Returns401(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	old := time.Now().Add(-10 * time.Minute).Unix()
	h := v2Sign("v2-secret", "", "svc", "POST", "/v1/otp/challenges", "", "nonce-drift", old, body)
	r := doReq(t, rw.App, h, body)
	if r.status != fiber.StatusUnauthorized {
		t.Errorf("stale timestamp status = %d, want 401", r.status)
	}
}

// TestAuthV2_InvalidHMAC_NoAPIKeyDowngrade proves that when both an API key is
// configured and v2 mode is active, an invalid v2 signature is NOT allowed to
// fall back to API-key auth.
func TestAuthV2_InvalidHMAC_NoAPIKeyDowngrade(t *testing.T) {
	defer withV2Config(t)()
	config.APIKey = "some-api-key"
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	h := v2Sign("v2-secret", "", "svc", "POST", "/v1/otp/challenges", "", "nonce-nodown", time.Now().Unix(), body)
	h[auth.HeaderSignature] = "deadbeef" // corrupt signature
	// Also present a valid API key; must still be rejected.
	h["X-API-Key"] = "some-api-key"
	r := doReq(t, rw.App, h, body)
	if r.status != fiber.StatusUnauthorized {
		t.Errorf("invalid v2 + valid API key status = %d, want 401 (no downgrade)", r.status)
	}
}

// TestAuthV2_V1Rejected_WhenNotEnabled proves that a legacy v1-style request
// (no X-Signature-Version) is rejected when HMAC_V1_ENABLED is false.
func TestAuthV2_V1Rejected_WhenNotEnabled(t *testing.T) {
	defer withV2Config(t)()
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeHMAC("v2-secret", ts, "svc", body) // v1 signature
	h := map[string]string{
		"X-Timestamp": ts,
		"X-Service":   "svc",
		"X-Signature": sig,
	}
	r := doReq(t, rw.App, h, body)
	if r.status != fiber.StatusUnauthorized {
		t.Errorf("v1 request without HMAC_V1_ENABLED status = %d, want 401", r.status)
	}
}

// TestAuthV1_Enabled_ReachesHandler proves the legacy v1 scheme works when
// explicitly enabled for the migration cycle.
func TestAuthV1_Enabled_ReachesHandler(t *testing.T) {
	defer withV2Config(t)()
	config.HMACV1Enabled = true
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeHMAC("v2-secret", ts, "svc", body)
	h := map[string]string{
		"X-Timestamp": ts,
		"X-Service":   "svc",
		"X-Signature": sig,
	}
	r := doReq(t, rw.App, h, body)
	if r.status == fiber.StatusUnauthorized {
		t.Errorf("v1 with HMAC_V1_ENABLED should pass auth, got 401 body=%s", string(r.body))
	}
}

// TestAuthAPIKeyMode reaches the handler only in api_key mode with the right key.
func TestAuthAPIKeyMode_ValidKey(t *testing.T) {
	defer withV2Config(t)()
	config.RequestAuthMode = "api_key"
	config.APIKey = "the-key"
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	if r := doReq(t, rw.App, map[string]string{"X-API-Key": "the-key"}, body); r.status == fiber.StatusUnauthorized {
		t.Errorf("valid api key should pass, got 401 body=%s", string(r.body))
	}
	if r := doReq(t, rw.App, map[string]string{"X-API-Key": "wrong"}, body); r.status != fiber.StatusUnauthorized {
		t.Errorf("wrong api key status = %d, want 401", r.status)
	}
}

func computeHMAC(secret, timestamp, service string, body []byte) string {
	// v1 canonical: timestamp:service:body (middleware-kit ComputeHMAC).
	msg := timestamp + ":" + service + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
