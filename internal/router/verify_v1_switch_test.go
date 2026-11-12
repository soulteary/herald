package router

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// TestVerifyV1RouteSwitch proves HERALD_VERIFY_V1_ENABLED controls only the v1
// verification route: enabled (the default) reaches the handler, disabled
// returns the router's 404, and v2 verification and v1 creation stay mounted.
func TestVerifyV1RouteSwitch(t *testing.T) {
	if !config.VerifyV1Enabled {
		t.Fatal("HERALD_VERIFY_V1_ENABLED must default to true")
	}

	oldEnabled, oldMode, oldEnv := config.VerifyV1Enabled, config.RequestAuthMode, config.Env
	defer func() { config.VerifyV1Enabled, config.RequestAuthMode, config.Env = oldEnabled, oldMode, oldEnv }()
	config.RequestAuthMode, config.Env = "none", config.EnvDevelopment

	post := func(t *testing.T, app *fiber.App, path string) (int, string) {
		t.Helper()
		// An empty JSON object passes auth (mode none) and fails handler
		// validation, so a mounted route answers 400 and never 404.
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	for _, enabled := range []bool{true, false} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			redisClient, _ := testutil.NewTestRedisClient()
			defer func() { _ = redisClient.Close() }()
			config.VerifyV1Enabled = enabled
			app := NewRouterWithClient(redisClient, testLogger())

			status, body := post(t, app, "/v1/otp/verifications")
			if enabled {
				if status != fiber.StatusBadRequest {
					t.Fatalf("v1 verification status = %d, want 400 from the handler, body=%s", status, body)
				}
			} else if status != fiber.StatusNotFound || !strings.Contains(body, `"reason":"not_found"`) {
				t.Fatalf("v1 verification status = %d, want the router 404, body=%s", status, body)
			}

			for _, path := range []string{"/v2/otp/verifications", "/v1/otp/challenges"} {
				if status, body := post(t, app, path); status != fiber.StatusBadRequest {
					t.Fatalf("%s status = %d, want 400 from the handler, body=%s", path, status, body)
				}
			}
		})
	}
}
