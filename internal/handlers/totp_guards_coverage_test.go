package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
	"github.com/soulteary/herald/internal/security"
)

func totpGuardApp(h *Handlers) *fiber.App {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error { c.Locals("trace_context", context.Background()); return c.Next() })
	app.Get("/status", h.TOTPStatus)
	app.Post("/verify", h.TOTPVerify)
	app.Post("/enroll/start", h.TOTPEnrollStart)
	app.Post("/enroll/confirm", h.TOTPEnrollConfirm)
	app.Post("/revoke", h.TOTPRevoke)
	return app
}

func TestTOTPGuardsAndTraceContext(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"ok":true,"subject":"user"}`)
	}))
	defer upstream.Close()
	c, err := newTOTPProxyClient(heraldtotp.DefaultOptions().WithBaseURL(upstream.URL), 1024, false)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handlers{totpClient: c, log: testLogger(), digester: security.NewDigester([]byte("pepper"))}
	if got := h.maskSubject(""); got != "" {
		t.Fatal("empty subject was not preserved")
	}
	for _, endpoint := range totpSecurityEndpoints {
		path := strings.Split(endpoint.path, "?")[0]
		status, body := totpSecurityRequest(t, totpGuardApp(&Handlers{}), endpoint.method, path, endpoint.body)
		if status != 503 || !strings.Contains(body, "totp_not_configured") {
			t.Fatalf("disabled TOTP: %d %s", status, body)
		}
		status, body = totpSecurityRequest(t, totpGuardApp(h), endpoint.method, path, `{}`)
		if status != 400 {
			t.Fatalf("missing required fields accepted: %d %s", status, body)
		}
		if endpoint.method == http.MethodPost {
			status, body = totpSecurityRequest(t, totpGuardApp(h), endpoint.method, path, `{"unknown":"field"}`)
			if status != 400 {
				t.Fatalf("unknown field accepted: %d %s", status, body)
			}
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid request reached upstream")
	}
	for _, endpoint := range totpSecurityEndpoints {
		status, body := totpSecurityRequest(t, totpGuardApp(h), endpoint.method, endpoint.path, endpoint.body)
		if status != 200 {
			t.Fatalf("trace context request failed: %d %s", status, body)
		}
	}
	if calls.Load() != 5 {
		t.Fatalf("forwarded requests=%d", calls.Load())
	}
}
