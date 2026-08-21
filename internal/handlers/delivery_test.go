package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
)

// failingProvider always reports a failed send so we can exercise the
// strict-vs-soft PROVIDER_FAILURE_POLICY branches.
type failingProvider struct{ channel provider.Channel }

func (p *failingProvider) Send(ctx context.Context, msg *provider.Message) (*provider.SendResult, error) {
	return provider.NewFailureResult("failing", p.channel, provider.NewProviderError(provider.ReasonSendFailed, "boom")), nil
}
func (p *failingProvider) Channel() provider.Channel { return p.channel }
func (p *failingProvider) Name() string              { return "failing" }
func (p *failingProvider) Validate() error           { return nil }

func newDeliveryApp(t *testing.T, policy string) *fiber.App {
	t.Helper()
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.ChallengeExpiry = 5 * time.Minute
	config.ResendCooldown = 60 * time.Second
	config.AllowedPurposes = []string{"login"}
	config.RateLimitPerUser = 100000
	config.RateLimitPerIP = 100000
	config.RateLimitPerDestination = 100000
	config.ProviderFailurePolicy = policy
	config.IdempotencySecret = "test-idem-secret"
	config.Env = config.EnvDevelopment

	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatalf("NewHandlersWithError: %v", err)
	}
	reg := provider.NewRegistry()
	if err := reg.Register(&failingProvider{channel: provider.ChannelEmail}); err != nil {
		t.Fatalf("register failing provider: %v", err)
	}
	h.providerRegistry = reg

	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app
}

func postChallenge(app *fiber.App, body string) *fiberTestResp {
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req, 5000)
	b, _ := io.ReadAll(resp.Body)
	return &fiberTestResp{status: resp.StatusCode, body: b}
}

// TestDelivery_StrictRevokesOnProviderFailure proves that in strict mode a
// failed provider send returns an error to the caller (challenge revoked, not
// left redeemable).
func TestDelivery_StrictRevokesOnProviderFailure(t *testing.T) {
	app := newDeliveryApp(t, "strict")
	body := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`
	r := postChallenge(app, body)
	if r.status != fiber.StatusInternalServerError {
		t.Fatalf("strict status = %d, want 500, body=%s", r.status, string(r.body))
	}
	var out map[string]interface{}
	_ = json.Unmarshal(r.body, &out)
	if out["reason"] != "send_failed" {
		t.Fatalf("strict reason = %v, want send_failed", out["reason"])
	}
}

// TestDelivery_SoftFlagsDeliveryFailed proves that in soft mode (dev/degraded)
// the challenge is still created but the response carries a delivery_status
// signal so the caller knows the code may not have been delivered.
func TestDelivery_SoftFlagsDeliveryFailed(t *testing.T) {
	app := newDeliveryApp(t, "soft")
	body := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`
	r := postChallenge(app, body)
	if r.status != fiber.StatusOK {
		t.Fatalf("soft status = %d, want 200, body=%s", r.status, string(r.body))
	}
	var out map[string]interface{}
	_ = json.Unmarshal(r.body, &out)
	if out["delivery_status"] != "failed" {
		t.Fatalf("soft delivery_status = %v, want failed (body=%s)", out["delivery_status"], string(r.body))
	}
}
