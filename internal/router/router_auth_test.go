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

	"github.com/gofiber/fiber/v2"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

func TestAuthRequired_NoHeaders_Returns401(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = ""
	config.HMACSecret = "test-secret-for-auth-test"
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 without auth headers, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestAuthRequired_WrongSignature_Returns401(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = ""
	config.HMACSecret = "test-secret-for-auth-test"
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("X-Service", "test-service")
	req.Header.Set("X-Signature", "invalid-signature-hex")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 with wrong signature, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestAuthRequired_TimestampDrift_Returns401(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = ""
	config.HMACSecret = "test-secret-for-auth-test"
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	ts := strconv.FormatInt(time.Now().Add(-400*time.Second).Unix(), 10) // 400s ago
	sig := computeHMAC("test-secret-for-auth-test", ts, "test-service", body)

	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Service", "test-service")
	req.Header.Set("X-Signature", sig)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 when timestamp is too old, got %d body=%s", resp.StatusCode, string(b))
	}
}

func TestAuthRequired_ValidHMAC_ReachesHandler(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = ""
	config.HMACSecret = "test-secret-for-auth-test"
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := computeHMAC("test-secret-for-auth-test", ts, "test-service", body)

	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Timestamp", ts)
	req.Header.Set("X-Service", "test-service")
	req.Header.Set("X-Signature", sig)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// Auth passed; handler may return 200 or 4xx for business logic (e.g. rate limit, validation)
	// but must not return 401 once HMAC is valid
	if resp.StatusCode == fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected non-401 with valid HMAC (auth should pass), got 401 body=%s", string(b))
	}
}

func TestAuthRequired_ValidAPIKey_ReachesHandler(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = "test-api-key-auth"
	config.HMACSecret = ""
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-api-key-auth")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode == fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected non-401 with valid API key, got 401 body=%s", string(b))
	}
}

func TestAuthRequired_InvalidAPIKey_Returns401(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = "expected-key"
	config.HMACSecret = ""
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	app := rw.App

	body := []byte(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "wrong-key")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 with wrong API key, got %d body=%s", resp.StatusCode, string(b))
	}
}

func computeHMAC(secret, timestamp, service string, body []byte) string {
	msg := timestamp + ":" + service + ":" + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}
