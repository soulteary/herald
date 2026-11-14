package router

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

func TestSanitizeForwardedChain(t *testing.T) {
	app := fiber.New()
	app.Use(sanitizeTrustedForwardedIP("X-Forwarded-For", []string{"10.0.0.0/8", "127.0.0.1"}))
	app.Get("/", func(c fiber.Ctx) error { return c.JSON(fiber.Map{"ip": c.Get("X-Forwarded-For")}) })
	for _, tc := range []struct{ raw, want string }{
		{"", ""}, {"203.0.113.7, 10.0.0.2", "203.0.113.7"}, {"127.0.0.1, 10.0.0.2", ""}, {"not-an-ip, 10.0.0.2", ""},
	} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", tc.raw)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil || body["ip"] != tc.want {
			t.Fatalf("chain %q: %v err=%v", tc.raw, body, err)
		}
	}
}

func TestFrameworkErrorContract(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		reason string
	}{
		{fiber.ErrMethodNotAllowed, 405, "method_not_allowed"},
		{fiber.ErrRequestEntityTooLarge, 413, "payload_too_large"},
		{fiber.ErrBadRequest, 400, "request_error"},
		{errors.New("private backend detail"), 500, "internal_error"},
	} {
		app := fiber.New(fiber.Config{ErrorHandler: jsonErrorHandler})
		app.Get("/", func(fiber.Ctx) error { return tc.err })
		resp, err := app.Test(httptest.NewRequest("GET", "/", nil))
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != tc.status || body["reason"] != tc.reason {
			t.Fatalf("status=%d body=%v err=%v", resp.StatusCode, body, err)
		}
	}
}

func TestRouterExplicitCORSAndTracing(t *testing.T) {
	oldOrigins, oldTracing := config.CORSAllowOrigins, config.OTLPEnabled
	defer func() { config.CORSAllowOrigins, config.OTLPEnabled = oldOrigins, oldTracing }()
	config.CORSAllowOrigins = " https://allowed.example , https://other.example "
	config.OTLPEnabled = true
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	app := NewRouterWithClient(rc, testLogger())
	for _, origin := range []string{"https://allowed.example", "https://attacker.example"} {
		req := httptest.NewRequest("GET", "/livez", nil)
		req.Header.Set("Origin", origin)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		allowed := resp.Header.Get("Access-Control-Allow-Origin")
		if origin == "https://allowed.example" && allowed != origin {
			t.Fatalf("allowlisted origin rejected: %q", allowed)
		}
		if origin == "https://attacker.example" && allowed != "" {
			t.Fatalf("untrusted origin accepted: %q", allowed)
		}
	}
}

func TestRouterLegacyConstructorAndInitializationFailure(t *testing.T) {
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	oldURL, oldRedis, oldPassword, oldV1 := config.HeraldSMTPAPIURL, config.RedisAddr, config.RedisPassword, config.HMACV1Enabled
	defer func() {
		config.HeraldSMTPAPIURL, config.RedisAddr, config.RedisPassword, config.HMACV1Enabled = oldURL, oldRedis, oldPassword, oldV1
	}()
	config.RedisAddr, config.RedisPassword, config.HMACV1Enabled = rc.Options().Addr, "", true
	app := NewRouterWithClient(rc, testLogger())
	if app == nil {
		t.Fatal("legacy constructor returned nil")
	}
	config.HeraldSMTPAPIURL = "://bad"
	defer func() {
		if recover() == nil {
			t.Error("legacy wrapper did not panic on initialization failure")
		}
	}()
	NewRouterWithClientAndHandlers(rc, testLogger())
}
