package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	audit "github.com/soulteary/audit-kit/v2"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/auditlog"
	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
)

func TestMain(m *testing.M) {
	config.AuditEnabled = false
	os.Exit(m.Run())
}

// Isolate the singleton audit lifecycle from ordinary OTP unit fixtures.
func TestAuditUnavailableRejectsBusinessSuccess(t *testing.T) {
	if os.Getenv("HERALD_AUDIT_REGRESSION") != "1" {
		for _, kind := range []string{"capacity", "healthy"} {
			t.Run(kind, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAuditUnavailableRejectsBusinessSuccess$")
				cmd.Env = append(os.Environ(), "HERALD_AUDIT_REGRESSION=1", "HERALD_AUDIT_CASE="+kind)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("audit subprocess: %v\n%s", err, out)
				}
			})
		}
		return
	}
	healthy := os.Getenv("HERALD_AUDIT_CASE") == "healthy"

	config.AuditEnabled = true
	config.AuditSpoolDir = t.TempDir()
	config.AuditSpoolMaxBytes = 1
	if healthy {
		config.AuditSpoolMaxBytes = 1 << 20
	}
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	t.Cleanup(func() { _ = h.StopAuditWriter() })
	p := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	app.Post("/v1/otp/challenges/:id/revoke", h.RevokeChallenge)
	resp, err := idemRequest(app, "audit-key", faultBody)
	require.NoError(t, err)
	if healthy {
		if resp.status != 200 || p.calls != 1 {
			t.Fatalf("healthy create: status=%d calls=%d body=%s", resp.status, p.calls, resp.body)
		}
	} else if resp.status != 503 || !strings.Contains(string(resp.body), "audit_unavailable") || p.calls != 0 {
		t.Fatalf("create: status=%d calls=%d body=%s", resp.status, p.calls, resp.body)
	}

	checkAuditVerificationAndRevoke(t, app, h, healthy)

	if healthy {
		waitForAuditDelivery(t)
	}

}

// Move the spool out of reach only after the initial creation audit succeeded.
type auditFailureProvider struct {
	failingProvider
	spool       string
	mu          sync.Mutex
	challengeID string
	calls       int
}

func (p *auditFailureProvider) Send(ctx context.Context, msg *provider.Message) (*provider.SendResult, error) {
	p.mu.Lock()
	p.challengeID = msg.IdempotencyKey
	p.calls++
	p.mu.Unlock()
	if err := os.Rename(p.spool, p.spool+"-offline"); err != nil {
		return nil, err
	}
	return p.failingProvider.Send(ctx, msg)
}

func TestStrictRevocationSurvivesAuditFailure(t *testing.T) {
	if os.Getenv("HERALD_AUDIT_REVOKE_REGRESSION") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStrictRevocationSurvivesAuditFailure$")
		cmd.Env = append(os.Environ(), "HERALD_AUDIT_REVOKE_REGRESSION=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("strict audit subprocess: %v\n%s", err, out)
		}
		return
	}
	config.AuditEnabled = true
	config.AuditSpoolDir = t.TempDir()
	config.AuditSpoolMaxBytes = 1 << 20
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	t.Cleanup(func() { _ = h.StopAuditWriter() })
	t.Cleanup(func() {
		if _, err := os.Stat(config.AuditSpoolDir + "-offline"); err == nil {
			_ = os.Rename(config.AuditSpoolDir+"-offline", config.AuditSpoolDir)
		}
	})
	p := &auditFailureProvider{failingProvider: failingProvider{channel: provider.ChannelEmail}, spool: config.AuditSpoolDir}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	r, err := idemRequest(app, "strict-audit-failure", faultBody)
	require.NoError(t, err)
	if r.status != 503 || !strings.Contains(string(r.body), "audit_unavailable") {
		t.Fatalf("status=%d body=%s", r.status, r.body)
	}
	p.mu.Lock()
	id := p.challengeID
	p.mu.Unlock()
	if id == "" {
		t.Fatal("provider was not called after the initial audit")
	}
	exists, err := rc.Exists(context.Background(), "otp:ch:"+id).Result()
	require.NoError(t, err)
	if exists != 0 {
		t.Fatalf("strict-policy challenge %s survived failed result auditing", id)
	}
	require.NoError(t, os.Rename(config.AuditSpoolDir+"-offline", config.AuditSpoolDir))
	retry, err := idemRequest(app, "strict-audit-failure", faultBody)
	require.NoError(t, err)
	if retry.status != 409 || !strings.Contains(string(retry.body), "delivery_unknown") {
		t.Fatalf("retry status=%d body=%s", retry.status, retry.body)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.calls != 1 {
		t.Fatalf("provider called %d times", p.calls)
	}
}

func checkAuditVerificationAndRevoke(t *testing.T, app *fiber.App, h *Handlers, healthy bool) {
	t.Helper()
	for _, path := range []string{"/v1/otp/verifications", "/v2/otp/verifications", "revoke"} {
		ch, code, err := h.challengeManager.Create(context.Background(), challengekit.CreateRequest{UserID: "u", Channel: challengekit.ChannelEmail, Destination: "a@b.com", Purpose: "login"})
		require.NoError(t, err)
		body := map[string]string{"challenge_id": ch.ID, "code": code}
		if strings.HasPrefix(path, "/v2") {
			body["expected_user_id"] = "u"
			body["expected_channel"] = "email"
			body["expected_purpose"] = "login"
		}
		if path == "revoke" {
			path = "/v1/otp/challenges/" + ch.ID + "/revoke"
		}
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest("POST", path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		r, err := app.Test(req)
		require.NoError(t, err)
		_ = r.Body.Close()
		want := 503
		if healthy {
			want = 200
		}
		if r.StatusCode != want {
			t.Fatalf("%s status=%d, want %d", path, r.StatusCode, want)
		}
		if healthy {
			continue
		}
		result, err := h.challengeManager.Verify(context.Background(), ch.ID, code, "")
		if err != nil || result == nil || !result.OK {
			t.Fatalf("%s consumed or revoked unaudited challenge: result=%v err=%v", path, result, err)
		}
	}
}

func waitForAuditDelivery(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		rows, err := auditlog.Query(context.Background(), audit.DefaultQueryFilter())
		if err == nil && len(rows) >= 8 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("audit events not delivered: records=%d err=%v", len(rows), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
