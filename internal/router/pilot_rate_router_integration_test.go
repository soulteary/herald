//go:build pilot_rate_integration

package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
	provider "github.com/soulteary/provider-kit"
)

func pilotSet[T any](t *testing.T, target *T, value T) {
	t.Helper()
	old := *target
	*target = value
	t.Cleanup(func() { *target = old })
}

// Builds the real production router, auth, proxy sanitizer, handler and rate
// manager. Only Redis (miniredis) and the final delivery HTTP hop are fixtures.
func pilotProductionRouter(t *testing.T, trusted []string, ipLimit, userLimit, destLimit int) (string, *atomic.Int64) {
	t.Helper()
	calls := new(atomic.Int64)
	providerHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/send" || r.Header.Get("X-API-Key") != "synthetic-adapter-key" {
			http.Error(w, "unexpected fixture request", 400)
			return
		}
		var msg provider.HTTPSendRequest
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil || msg.Channel != "dingtalk" {
			http.Error(w, "unexpected synthetic message", 400)
			return
		}
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(provider.HTTPSendResponse{OK: true, MessageID: "synthetic-task", Provider: "dingtalk"})
	}))
	t.Cleanup(providerHTTP.Close)
	pilotSet(t, &config.Env, config.EnvTest)
	pilotSet(t, &config.RequestAuthMode, "api_key")
	pilotSet(t, &config.APIKey, "synthetic-gateway-key")
	pilotSet(t, &config.ServicePoliciesJSON, "")
	pilotSet(t, &config.HeraldDingtalkAPIURL, providerHTTP.URL)
	pilotSet(t, &config.HeraldDingtalkAPIKey, "synthetic-adapter-key")
	pilotSet(t, &config.HeraldSMTPAPIURL, "")
	pilotSet(t, &config.SMSAPIBaseURL, "")
	pilotSet(t, &config.TrustedProxyHeader, "X-Forwarded-For")
	pilotSet(t, &config.TrustedProxies, trusted)
	pilotSet(t, &config.RateLimitPerIP, ipLimit)
	pilotSet(t, &config.RateLimitPerUser, userLimit)
	pilotSet(t, &config.RateLimitPerDestination, destLimit)
	pilotSet(t, &config.PIIPepper, "synthetic-pilot-pepper")
	pilotSet(t, &config.IdempotencySecret, "synthetic-pilot-idem")
	pilotSet(t, &config.CodeLength, 6)
	pilotSet(t, &config.MaxAttempts, 5)
	pilotSet(t, &config.AllowedPurposes, []string{"login"})
	pilotSet(t, &config.ResendCooldown, time.Minute)
	pilotSet(t, &config.ChallengeExpiry, 5*time.Minute)
	pilotSet(t, &config.ProviderTimeout, time.Second)
	pilotSet(t, &config.ProviderFailurePolicy, "strict")
	pilotSet(t, &config.TestMode, false)
	rc, _ := testutil.NewTestRedisClient()
	t.Cleanup(func() { _ = rc.Close() })
	rw, err := NewRouterWithClientAndHandlersE(rc, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- rw.App.Listener(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rw.App.ShutdownWithContext(ctx); err != nil {
			t.Error(err)
		}
		if err := rw.Handlers.StopAuditWriter(); err != nil {
			t.Error(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-ctx.Done():
			t.Error("router did not stop")
		}
	})
	return "http://" + listener.Addr().String(), calls
}

func pilotCreate(t *testing.T, base, user, destination, reported, forwarded string, want int) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"user_id": user, "channel": "dingtalk", "destination": destination, "purpose": "login", "client_ip": reported})
	req, err := http.NewRequest("POST", base+"/v1/otp/challenges", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "synthetic-gateway-key")
	if forwarded != "" {
		req.Header.Set("X-Forwarded-For", forwarded)
	}
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("status=%d want=%d body=%s", response.StatusCode, want, body)
	}
	if want == 429 && (!strings.Contains(string(body), "rate_limit_exceeded") || response.Header.Get("Retry-After") == "") {
		t.Fatalf("missing stable rate-limit reason/retry: %s", body)
	}
}

func TestPilotProductionRouterSharedGatewayBucketAndBodyCannotBypass(t *testing.T) {
	base, calls := pilotProductionRouter(t, nil, 5, 100, 100)
	for n := 0; n < 6; n++ {
		want := 200
		if n == 5 {
			want = 429
		}
		pilotCreate(t, base, fmt.Sprintf("u%d", n), fmt.Sprintf("Ding-%d", n), fmt.Sprintf("192.0.2.%d", n+1), fmt.Sprintf("198.51.100.%d", n+1), want)
	}
	if calls.Load() != 5 {
		t.Fatalf("mock deliveries=%d want5", calls.Load())
	}
}

func TestPilotProductionRouterTrustedProxySplitsClientsAndBoundsChain(t *testing.T) {
	base, calls := pilotProductionRouter(t, []string{"127.0.0.1"}, 1, 100, 100)
	pilotCreate(t, base, "a", "Ding-A", "192.0.2.1", "198.51.100.10, 127.0.0.1", 200)
	pilotCreate(t, base, "b", "Ding-B", "192.0.2.2", "198.51.100.11, 127.0.0.1", 200)
	// Rotating a spoofed leftmost hop leaves the rightmost untrusted client bucket.
	pilotCreate(t, base, "c", "Ding-C", "192.0.2.3", "203.0.113.99, 198.51.100.10, 127.0.0.1", 429)
	if calls.Load() != 2 {
		t.Fatalf("mock deliveries=%d want2", calls.Load())
	}
}

func TestPilotProductionRouterAccountAndReceiverLimitsRemain(t *testing.T) {
	for _, axis := range []string{"user", "destination"} {
		t.Run(axis, func(t *testing.T) {
			u, d := 100, 100
			if axis == "user" {
				u = 1
			} else {
				d = 1
			}
			base, calls := pilotProductionRouter(t, []string{"127.0.0.1"}, 100, u, d)
			pilotCreate(t, base, "a", "Ding-A", "192.0.2.1", "198.51.100.10", 200)
			user, dest := "b", "Ding-B"
			if axis == "user" {
				user = "a"
			} else {
				dest = "Ding-A"
			}
			pilotCreate(t, base, user, dest, "192.0.2.2", "198.51.100.11", 429)
			pilotCreate(t, base, "c", "Ding-C", "192.0.2.3", "198.51.100.12", 200)
			if calls.Load() != 2 {
				t.Fatalf("mock deliveries=%d want2", calls.Load())
			}
		})
	}
}
