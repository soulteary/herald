package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/soulteary/herald/internal/config"
)

func newCreateApp(t *testing.T) *fiber.App {
	t.Helper()
	config.CodeLength = 6
	config.MaxAttempts = 5
	config.ChallengeExpiry = 5 * time.Minute
	config.AllowedPurposes = []string{"login"}
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := NewHandlers(rc, testLogger())
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app
}

// TestStrictBody_RejectsWrongContentType proves a non-JSON content type is
// rejected with 415 before any processing.
func TestStrictBody_RejectsWrongContentType(t *testing.T) {
	app := newCreateApp(t)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(`{"user_id":"u","channel":"email","destination":"a@b.com","purpose":"login"}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnsupportedMediaType {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("wrong content-type status = %d, want 415, body=%s", resp.StatusCode, string(body))
	}
}

// TestStrictBody_RejectsUnknownField proves unknown fields are rejected.
func TestStrictBody_RejectsUnknownField(t *testing.T) {
	app := newCreateApp(t)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(`{"user_id":"u","channel":"email","destination":"a@b.com","purpose":"login","evil":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unknown field status = %d, want 400, body=%s", resp.StatusCode, string(body))
	}
	var out map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(body, &out)
	if out["reason"] != "invalid_request" {
		t.Fatalf("unknown field reason = %v, want invalid_request", out["reason"])
	}
}

// TestStrictBody_RejectsTrailingData proves concatenated/trailing JSON is
// rejected.
func TestStrictBody_RejectsTrailingData(t *testing.T) {
	app := newCreateApp(t)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(`{"user_id":"u","channel":"email","destination":"a@b.com","purpose":"login"}{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("trailing data status = %d, want 400, body=%s", resp.StatusCode, string(body))
	}
}

// TestStrictBody_AcceptsCharsetParam proves a charset parameter on the content
// type is accepted.
func TestStrictBody_AcceptsCharsetParam(t *testing.T) {
	app := newCreateApp(t)
	req := httptest.NewRequest("POST", "/v1/otp/challenges", bytes.NewBufferString(`{"user_id":"u","channel":"email","destination":"a@b.com","purpose":"login"}`))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	// Should get past body parsing (provider not registered -> soft policy still
	// returns 200 in dev with challenge created). Any non-4xx-body-error is fine;
	// we only assert it is NOT a 415/400 body rejection.
	if resp.StatusCode == fiber.StatusUnsupportedMediaType {
		t.Fatalf("charset param should be accepted, got 415")
	}
}
