package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/config"
)

// setupV2 creates a handler + a directly-created challenge for v2 tests.
func setupV2(t *testing.T) (*Handlers, *challengekit.Challenge, string) {
	t.Helper()
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.LockoutDuration = 10 * time.Minute
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	t.Cleanup(func() { _ = redisClient.Close() })

	handlers := NewHandlers(redisClient, testLogger())

	challengeMgr := newActiveTestChallengeManager(redisClient, challengekit.Config{
		Expiry:          config.ChallengeExpiry,
		MaxAttempts:     config.MaxAttempts,
		LockoutDuration: config.LockoutDuration,
		CodeLength:      config.CodeLength,
	})
	ch, code, err := challengeMgr.Create(context.Background(), challengekit.CreateRequest{
		UserID:      "user123",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	return handlers, ch, code
}

func doV2(t *testing.T, h *Handlers, body any) (int, map[string]interface{}) {
	t.Helper()
	app := fiber.New()
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v2/otp/verifications", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// A mismatch must not consume the challenge; the correct context can still
// redeem it once, but cannot redeem it a second time.
func TestVerifyChallengeV2_ContextMismatch(t *testing.T) {
	for _, field := range []string{"user", "purpose", "channel"} {
		t.Run(field, func(t *testing.T) {
			h, ch, code := setupV2(t)
			valid := VerifyChallengeV2Request{ChallengeID: ch.ID, Code: code, ExpectedUserID: "user123", ExpectedPurpose: "login", ExpectedChannel: "email"}
			wrong := valid
			switch field {
			case "user":
				wrong.ExpectedUserID = "someone-else"
			case "purpose":
				wrong.ExpectedPurpose = "reset"
			case "channel":
				wrong.ExpectedChannel = "sms"
			}
			status, out := doV2(t, h, wrong)
			if status != fiber.StatusConflict || out["reason"] != "context_mismatch" {
				t.Fatalf("mismatch: status=%d body=%v, want 409/context_mismatch", status, out)
			}
			status, out = doV2(t, h, valid)
			if status != fiber.StatusOK || out["ok"] != true || out["user_id"] != "user123" || out["purpose"] != "login" || out["channel"] != "email" || out["challenge_id"] != ch.ID {
				t.Fatalf("correct context after mismatch: status=%d body=%v", status, out)
			}
			status, out = doV2(t, h, valid)
			if status != fiber.StatusUnauthorized || out["ok"] != false {
				t.Fatalf("second redemption: status=%d body=%v, want 401", status, out)
			}
		})
	}
}

func TestVerifyChallengeV1_StillAcceptsUnboundRequest(t *testing.T) {
	h, ch, code := setupV2(t)
	app := fiber.New()
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	body, err := json.Marshal(VerifyChallengeRequest{ChallengeID: ch.ID, Code: code})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/v1/otp/verifications", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusOK || out["ok"] != true || out["user_id"] != "user123" {
		t.Fatalf("v1 verification: status=%d body=%v", resp.StatusCode, out)
	}
}

// Missing binding must be rejected before the challenge is consumed.
func TestVerifyChallengeV2_RequiresContext(t *testing.T) {
	for _, field := range []string{"all", "expected_user_id", "expected_purpose", "expected_channel"} {
		for _, value := range []string{"omitted", "empty", "whitespace", "null"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				h, ch, code := setupV2(t)
				body := map[string]any{"challenge_id": ch.ID, "code": code, "expected_user_id": "user123", "expected_purpose": "login", "expected_channel": "email"}
				for _, key := range []string{"expected_user_id", "expected_purpose", "expected_channel"} {
					if field != "all" && field != key {
						continue
					}
					switch value {
					case "omitted":
						delete(body, key)
					case "empty":
						body[key] = ""
					case "whitespace":
						body[key] = " \t\n"
					case "null":
						body[key] = nil
					}
				}
				status, out := doV2(t, h, body)
				if status != fiber.StatusBadRequest || out["reason"] != "context_required" {
					t.Fatalf("missing context: status=%d body=%v, want 400/context_required", status, out)
				}
				body["expected_user_id"], body["expected_purpose"], body["expected_channel"] = "user123", "login", "email"
				status, out = doV2(t, h, body)
				if status != fiber.StatusOK || out["ok"] != true {
					t.Fatalf("valid context after rejection: status=%d body=%v", status, out)
				}
			})
		}
	}
}
