package router

import (
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"
	logger "github.com/soulteary/logger-kit/v2"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

var (
	testLoggerOnce     sync.Once
	testLoggerInstance *logger.Logger
)

// testLogger returns a shared logger for testing. logger.New configures
// zerolog package globals, so constructing a new logger while the asynchronous
// audit writer is emitting an event causes a data race under go test -race.
func testLogger() *logger.Logger {
	testLoggerOnce.Do(func() {
		testLoggerInstance = logger.New(logger.Config{
			Level:  logger.ErrorLevel, // Only log errors during tests
			Format: logger.FormatJSON,
		})
	})
	return testLoggerInstance
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

	// Test router construction a second time to ensure it is idempotent.
	app2 := NewRouterWithClient(redisClient, testLogger())
	if app2 == nil {
		t.Fatal("NewRouterWithClient() returned nil")
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

func TestNewRouterWithClientAndHandlersE_ReturnsProviderInitError(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	originalURL := config.HeraldSMTPAPIURL
	defer func() { config.HeraldSMTPAPIURL = originalURL }()
	config.HeraldSMTPAPIURL = "://invalid-provider-url"

	if _, err := NewRouterWithClientAndHandlersE(redisClient, testLogger()); err == nil {
		t.Fatal("expected configured provider initialization error")
	}
}

func TestRouter_FrameworkErrorsAreJSON(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	app := NewRouterWithClient(redisClient, testLogger())
	resp, err := app.Test(httptest.NewRequest("GET", "/does-not-exist", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	if !strings.Contains(string(body), `"reason":"not_found"`) {
		t.Fatalf("body = %s, want stable not_found reason", body)
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

func TestLivez_AlwaysReturns200(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	resp, err := rw.App.Test(httptest.NewRequest("GET", "/livez", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("GET /livez status = %d, want 200", resp.StatusCode)
	}
}

func TestReadyz_OkWhenRedisReachable(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	resp, err := rw.App.Test(httptest.NewRequest("GET", "/readyz", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /readyz status = %d, want 200, body=%s", resp.StatusCode, string(body))
	}
}

func TestReadyz_FailsClosedWhenRedisDown(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	// Close the client so Ping fails; readyz must report 503, not 200.
	_ = redisClient.Close()

	resp, err := rw.App.Test(httptest.NewRequest("GET", "/readyz", nil))
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /readyz (redis down) status = %d, want 503, body=%s", resp.StatusCode, string(body))
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
	originalEnv := config.Env
	originalTestKey := config.TestAPIKey
	defer func() {
		config.TestMode = originalTestMode
		config.Env = originalEnv
		config.TestAPIKey = originalTestKey
	}()
	// The test-code endpoint is only mounted under the combined
	// test-environment + test-mode switch AND requires a test API key.
	config.Env = config.EnvTest
	config.TestMode = true
	config.TestAPIKey = "test-secret-key"

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	if rw.TestApp == nil {
		t.Fatal("expected a dedicated test app")
	}

	// The public app must never expose the test-code route.
	publicReq := httptest.NewRequest("GET", "/v1/test/code/some-id", nil)
	publicReq.Header.Set("X-Test-Api-Key", "test-secret-key")
	publicResp, err := rw.App.Test(publicReq)
	if err != nil {
		t.Fatalf("public app.Test: %v", err)
	}
	if publicResp.StatusCode != fiber.StatusNotFound {
		t.Errorf("public test-code route status = %d, want 404", publicResp.StatusCode)
	}

	// Without the test key the endpoint must reject with 401.
	reqNoKey := httptest.NewRequest("GET", "/v1/test/code/some-id", nil)
	respNoKey, err := rw.TestApp.Test(reqNoKey)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if respNoKey.StatusCode != fiber.StatusUnauthorized {
		body, _ := io.ReadAll(respNoKey.Body)
		t.Errorf("GET /v1/test/code/:id without test key status = %d, want 401, body=%s", respNoKey.StatusCode, string(body))
	}

	// With the test key it is reachable (200 or 404 depending on code presence).
	req := httptest.NewRequest("GET", "/v1/test/code/some-id", nil)
	req.Header.Set("X-Test-Api-Key", "test-secret-key")
	resp, err := rw.TestApp.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK && resp.StatusCode != fiber.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /v1/test/code/:id with test key status = %d, body=%s", resp.StatusCode, string(body))
	}
}

// TestRouter_OversizedBodyRejected verifies the Fiber BodyLimit rejects a
// request body larger than the configured cap with 413.
func TestRouter_OversizedBodyRejected(t *testing.T) {
	origLimit := config.MaxBodyBytes
	defer func() { config.MaxBodyBytes = origLimit }()
	config.MaxBodyBytes = 1024 // 1 KiB cap for the test

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())

	big := strings.Repeat("a", 4096)
	body := `{"user_id":"` + big + `","channel":"email","destination":"a@b.com","purpose":"login"}`
	req := httptest.NewRequest("POST", "/v1/otp/challenges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusRequestEntityTooLarge {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("oversized body status = %d, want 413, body=%s", resp.StatusCode, string(b))
	}
}

// TestRouter_CORSDisabledByDefault verifies that with no allowlist configured,
// no Access-Control-Allow-Origin header is emitted.
func TestRouter_CORSDisabledByDefault(t *testing.T) {
	origCORS := config.CORSAllowOrigins
	defer func() { config.CORSAllowOrigins = origCORS }()
	config.CORSAllowOrigins = ""

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS disabled but Access-Control-Allow-Origin = %q", got)
	}
}

func TestRouter_TestCodeRoute_NotMountedInDevOrProd(t *testing.T) {
	originalTestMode := config.TestMode
	originalEnv := config.Env
	originalTestKey := config.TestAPIKey
	defer func() {
		config.TestMode = originalTestMode
		config.Env = originalEnv
		config.TestAPIKey = originalTestKey
	}()
	config.Env = config.EnvDevelopment
	config.TestMode = true // set, but env is development
	config.TestAPIKey = "test-secret-key"

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()

	rw := NewRouterWithClientAndHandlers(redisClient, testLogger())
	if rw.TestApp != nil {
		t.Fatal("test app must not be created outside the test environment")
	}
	req := httptest.NewRequest("GET", "/v1/test/code/some-id", nil)
	req.Header.Set("X-Test-Api-Key", "test-secret-key")
	resp, err := rw.App.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	// Route should not be mounted -> 404 (route not found), never 200 with a code.
	if resp.StatusCode != fiber.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GET /v1/test/code/:id in development status = %d, want 404, body=%s", resp.StatusCode, string(body))
	}
}

func TestTrustedForwardedClientIPRejectsSpoofedPrefix(t *testing.T) {
	trusted := []string{"10.0.0.0/8", "192.168.1.10"}
	got, ok := trustedForwardedClientIP(
		"203.0.113.250, 198.51.100.7, 10.0.0.1, 192.168.1.10",
		trusted,
	)
	if !ok || got != "198.51.100.7" {
		t.Fatalf("trusted client IP = %q, %v; want 198.51.100.7, true", got, ok)
	}
}

func TestTrustedForwardedClientIPRejectsMalformedChain(t *testing.T) {
	if got, ok := trustedForwardedClientIP("198.51.100.7, not-an-ip", []string{"10.0.0.0/8"}); ok {
		t.Fatalf("malformed chain resolved to %q; want rejection", got)
	}
}

func TestRouter_OversizedBodyReturnsStableJSON(t *testing.T) {
	const limit = 8
	if got, want := transportBodyLimit(), int(^uint(0)>>1); got != want {
		t.Fatalf("transport body limit = %d, want platform maximum %d", got, want)
	}
	app := fiber.New(fiber.Config{
		BodyLimit:         transportBodyLimit(),
		StreamRequestBody: true,
		ErrorHandler:      jsonErrorHandler,
	})
	app.Use(stableBodyLimitMiddleware(limit))
	app.Post("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("POST", "/", strings.NewReader("0123456789abcdef"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", got)
	}
	if !strings.Contains(string(body), `"reason":"payload_too_large"`) {
		t.Fatalf("body = %s, want stable payload_too_large reason", body)
	}
}
