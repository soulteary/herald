package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/serviceauth"
)

func TestAuthorizeService(t *testing.T) {
	policies := serviceauth.ServicePolicies{"reader": {Service: "gateway", Actions: []string{"otp:verify"}}}
	for _, tc := range []struct {
		name, service, key, action string
		identified, disabled       bool
		want                       int
	}{
		{name: "allowed", service: "gateway", key: "reader", action: "otp:verify", identified: true, want: 204},
		{name: "denied action", service: "gateway", key: "reader", action: "totp:revoke", identified: true, want: 403},
		{name: "wrong service", service: "other", key: "reader", action: "otp:verify", identified: true, want: 403},
		{name: "unknown key", service: "gateway", key: "other", action: "otp:verify", identified: true, want: 403},
		{name: "unauthenticated", action: "otp:verify", want: 403},
		{name: "legacy model", disabled: true, want: 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New()
			app.Use(func(c fiber.Ctx) error {
				if tc.identified {
					c.Locals(hmacIdentityKey{}, hmacIdentity{tc.service, tc.key})
				}
				return c.Next()
			})
			p := policies
			if tc.disabled {
				p = nil
			}
			app.Post("/", AuthorizeService(p, tc.action), func(c fiber.Ctx) error { return c.SendStatus(204) })
			resp, err := app.Test(httptest.NewRequest("POST", "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.want {
				t.Fatalf("got %d,want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestServiceBindingRejectsBeforeNonceConsumption(t *testing.T) {
	now := time.Now()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rc.Close() }()
	cfg := Config{Mode: ModeHMACV2, Clock: func() time.Time { return now }, KeyProvider: func(string) string { return "secret" }, ServicePolicies: serviceauth.ServicePolicies{"k": {Service: "other", Actions: []string{"otp:verify"}}}}
	cfg.NonceStore = NewNonceStore(rc, "nonce:", []byte("secret"), time.Minute)
	headers := signedHeaders(now, "nonce", "k", "secret")
	status, _ := runMiddlewareRequest(t, cfg, headers)
	if status != http.StatusForbidden {
		t.Fatalf("status=%d", status)
	}
	cfg.ServicePolicies = serviceauth.ServicePolicies{"other-key": {Service: "caller"}}
	status, _ = runMiddlewareRequest(t, cfg, headers)
	if status != http.StatusForbidden {
		t.Fatalf("unmapped key status=%d", status)
	}
	cfg.ServicePolicies = serviceauth.ServicePolicies{"k": {Service: "caller"}}
	// A rejected identity must not burn the nonce of the legitimate service.
	status, _ = runMiddlewareRequest(t, cfg, headers)
	if status != http.StatusNoContent {
		t.Fatalf("matching service status=%d", status)
	}
	status, reason := runMiddlewareRequest(t, Config{Mode: "typo"}, nil)
	if status != 401 || reason != "invalid_auth_mode" {
		t.Fatalf("mode status=%d reason=%s", status, reason)
	}
}
