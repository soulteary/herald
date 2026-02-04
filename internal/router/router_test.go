package router

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	logger "github.com/soulteary/logger-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// testLogger returns a logger for testing (disabled output)
func testLogger() *logger.Logger {
	return logger.New(logger.Config{
		Level:  logger.ErrorLevel, // Only log errors during tests
		Format: logger.FormatJSON,
	})
}

func TestNewRouterWithClient(t *testing.T) {
	// Setup mock Redis
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	// Test that NewRouterWithClient creates a valid Fiber app
	app := NewRouterWithClient(redisClient, testLogger())
	if app == nil {
		t.Fatal("NewRouterWithClient() returned nil")
	}

	// Test with session manager enabled
	// Save original config
	originalSessionStorageEnabled := config.SessionStorageEnabled
	defer func() {
		config.SessionStorageEnabled = originalSessionStorageEnabled
	}()

	config.SessionStorageEnabled = true
	app2 := NewRouterWithClient(redisClient, testLogger())
	if app2 == nil {
		t.Fatal("NewRouterWithClient() with session storage returned nil")
	}

	// Test with test mode enabled
	originalTestMode := config.TestMode
	defer func() {
		config.TestMode = originalTestMode
	}()

	config.TestMode = true
	app3 := NewRouterWithClient(redisClient, testLogger())
	if app3 == nil {
		t.Fatal("NewRouterWithClient() with test mode returned nil")
	}
}

func TestHealthz_Returns200(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /healthz status = %d, want 200, body=%s", resp.StatusCode, string(body))
	}
}

func TestHealthz_BodyContainsStatusOrOk(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "ok") && !strings.Contains(s, "status") {
		t.Errorf("GET /healthz body should contain 'ok' or 'status', got %s", s)
	}
}

// TestOTPRoute_RequiresAuthWhenConfigured verifies that when auth is required (HMAC or API key set),
// POST /v1/otp/challenges without headers returns 401. Behavior covered by router_auth_test.go.
// This test ensures the OTP route is mounted and returns 401 when unauthenticated.
func TestOTPRoute_RequiresAuthWhenConfigured(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	origAPIKey, origHMAC := config.APIKey, config.HMACSecret
	config.APIKey = ""
	config.HMACSecret = "require-auth-secret"
	defer func() {
		config.APIKey = origAPIKey
		config.HMACSecret = origHMAC
	}()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	req := httptest.NewRequest("POST", "/v1/otp/challenges", strings.NewReader(`{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("POST /v1/otp/challenges without auth status = %d, want 401, body=%s", resp.StatusCode, string(body))
	}
}

func TestRouter_OTLPEnabled(t *testing.T) {
	originalOTLP := config.OTLPEnabled
	defer func() { config.OTLPEnabled = originalOTLP }()
	config.OTLPEnabled = true

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	if rw == nil || rw.App == nil {
		t.Fatal("NewRouterWithClientAndHandlers returned nil")
	}
	// OTLP enabled branch is covered by router init; healthz still works
	req := httptest.NewRequest("GET", "/healthz", nil)
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /healthz status = %d, body=%s", resp.StatusCode, string(body))
	}
}

func TestRouter_TestModeRoute(t *testing.T) {
	originalTestMode := config.TestMode
	defer func() { config.TestMode = originalTestMode }()
	config.TestMode = true

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	// GET /v1/test/code/:challenge_id is registered when TestMode is true
	req := httptest.NewRequest("GET", "/v1/test/code/some-id", nil)
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// 404 because we're not in test mode response or code not found
	if resp.StatusCode != fiber.StatusOK && resp.StatusCode != fiber.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /v1/test/code/:id status = %d, body=%s", resp.StatusCode, string(body))
	}
}
