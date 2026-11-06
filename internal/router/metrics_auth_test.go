package router

import (
	"fmt"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestReaderPolicyAllowsVerificationWithoutBusinessSideEffects(t *testing.T) {
	defer withV2Config(t)()
	oldPolicy, oldAudit, oldTOTP, oldBase, oldEnv := config.ServicePoliciesJSON, config.AuditEnabled, config.TOTPEnabled, config.TOTPBaseURL, config.Env
	defer func() {
		config.ServicePoliciesJSON, config.AuditEnabled, config.TOTPEnabled, config.TOTPBaseURL, config.Env = oldPolicy, oldAudit, oldTOTP, oldBase, oldEnv
	}()
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { upstreamCalls.Add(1); w.WriteHeader(200) }))
	defer upstream.Close()
	config.ServicePoliciesJSON = `{"default":{"service":"reader","actions":["otp:verify"]}}`
	config.AuditEnabled, config.TOTPEnabled, config.TOTPBaseURL, config.Env = false, true, upstream.URL, config.EnvDevelopment
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rc.Close() }()
	rw := NewRouterWithClientAndHandlers(rc, testLogger())
	for i, tc := range []struct {
		path, body string
		want       int
		reason     string
	}{
		{"/v1/otp/verifications", `{}`, 400, "challenge_id_required"},
		{"/v1/otp/challenges", `{"user_id":"user","channel":"email","destination":"user@example.com","purpose":"login"}`, 403, "forbidden"},
		{"/v1/totp/revoke", `{"subject":"other-user"}`, 403, "forbidden"},
	} {
		req := httptest.NewRequest("POST", tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range v3Sign("v2-secret", "", "reader", "POST", tc.path, "", fmt.Sprintf("reader-%d", i), time.Now().Unix(), []byte(tc.body)) {
			req.Header.Set(k, v)
		}
		resp, err := rw.App.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != tc.want || !strings.Contains(string(body), tc.reason) {
			t.Fatalf("%s status=%d body=%s", tc.path, resp.StatusCode, body)
		}
	}
	if upstreamCalls.Load() != 0 {
		t.Fatalf("denied TOTP revoke reached upstream %d times", upstreamCalls.Load())
	}
	// Verified requests consume nonces, including action denials. No challenge,
	// limiter, audit, or idempotency records may be created by these requests.
	keys := mr.Keys()
	if len(keys) != 3 {
		t.Fatalf("expected exactly 3 consumed nonces, got %v", keys)
	}
	for _, key := range keys {
		if !strings.HasPrefix(key, config.NonceStorePrefix) {
			t.Fatalf("unexpected backend mutation: %s", key)
		}
	}
}
