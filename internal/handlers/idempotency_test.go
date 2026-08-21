package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
)

// countingProvider records how many times Send is invoked so we can assert
// exactly-once delivery under concurrent idempotent requests.
type countingProvider struct {
	channel provider.Channel
	calls   int64
}

func (p *countingProvider) Send(ctx context.Context, msg *provider.Message) (*provider.SendResult, error) {
	atomic.AddInt64(&p.calls, 1)
	return provider.NewSuccessResult("counting", p.channel, "msg-1"), nil
}
func (p *countingProvider) Channel() provider.Channel { return p.channel }
func (p *countingProvider) Name() string              { return "counting" }
func (p *countingProvider) Validate() error           { return nil }

func newIdemApp(t *testing.T) (*fiber.App, *countingProvider) {
	t.Helper()
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.ChallengeExpiry = 5 * time.Minute
	config.ResendCooldown = 60 * time.Second
	config.AllowedPurposes = []string{"login"}
	config.RateLimitPerUser = 100000
	config.RateLimitPerIP = 100000
	config.RateLimitPerDestination = 100000
	config.ProviderFailurePolicy = "strict"
	config.IdempotencySecret = "test-idem-secret"

	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatalf("NewHandlersWithError: %v", err)
	}
	cp := &countingProvider{channel: provider.ChannelEmail}
	// Replace the registry with one that only has our counting provider.
	reg := provider.NewRegistry()
	if err := reg.Register(cp); err != nil {
		t.Fatalf("register counting provider: %v", err)
	}
	h.providerRegistry = reg

	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app, cp
}

func idemRequest(app *fiber.App, key, body string) (*fiberTestResp, error) {
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		return nil, err
	}
	b, _ := io.ReadAll(resp.Body)
	return &fiberTestResp{status: resp.StatusCode, body: b}, nil
}

type fiberTestResp struct {
	status int
	body   []byte
}

// TestIdempotency_ConcurrentDuplicates_ExactlyOneProviderCall proves that N
// concurrent requests with the same idempotency key produce exactly one
// provider send.
func TestIdempotency_ConcurrentDuplicates_ExactlyOneProviderCall(t *testing.T) {
	app, cp := newIdemApp(t)
	body := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`
	const n = 40

	var wg sync.WaitGroup
	var okCount, conflictCount int64
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			resp, err := idemRequest(app, "dup-key-1", body)
			if err != nil {
				return
			}
			switch resp.status {
			case fiber.StatusOK:
				atomic.AddInt64(&okCount, 1)
			case fiber.StatusConflict:
				atomic.AddInt64(&conflictCount, 1)
			}
		}()
	}
	wg.Wait()

	calls := atomic.LoadInt64(&cp.calls)
	if calls != 1 {
		t.Fatalf("provider called %d times, want exactly 1", calls)
	}
	if okCount < 1 {
		t.Fatalf("expected at least one 200 OK, got %d (conflict=%d)", okCount, conflictCount)
	}
}

// TestIdempotency_SameKeyDifferentBody_Conflict proves a reused key with a
// different request body is rejected with 409.
func TestIdempotency_SameKeyDifferentBody_Conflict(t *testing.T) {
	app, cp := newIdemApp(t)
	first := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`
	second := `{"user_id":"u2","channel":"email","destination":"c@d.com","purpose":"login"}`

	r1, err := idemRequest(app, "conflict-key", first)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if r1.status != fiber.StatusOK {
		t.Fatalf("first status = %d, want 200, body=%s", r1.status, string(r1.body))
	}
	r2, err := idemRequest(app, "conflict-key", second)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if r2.status != fiber.StatusConflict {
		t.Fatalf("second status = %d, want 409, body=%s", r2.status, string(r2.body))
	}
	var out map[string]interface{}
	_ = json.Unmarshal(r2.body, &out)
	if out["reason"] != "idempotency_conflict" {
		t.Fatalf("conflict reason = %v, want idempotency_conflict", out["reason"])
	}
	if c := atomic.LoadInt64(&cp.calls); c != 1 {
		t.Fatalf("provider called %d times, want 1 (conflict must not send)", c)
	}
}

// TestIdempotency_DifferentPrincipalsIsolated proves that the same client key
// used by different principals does not collide (each sends once).
func TestIdempotency_DifferentPrincipalsIsolated(t *testing.T) {
	app, cp := newIdemApp(t)

	mk := func(service, body string) *fiberTestResp {
		req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "shared-key")
		req.Header.Set("X-Service", service)
		resp, _ := app.Test(req, 5000)
		b, _ := io.ReadAll(resp.Body)
		return &fiberTestResp{status: resp.StatusCode, body: b}
	}

	// Distinct destinations so the per-destination resend cooldown does not
	// interfere; the point is that the SAME idempotency key under DIFFERENT
	// principals must not collide.
	bodyA := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`
	bodyB := `{"user_id":"u2","channel":"email","destination":"c@d.com","purpose":"login"}`

	if r := mk("svc-a", bodyA); r.status != fiber.StatusOK {
		t.Fatalf("svc-a status = %d, want 200, body=%s", r.status, string(r.body))
	}
	if r := mk("svc-b", bodyB); r.status != fiber.StatusOK {
		t.Fatalf("svc-b status = %d, want 200 (different principal must not collide), body=%s", r.status, string(r.body))
	}
	if c := atomic.LoadInt64(&cp.calls); c != 2 {
		t.Fatalf("provider called %d times, want 2 (one per principal)", c)
	}
}

// TestIdempotency_ReplayReturnsSameResponse proves a duplicate request replays
// the stored response without another provider send.
func TestIdempotency_ReplayReturnsSameResponse(t *testing.T) {
	app, cp := newIdemApp(t)
	body := `{"user_id":"u1","channel":"email","destination":"a@b.com","purpose":"login"}`

	r1, _ := idemRequest(app, "replay-key", body)
	r2, _ := idemRequest(app, "replay-key", body)
	if r1.status != fiber.StatusOK || r2.status != fiber.StatusOK {
		t.Fatalf("statuses = %d,%d want 200,200", r1.status, r2.status)
	}
	if !bytes.Equal(r1.body, r2.body) {
		t.Fatalf("replay body differs:\n first=%s\nsecond=%s", string(r1.body), string(r2.body))
	}
	if c := atomic.LoadInt64(&cp.calls); c != 1 {
		t.Fatalf("provider called %d times, want 1 on replay", c)
	}
}
