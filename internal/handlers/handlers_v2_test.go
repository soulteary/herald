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

	challengeMgr := challengekit.NewManager(redisClient, challengekit.Config{
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

func doV2(t *testing.T, h *Handlers, body VerifyChallengeV2Request) (int, map[string]interface{}) {
	t.Helper()
	app := fiber.New()
	app.Post("/v2/verify", h.VerifyChallengeV2)
	b, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/v2/verify", bytes.NewBuffer(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]interface{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// TestVerifyChallengeV2_PurposeMismatch proves the v2 endpoint rejects a
// challenge redeemed under the wrong purpose with 409 context_mismatch, and
// that this does not consume the challenge (a subsequent correct request with
// the right context still succeeds).
func TestVerifyChallengeV2_PurposeMismatch(t *testing.T) {
	h, ch, code := setupV2(t)

	status, out := doV2(t, h, VerifyChallengeV2Request{
		ChallengeID:     ch.ID,
		Code:            code,
		ClientIP:        "127.0.0.1",
		ExpectedPurpose: "reset",
	})
	if status != fiber.StatusConflict {
		t.Fatalf("mismatch status = %d, want %d (body=%v)", status, fiber.StatusConflict, out)
	}
	if out["reason"] != "context_mismatch" {
		t.Fatalf("reason = %v, want context_mismatch", out["reason"])
	}

	// The challenge must still be redeemable with the correct context.
	status, out = doV2(t, h, VerifyChallengeV2Request{
		ChallengeID:     ch.ID,
		Code:            code,
		ClientIP:        "127.0.0.1",
		ExpectedUserID:  "user123",
		ExpectedPurpose: "login",
		ExpectedChannel: "email",
	})
	if status != fiber.StatusOK || out["ok"] != true {
		t.Fatalf("correct context status = %d body=%v, want 200 ok", status, out)
	}
	if out["purpose"] != "login" || out["user_id"] != "user123" {
		t.Fatalf("success body missing bound fields: %v", out)
	}
}

// TestVerifyChallengeV2_UserMismatch proves user binding is enforced.
func TestVerifyChallengeV2_UserMismatch(t *testing.T) {
	h, ch, code := setupV2(t)
	status, out := doV2(t, h, VerifyChallengeV2Request{
		ChallengeID:    ch.ID,
		Code:           code,
		ClientIP:       "127.0.0.1",
		ExpectedUserID: "someone-else",
	})
	if status != fiber.StatusConflict || out["reason"] != "context_mismatch" {
		t.Fatalf("user mismatch status=%d body=%v, want 409 context_mismatch", status, out)
	}
}

// TestVerifyChallengeV2_NoBindingBehavesLikeV1 proves that omitting the expected
// fields keeps v1 semantics (a correct code succeeds).
func TestVerifyChallengeV2_NoBindingBehavesLikeV1(t *testing.T) {
	h, ch, code := setupV2(t)
	status, out := doV2(t, h, VerifyChallengeV2Request{
		ChallengeID: ch.ID,
		Code:        code,
		ClientIP:    "127.0.0.1",
	})
	if status != fiber.StatusOK || out["ok"] != true {
		t.Fatalf("no-binding status=%d body=%v, want 200 ok", status, out)
	}
}
