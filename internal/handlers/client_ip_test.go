package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
)

// okProvider always succeeds.
type okProvider struct{ channel provider.Channel }

func (p *okProvider) Send(ctx context.Context, msg *provider.Message) (*provider.SendResult, error) {
	return provider.NewSuccessResult("ok", p.channel, "msg-1"), nil
}
func (p *okProvider) Channel() provider.Channel { return p.channel }
func (p *okProvider) Name() string              { return "ok" }
func (p *okProvider) Validate() error           { return nil }

func newClientIPApp(t *testing.T, perIP int) *fiber.App {
	t.Helper()
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.ChallengeExpiry = 5 * time.Minute
	config.ResendCooldown = 60 * time.Second
	config.AllowedPurposes = []string{"login"}
	config.RateLimitPerUser = 100000
	config.RateLimitPerIP = perIP
	config.RateLimitPerDestination = 100000
	config.ProviderFailurePolicy = "strict"
	config.IdempotencySecret = "test-idem-secret"
	config.PIIPepper = "test-pepper"

	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatalf("NewHandlersWithError: %v", err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(&okProvider{channel: provider.ChannelEmail}); err != nil {
		t.Fatalf("register: %v", err)
	}
	h.providerRegistry = reg

	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app
}

func postChallengeIP(app *fiber.App, body string) (int, []byte) {
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 5000)
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// TestClientIPTrustSplit_ReportedIPCannotEvadeRateLimit proves the per-IP rate
// limit is keyed on the trusted peer IP: a caller cannot get a fresh bucket by
// rotating the self-reported client_ip field. With RateLimitPerIP=1, the second
// request from the same peer must be rejected even though it reports a
// different client_ip.
func TestClientIPTrustSplit_ReportedIPCannotEvadeRateLimit(t *testing.T) {
	app := newClientIPApp(t, 1)

	// Distinct destinations/users so only the per-IP limit can trip.
	status1, body1 := postChallengeIP(app, `{"user_id":"u1","channel":"email","destination":"a@example.com","purpose":"login","client_ip":"1.1.1.1"}`)
	if status1 != fiber.StatusOK {
		t.Fatalf("first request should succeed, got %d: %s", status1, body1)
	}

	status2, body2 := postChallengeIP(app, `{"user_id":"u2","channel":"email","destination":"b@example.com","purpose":"login","client_ip":"2.2.2.2"}`)
	if status2 != fiber.StatusTooManyRequests {
		t.Fatalf("second request (same peer, different reported client_ip) must be rate-limited, got %d: %s", status2, body2)
	}
}

// TestClientIPTrustSplit_HighLimitAllowsBoth is a control: with a high per-IP
// limit both requests succeed, confirming the previous test's rejection is due
// to the limit and not something else.
func TestClientIPTrustSplit_HighLimitAllowsBoth(t *testing.T) {
	app := newClientIPApp(t, 100000)

	status1, body1 := postChallengeIP(app, `{"user_id":"u1","channel":"email","destination":"a@example.com","purpose":"login","client_ip":"1.1.1.1"}`)
	if status1 != fiber.StatusOK {
		t.Fatalf("first request should succeed, got %d: %s", status1, body1)
	}
	status2, body2 := postChallengeIP(app, `{"user_id":"u2","channel":"email","destination":"b@example.com","purpose":"login","client_ip":"2.2.2.2"}`)
	if status2 != fiber.StatusOK {
		t.Fatalf("second request should succeed under high limit, got %d: %s", status2, body2)
	}
}
