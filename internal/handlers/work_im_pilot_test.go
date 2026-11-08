package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
)

// Real Herald challenge/Redis/idempotency/HTTPProvider pipeline. The last HTTP
// hop is a controlled provider fixture, never the DingTalk service.
func workIMPilotApp(t *testing.T, providerURL string) (*fiber.App, *Handlers) {
	t.Helper()
	oldURL, oldKey := config.HeraldDingtalkAPIURL, config.HeraldDingtalkAPIKey
	oldEnv, oldPolicy, oldSecret := config.Env, config.ProviderFailurePolicy, config.IdempotencySecret
	oldUser, oldIP, oldDest := config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination
	oldCooldown, oldExpiry, oldTimeout := config.ResendCooldown, config.ChallengeExpiry, config.ProviderTimeout
	oldCode, oldAttempts, oldPurposes := config.CodeLength, config.MaxAttempts, config.AllowedPurposes
	t.Cleanup(func() {
		config.HeraldDingtalkAPIURL, config.HeraldDingtalkAPIKey = oldURL, oldKey
		config.Env, config.ProviderFailurePolicy, config.IdempotencySecret = oldEnv, oldPolicy, oldSecret
		config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination = oldUser, oldIP, oldDest
		config.ResendCooldown, config.ChallengeExpiry, config.ProviderTimeout = oldCooldown, oldExpiry, oldTimeout
		config.CodeLength, config.MaxAttempts, config.AllowedPurposes = oldCode, oldAttempts, oldPurposes
	})
	config.HeraldDingtalkAPIURL, config.HeraldDingtalkAPIKey = providerURL, "synthetic-adapter-key"
	config.Env = config.EnvTest
	config.ProviderFailurePolicy = "strict"
	config.IdempotencySecret = "synthetic-idempotency-secret"
	config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination = 10000, 10000, 10000
	config.ResendCooldown = time.Second
	config.ChallengeExpiry = 5 * time.Minute
	config.ProviderTimeout = time.Second
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.AllowedPurposes = []string{"login"}
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.StopAuditWriter() })
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v2/verify", h.VerifyChallengeV2)
	return app, h
}

// Invoked only by the separate Stargate integration process using this test
// executable. Only synthetic messages exist in this local fixture.
func TestWorkIMPilotServeLocalBridge(t *testing.T) {
	path := os.Getenv("WORK_IM_FIXTURE_URL_FILE")
	if path == "" {
		t.Skip("external bridge helper requires WORK_IM_FIXTURE_URL_FILE")
	}
	var mu sync.Mutex
	var messages []map[string]string
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" || r.Header.Get("X-API-Key") != "synthetic-adapter-key" {
			http.Error(w, "invalid test provider call", 401)
			return
		}
		var req provider.HTTPSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		code := regexp.MustCompile(`\b[0-9]{6}\b`).FindString(req.Body)
		mu.Lock()
		messages = append(messages, map[string]string{"to": req.To, "channel": req.Channel, "challenge_id": req.IdempotencyKey, "synthetic_code": code})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provider.HTTPSendResponse{OK: true, MessageID: "synthetic-local-task", Provider: "dingtalk"})
	}))
	defer providerServer.Close()
	app, h := workIMPilotApp(t, providerServer.URL)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	stop := make(chan struct{})
	var once sync.Once
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_test/inbox" {
			mu.Lock()
			defer mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(messages)
			return
		}
		if r.URL.Path == "/_test/shutdown" {
			w.WriteHeader(204)
			once.Do(func() { close(stop) })
			return
		}
		resp, err := app.Test(r, fiber.TestConfig{Timeout: 5 * time.Second})
		if err != nil {
			http.Error(w, "fixture failure", 500)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer bridge.Close()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(bridge.URL); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stop:
	case <-time.After(45 * time.Second):
		t.Fatal("bridge fixture shutdown timeout")
	}
}

func TestWorkIMPilotHTTPProviderBindingAndVerify(t *testing.T) {
	var calls atomic.Int32
	var received provider.HTTPSendRequest
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" || r.Header.Get("X-API-Key") != "synthetic-adapter-key" {
			t.Error("incorrect adapter request route/auth")
		}
		calls.Add(1)
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provider.HTTPSendResponse{OK: true, MessageID: "synthetic-task", Provider: "dingtalk"})
	}))
	defer ps.Close()
	app, h := workIMPilotApp(t, ps.URL)
	body := `{"user_id":"user-A","channel":"dingtalk","destination":"ding-A","purpose":"login"}`
	first, err := idemRequest(app, "synthetic-key-A", body)
	if err != nil {
		t.Fatal(err)
	}
	if first.status != 200 {
		t.Fatalf("create status=%d", first.status)
	}
	var created struct {
		ChallengeID string `json:"challenge_id"`
	}
	if err := json.Unmarshal(first.body, &created); err != nil {
		t.Fatal(err)
	}
	if created.ChallengeID == "" || received.Channel != "dingtalk" || received.To != "ding-A" || received.IdempotencyKey != created.ChallengeID {
		t.Fatalf("incorrect challenge/provider binding: channel=%s destination=%s", received.Channel, received.To)
	}
	codes := regexp.MustCompile(`\b[0-9]{6}\b`).FindAllString(received.Body, -1)
	if len(codes) != 1 {
		t.Fatal("controlled provider body lacks one synthetic code")
	}
	replay, err := idemRequest(app, "synthetic-key-A", body)
	if err != nil {
		t.Fatal(err)
	}
	if replay.status != 200 || string(replay.body) != string(first.body) || calls.Load() != 1 {
		t.Fatal("idempotent challenge replay caused duplicate send or changed response")
	}
	changed, err := idemRequest(app, "synthetic-key-A", strings.Replace(body, "ding-A", "ding-B", 1))
	if err != nil {
		t.Fatal(err)
	}
	if changed.status != 409 || calls.Load() != 1 {
		t.Fatal("recipient change did not conflict before send")
	}
	caseChanged, err := idemRequest(app, "synthetic-key-A", strings.Replace(body, "ding-A", "ding-a", 1))
	if err != nil {
		t.Fatal(err)
	}
	if caseChanged.status != 409 || calls.Load() != 1 {
		t.Fatal("opaque recipient case change replayed another recipient's cached challenge")
	}
	for _, mismatch := range []VerifyChallengeV2Request{
		{ChallengeID: created.ChallengeID, Code: codes[0], ExpectedUserID: "user-B", ExpectedPurpose: "login", ExpectedChannel: "dingtalk"},
		{ChallengeID: created.ChallengeID, Code: codes[0], ExpectedUserID: "user-A", ExpectedPurpose: "reset", ExpectedChannel: "dingtalk"},
	} {
		status, out := doV2(t, h, mismatch)
		if status != 409 || out["reason"] != "context_mismatch" {
			t.Fatalf("context mismatch not rejected as expected: status=%d reason=%v", status, out["reason"])
		}
	}
	status, out := doV2(t, h, VerifyChallengeV2Request{ChallengeID: created.ChallengeID, Code: codes[0], ExpectedUserID: "user-A", ExpectedPurpose: "login", ExpectedChannel: "dingtalk"})
	if status != 200 || out["ok"] != true || out["user_id"] != "user-A" {
		t.Fatalf("valid context failed: status=%d", status)
	}
	status, _ = doV2(t, h, VerifyChallengeV2Request{ChallengeID: created.ChallengeID, Code: codes[0], ExpectedUserID: "user-A", ExpectedPurpose: "login", ExpectedChannel: "dingtalk"})
	if status == 200 {
		t.Fatal("consumed code accepted twice")
	}
}

func TestWorkIMPilotOpaqueRecipientRateLimitKeys(t *testing.T) {
	var calls atomic.Int32
	ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provider.HTTPSendResponse{OK: true, MessageID: "synthetic-rate-task", Provider: "dingtalk"})
	}))
	defer ps.Close()
	app, _ := workIMPilotApp(t, ps.URL)
	config.RateLimitPerDestination = 1
	for i, tc := range []struct {
		user, dest string
		status     int
	}{
		{"user-A", "Ding-A", 200},
		{"user-B", "ding-a", 200},
		{"user-C", "  Ding-A  ", 429},
	} {
		body, err := json.Marshal(map[string]string{"user_id": tc.user, "channel": "dingtalk", "destination": tc.dest, "purpose": "login"})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := idemRequest(app, tc.user, string(body))
		if err != nil {
			t.Fatal(err)
		}
		if resp.status != tc.status {
			t.Fatalf("case %d status=%d want=%d", i, resp.status, tc.status)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("provider sends=%d want2", calls.Load())
	}
}

func TestWorkIMPilotHTTPProviderFailureNoActiveChallenge(t *testing.T) {
	for _, failure := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limited", 429, `{"ok":false,"error_code":"rate_limited","error_message":"synthetic"}`},
		{"provider unavailable", 503, `{"ok":false,"error_code":"provider_down","error_message":"synthetic"}`},
		{"invalid destination", 400, `{"ok":false,"error_code":"invalid_destination","error_message":"synthetic"}`},
		{"malformed response", 200, `not-json`},
	} {
		t.Run(failure.name, func(t *testing.T) {
			var calls atomic.Int32
			ps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(failure.status)
				_, _ = io.WriteString(w, failure.body)
			}))
			defer ps.Close()
			app, _ := workIMPilotApp(t, ps.URL)
			response, err := idemRequest(app, "synthetic-failed-key", `{"user_id":"user-A","channel":"dingtalk","destination":"ding-A","purpose":"login"}`)
			if err != nil {
				t.Fatal(err)
			}
			if response.status != 500 || calls.Load() != 1 {
				t.Fatalf("provider failure status=%d calls=%d", response.status, calls.Load())
			}
			var result map[string]any
			if err := json.Unmarshal(response.body, &result); err != nil {
				t.Fatal(err)
			}
			if result["reason"] != "send_failed" || result["challenge_id"] != nil {
				t.Fatal("provider failure exposed usable challenge or false delivery success")
			}
		})
	}
}
