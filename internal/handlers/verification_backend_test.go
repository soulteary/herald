package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
)

type verificationBackend struct {
	challengekit.ManagerInterface
	result *challengekit.VerifyResult
	err    error
}

func (b verificationBackend) Verify(context.Context, string, string, string) (*challengekit.VerifyResult, error) {
	return b.result, b.err
}
func (b verificationBackend) VerifyWithOptions(context.Context, string, string, string, challengekit.VerifyOptions) (*challengekit.VerifyResult, error) {
	return b.result, b.err
}

func TestVerificationBackendFailures(t *testing.T) {
	h, _, _ := setupV2(t)
	remaining := 2
	for _, v2 := range []bool{false, true} {
		for _, tc := range []struct {
			name   string
			result *challengekit.VerifyResult
			err    error
			reason string
			status int
			retry  bool
		}{
			{"nil result", nil, nil, "backend_unavailable", 503, true},
			{"transport", nil, errors.New("connection reset"), "backend_unavailable", 503, true},
			{"empty failure", &challengekit.VerifyResult{}, nil, "verification_failed", 401, false},
			{"contention", &challengekit.VerifyResult{Reason: "contended", RemainingAttempts: &remaining}, nil, "contended", 409, true},
		} {
			t.Run(tc.name+map[bool]string{true: "/v2", false: "/v1"}[v2], func(t *testing.T) {
				h.challengeManager = verificationBackend{result: tc.result, err: tc.err}
				app := fiber.New()
				if v2 {
					app.Post("/verify", h.VerifyChallengeV2)
				} else {
					app.Post("/verify", h.VerifyChallenge)
				}
				req := httptest.NewRequest("POST", "/verify", bytes.NewBufferString(`{"challenge_id":"id","code":"123456","client_ip":"203.0.113.5","expected_user_id":"u","expected_purpose":"login","expected_channel":"sms"}`))
				// v1 deliberately has no v2-only fields: strict JSON rejects unknown keys.
				if !v2 {
					req = httptest.NewRequest("POST", "/verify", bytes.NewBufferString(`{"challenge_id":"id","code":"123456","client_ip":"203.0.113.5"}`))
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := app.Test(req)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = resp.Body.Close() }()
				var body map[string]any
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if resp.StatusCode != tc.status || body["reason"] != tc.reason {
					t.Fatalf("status=%d body=%v", resp.StatusCode, body)
				}
				if (resp.Header.Get("Retry-After") == "1") != tc.retry {
					t.Fatal("incorrect retry guidance")
				}
				if tc.result != nil && tc.result.RemainingAttempts != nil && body["remaining_attempts"] != float64(remaining) {
					t.Fatalf("attempt budget missing: %v", body)
				}
			})
		}
	}
}

func TestVerificationErrorReasons(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{{nil, "verification_failed"}, {errors.New(" User Locked "), "locked"}, {errors.New("challenge not found"), "not_found"}} {
		if got := verificationReasonFromError(tc.err); got != tc.want {
			t.Fatalf("reason=%s want %s", got, tc.want)
		}
	}
}
