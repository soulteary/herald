package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	logger "github.com/soulteary/logger-kit/v3"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/idempotency"
	"github.com/soulteary/herald/internal/metrics"
)

const (
	auditTestUser        = "audit-user-7"
	auditTestDestination = "audit-dest@example.com"
)

// auditSpoolSwitch makes audit writes fail by moving the spool directory away,
// and restores it afterwards. The audit writer is a process-wide singleton, so
// this is the only point where a test can fail one specific write.
type auditSpoolSwitch struct {
	mu      sync.Mutex
	dir     string
	offline bool
}

func (s *auditSpoolSwitch) takeOffline(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.offline {
		if err := os.Rename(s.dir, s.dir+"-offline"); err != nil {
			t.Errorf("take audit spool offline: %v", err)
		}
		s.offline = true
	}
}

func (s *auditSpoolSwitch) restore(t *testing.T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.offline {
		if err := os.Rename(s.dir+"-offline", s.dir); err != nil {
			t.Errorf("restore audit spool: %v", err)
		}
		s.offline = false
	}
}

// spoolOfflineProvider takes the audit spool offline while the provider is
// handling the message, after the pre-send audit record was accepted.
type spoolOfflineProvider struct {
	t     *testing.T
	spool *auditSpoolSwitch
	fail  bool
	calls atomic.Int64
}

func (p *spoolOfflineProvider) Send(context.Context, *provider.Message) (*provider.SendResult, error) {
	p.calls.Add(1)
	p.spool.takeOffline(p.t)
	if p.fail {
		return provider.NewFailureResult("spool-offline", provider.ChannelEmail, provider.NewProviderError(provider.ReasonSendFailed, "boom")), nil
	}
	return provider.NewSuccessResult("spool-offline", provider.ChannelEmail, "msg-1"), nil
}
func (*spoolOfflineProvider) Channel() provider.Channel { return provider.ChannelEmail }
func (*spoolOfflineProvider) Name() string              { return "spool-offline" }
func (*spoolOfflineProvider) Validate() error           { return nil }

// spoolOfflineManager takes the audit spool offline once verification or
// revocation has changed challenge state.
type spoolOfflineManager struct {
	challengekit.ManagerInterface
	t     *testing.T
	spool *auditSpoolSwitch
}

func (m *spoolOfflineManager) Verify(ctx context.Context, id, code, ip string) (*challengekit.VerifyResult, error) {
	result, err := m.ManagerInterface.Verify(ctx, id, code, ip)
	m.spool.takeOffline(m.t)
	return result, err
}

func (m *spoolOfflineManager) VerifyWithOptions(ctx context.Context, id, code, ip string, opts challengekit.VerifyOptions) (*challengekit.VerifyResult, error) {
	result, err := m.ManagerInterface.VerifyWithOptions(ctx, id, code, ip, opts)
	m.spool.takeOffline(m.t)
	return result, err
}

func (m *spoolOfflineManager) Revoke(ctx context.Context, id string) error {
	err := m.ManagerInterface.Revoke(ctx, id)
	m.spool.takeOffline(m.t)
	return err
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) take() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.buf.String()
	b.buf.Reset()
	return out
}

func postActionAuditFailures(t *testing.T, operation string) float64 {
	t.Helper()
	families, err := metrics.Registry.Gatherer().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "herald_audit_post_action_failure_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "operation" && label.GetValue() == operation {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// requirePostActionLog checks the one line that replaces the lost audit
// record: it identifies the event without the destination or the raw user ID.
func requirePostActionLog(t *testing.T, h *Handlers, logs, operation, event, challengeID, userID string) {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(logs, "\n") {
		if strings.Contains(line, `"stage":"post_action"`) {
			lines = append(lines, line)
		}
	}
	require.Len(t, lines, 1, "want exactly one post-action line in: %s", logs)
	line := lines[0]
	require.Contains(t, line, `"level":"error"`)
	require.Contains(t, line, `"operation":"`+operation+`"`)
	require.Contains(t, line, `"event_type":"`+event+`"`)
	require.Contains(t, line, `"challenge_id":"`+challengeID+`"`)
	if userID != "" {
		require.Contains(t, line, `"user_id_digest":"`+h.digester.Digest(userID)+`"`)
	}
	require.NotContains(t, logs, auditTestUser)
	require.NotContains(t, logs, auditTestDestination)
}

func auditTestRequest(t *testing.T, app *fiber.App, path string, body map[string]string) (int, map[string]interface{}) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 5 * time.Second})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var decoded map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&decoded))
	return resp.StatusCode, decoded
}

func activeAuditChallenge(t *testing.T, h *Handlers) (*challengekit.Challenge, string) {
	t.Helper()
	ch, code, err := h.challengeManager.Create(context.Background(), challengekit.CreateRequest{
		UserID: auditTestUser, Channel: challengekit.ChannelEmail, Destination: auditTestDestination, Purpose: "login",
	})
	require.NoError(t, err)
	_, err = h.challengeManager.SwapActive(context.Background(), ch)
	require.NoError(t, err)
	return ch, code
}

// An audit write that fails after the action it describes must not change the
// response. Each case accepts the pre-action audit record, then loses the
// spool before the post-action record is written. The audit writer is a
// singleton, so the cases share one subprocess.
func TestPostActionAuditFailureDoesNotFailRequest(t *testing.T) {
	if os.Getenv("HERALD_AUDIT_POST_ACTION") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPostActionAuditFailureDoesNotFailRequest$")
		cmd.Env = append(os.Environ(), "HERALD_AUDIT_POST_ACTION=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("post-action audit subprocess: %v\n%s", err, out)
		}
		return
	}

	config.AuditEnabled = true
	config.AuditSpoolDir = t.TempDir()
	config.AuditSpoolMaxBytes = 1 << 20
	spool := &auditSpoolSwitch{dir: config.AuditSpoolDir}
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	t.Cleanup(func() { _ = h.StopAuditWriter() })
	t.Cleanup(func() { spool.restore(t) })
	logs := &lockedBuffer{}
	h.log = logger.New(logger.Config{Output: logs, Format: logger.FormatJSON})
	realManager := h.challengeManager

	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	app.Post("/v1/otp/challenges/:id/revoke", h.RevokeChallenge)

	createBody := emailChallengeBody(auditTestUser, auditTestDestination)
	// Each create case starts without limiter or idempotency state.
	reset := func() {
		spool.restore(t)
		mr.FlushAll()
		logs.take()
		h.challengeManager = realManager
	}

	t.Run("create after provider accepted", func(t *testing.T) {
		reset()
		p := &spoolOfflineProvider{t: t, spool: spool}
		h.providerRegistry = provider.NewRegistry()
		require.NoError(t, h.providerRegistry.Register(p))
		before := postActionAuditFailures(t, "create")

		resp, err := idemRequest(app, "post-action-key", createBody)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))
		var created map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.body, &created))
		challengeID, _ := created["challenge_id"].(string)
		require.NotEmpty(t, challengeID)
		require.NotContains(t, created, "delivery_status")
		require.EqualValues(t, 1, p.calls.Load())
		require.Equal(t, before+1, postActionAuditFailures(t, "create"))
		requirePostActionLog(t, h, logs.take(), "create", "send_success", challengeID, auditTestUser)

		// The idempotency record is resolved, not left in the sending state.
		keys, err := rc.Keys(context.Background(), "otp:idem2:*").Result()
		require.NoError(t, err)
		require.Len(t, keys, 1)
		raw, err := rc.Get(context.Background(), keys[0]).Bytes()
		require.NoError(t, err)
		var record idempotency.Record
		require.NoError(t, json.Unmarshal(raw, &record))
		require.Equal(t, idempotency.StateSucceeded, record.State)

		// The challenge was activated and a retry replays without a second send.
		got, err := realManager.Get(context.Background(), challengeID)
		require.NoError(t, err)
		require.Equal(t, challengeID, got.ID)
		spool.restore(t)
		replay, err := idemRequest(app, "post-action-key", createBody)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, replay.status)
		require.JSONEq(t, string(resp.body), string(replay.body))
		require.EqualValues(t, 1, p.calls.Load())
	})

	t.Run("create with soft provider failure", func(t *testing.T) {
		reset()
		oldPolicy := config.ProviderFailurePolicy
		config.ProviderFailurePolicy = "soft"
		defer func() { config.ProviderFailurePolicy = oldPolicy }()
		p := &spoolOfflineProvider{t: t, spool: spool, fail: true}
		h.providerRegistry = provider.NewRegistry()
		require.NoError(t, h.providerRegistry.Register(p))
		before := postActionAuditFailures(t, "create")

		resp, err := idemRequest(app, "soft-post-action-key", createBody)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))
		var created map[string]interface{}
		require.NoError(t, json.Unmarshal(resp.body, &created))
		require.Equal(t, "failed", created["delivery_status"])
		challengeID, _ := created["challenge_id"].(string)
		require.Equal(t, before+1, postActionAuditFailures(t, "create"))
		requirePostActionLog(t, h, logs.take(), "create", "send_failed", challengeID, auditTestUser)

		keys, err := rc.Keys(context.Background(), "otp:idem2:*").Result()
		require.NoError(t, err)
		require.Len(t, keys, 1)
		raw, err := rc.Get(context.Background(), keys[0]).Bytes()
		require.NoError(t, err)
		var record idempotency.Record
		require.NoError(t, json.Unmarshal(raw, &record))
		require.Equal(t, idempotency.StateSucceeded, record.State)
	})

	for _, tc := range []struct{ name, path string }{
		{"verify v1 after code consumed", "/v1/otp/verifications"},
		{"verify v2 after code consumed", "/v2/otp/verifications"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			ch, code := activeAuditChallenge(t, h)
			h.challengeManager = &spoolOfflineManager{ManagerInterface: realManager, t: t, spool: spool}
			before := postActionAuditFailures(t, "verify")
			body := map[string]string{"challenge_id": ch.ID, "code": code}
			if strings.HasPrefix(tc.path, "/v2") {
				body["expected_user_id"] = auditTestUser
				body["expected_channel"] = "email"
				body["expected_purpose"] = "login"
			}

			status, decoded := auditTestRequest(t, app, tc.path, body)
			require.Equal(t, fiber.StatusOK, status, "%v", decoded)
			require.Equal(t, true, decoded["ok"])
			require.Equal(t, auditTestUser, decoded["user_id"])
			require.Equal(t, before+1, postActionAuditFailures(t, "verify"))
			captured := logs.take()
			requirePostActionLog(t, h, captured, "verify", "challenge_verified", ch.ID, auditTestUser)
			require.NotContains(t, captured, code)

			// The code was consumed by the request that reported success.
			spool.restore(t)
			h.challengeManager = realManager
			status, _ = auditTestRequest(t, app, tc.path, body)
			require.NotEqual(t, fiber.StatusOK, status)
		})
	}

	t.Run("revoke after challenge removed", func(t *testing.T) {
		reset()
		ch, _ := activeAuditChallenge(t, h)
		h.challengeManager = &spoolOfflineManager{ManagerInterface: realManager, t: t, spool: spool}
		before := postActionAuditFailures(t, "revoke")

		status, decoded := auditTestRequest(t, app, "/v1/otp/challenges/"+ch.ID+"/revoke", map[string]string{})
		require.Equal(t, fiber.StatusOK, status, "%v", decoded)
		require.Equal(t, true, decoded["ok"])
		require.Equal(t, before+1, postActionAuditFailures(t, "revoke"))
		requirePostActionLog(t, h, logs.take(), "revoke", "challenge_revoked", ch.ID, "")
		_, err := realManager.Get(context.Background(), ch.ID)
		require.Error(t, err, "the challenge must be gone")
	})

	// With the spool unavailable before the action, nothing may happen and the
	// post-action counter must not move.
	t.Run("pre-action failure still refuses", func(t *testing.T) {
		reset()
		p := &spoolOfflineProvider{t: t, spool: spool}
		h.providerRegistry = provider.NewRegistry()
		require.NoError(t, h.providerRegistry.Register(p))
		ch, code := activeAuditChallenge(t, h)
		beforeCreate, beforeVerify, beforeRevoke := postActionAuditFailures(t, "create"), postActionAuditFailures(t, "verify"), postActionAuditFailures(t, "revoke")
		spool.takeOffline(t)

		resp, err := idemRequest(app, "pre-action-key", createBody)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusServiceUnavailable, resp.status, string(resp.body))
		require.Contains(t, string(resp.body), `"reason":"audit_unavailable"`)
		require.Zero(t, p.calls.Load(), "no message may be sent without the creation audit record")
		keys, err := rc.Keys(context.Background(), "otp:idem2:*").Result()
		require.NoError(t, err)
		require.Empty(t, keys, "the pre-send claim must be released")

		verifyBody := map[string]string{"challenge_id": ch.ID, "code": code}
		status, decoded := auditTestRequest(t, app, "/v1/otp/verifications", verifyBody)
		require.Equal(t, fiber.StatusServiceUnavailable, status)
		require.Equal(t, "audit_unavailable", decoded["reason"])
		status, decoded = auditTestRequest(t, app, "/v1/otp/challenges/"+ch.ID+"/revoke", map[string]string{})
		require.Equal(t, fiber.StatusServiceUnavailable, status)
		require.Equal(t, "audit_unavailable", decoded["reason"])

		require.Equal(t, beforeCreate, postActionAuditFailures(t, "create"))
		require.Equal(t, beforeVerify, postActionAuditFailures(t, "verify"))
		require.Equal(t, beforeRevoke, postActionAuditFailures(t, "revoke"))
		require.NotContains(t, logs.take(), `"stage":"post_action"`)

		// The code was neither consumed nor revoked.
		spool.restore(t)
		result, err := realManager.Verify(context.Background(), ch.ID, code, "")
		require.NoError(t, err)
		require.True(t, result.OK)
	})
}
