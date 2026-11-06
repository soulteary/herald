package router

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

func TestMetricsAuthentication(t *testing.T) {
	for _, tc := range []struct {
		name, key, header, value string
		prod                     bool
		want                     int
	}{
		{name: "development anonymous", want: 204},
		{name: "production disabled", prod: true, want: 401},
		{name: "production missing", key: "metrics-only", prod: true, want: 401},
		{name: "wrong", key: "metrics-only", header: "X-Metrics-API-Key", value: "wrong", prod: true, want: 401},
		{name: "header", key: "metrics-only", header: "X-Metrics-API-Key", value: "metrics-only", prod: true, want: 204},
		{name: "bearer", key: "metrics-only", header: "Authorization", value: "Bearer metrics-only", prod: true, want: 204},
		{name: "business key denied", key: "metrics-only", header: "X-API-Key", value: "metrics-only", prod: true, want: 401},
		{name: "development configured", key: "metrics-only", want: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/metrics", metricsAuthMiddleware(tc.key, tc.prod), func(c fiber.Ctx) error { return c.SendStatus(204) })
			req := httptest.NewRequest("GET", "/metrics", nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.want {
				t.Fatalf("status=%d,want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestRouterRejectsPolicyBeforeInitializingHandlers(t *testing.T) {
	oldMode, oldPolicy := config.RequestAuthMode, config.ServicePoliciesJSON
	defer func() { config.RequestAuthMode, config.ServicePoliciesJSON = oldMode, oldPolicy }()
	for _, tc := range []struct{ mode, policy, want string }{
		{"typo", "", "REQUEST_AUTH_MODE"}, {"hmac_v2", "{", "HERALD_SERVICE_POLICIES"},
	} {
		config.RequestAuthMode, config.ServicePoliciesJSON = tc.mode, tc.policy
		_, err := NewRouterWithClientAndHandlersE(nil, testLogger())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("error=%v", err)
		}
	}
}

func TestRouterAppliesServicePoliciesToEveryProtectedRoute(t *testing.T) {
	defer withV2Config(t)()
	oldPolicy := config.ServicePoliciesJSON
	defer func() { config.ServicePoliciesJSON = oldPolicy }()
	config.ServicePoliciesJSON = `{"default":{"service":"gateway","actions":["totp:status"]}}`
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())
	for i, tc := range []struct {
		method, path string
		want         int
	}{
		{"POST", "/v1/otp/challenges", 403}, {"POST", "/v1/otp/verifications", 403},
		{"POST", "/v2/otp/verifications", 403}, {"POST", "/v1/otp/challenges/any/revoke", 403},
		{"GET", "/v1/totp/status", 503}, {"POST", "/v1/totp/verify", 403},
		{"POST", "/v1/totp/enroll/start", 403}, {"POST", "/v1/totp/enroll/confirm", 403}, {"POST", "/v1/totp/revoke", 403},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		for k, v := range v3Sign("v2-secret", "", "gateway", tc.method, tc.path, "", fmt.Sprintf("policy-%d", i), time.Now().Unix(), []byte(`{}`)) {
			req.Header.Set(k, v)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := rw.App.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s: status=%d,want %d", tc.path, resp.StatusCode, tc.want)
		}
	}
}
