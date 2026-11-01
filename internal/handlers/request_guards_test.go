package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/config"
)

func TestVerificationRequestGuards(t *testing.T) {
	h, _, _ := setupV2(t)
	for _, tc := range []struct{ path, body, reason string }{
		{"/v1", "not JSON", "invalid_request"},
		{"/v2", "not JSON", "invalid_request"},
		{"/v2", `{}`, "challenge_id_required"},
		{"/v2", `{"challenge_id":"id"}`, "code_required"},
		{"/v2", `{"challenge_id":"id","code":"bad"}`, "invalid_code_format"},
	} {
		app := fiber.New()
		app.Post("/v1", h.VerifyChallenge)
		app.Post("/v2", h.VerifyChallengeV2)
		req := httptest.NewRequest("POST", tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		err = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != 400 || out["reason"] != tc.reason {
			t.Fatalf("status=%d body=%v err=%v", resp.StatusCode, out, err)
		}
	}
}

func TestVerificationChannelAMR(t *testing.T) {
	h, _, _ := setupV2(t)
	for _, channel := range []string{"sms", "dingtalk"} {
		for _, version := range []string{"v1", "v2"} {
			h.challengeManager = verificationBackend{result: &challengekit.VerifyResult{OK: true, Challenge: &challengekit.Challenge{ID: "ch", UserID: "u", Purpose: "login", Channel: challengekit.Channel(channel)}}}
			app := fiber.New()
			app.Post("/verify", h.VerifyChallenge)
			if version == "v2" {
				app = fiber.New()
				app.Post("/verify", h.VerifyChallengeV2)
			}
			body := `{"challenge_id":"ch","code":"123456"}`
			if version == "v2" {
				body = `{"challenge_id":"ch","code":"123456","expected_user_id":"u","expected_purpose":"login","expected_channel":"` + channel + `"}`
			}
			req := httptest.NewRequest("POST", "/verify", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			var out struct {
				AMR []string `json:"amr"`
			}
			err = json.NewDecoder(resp.Body).Decode(&out)
			_ = resp.Body.Close()
			if err != nil || resp.StatusCode != 200 || len(out.AMR) != 2 || out.AMR[1] != channel {
				t.Fatalf("channel=%s status=%d AMR=%v err=%v", channel, resp.StatusCode, out.AMR, err)
			}
		}
	}
}

func TestMissingRouteParametersAndRevokeBackend(t *testing.T) {
	h, _, _ := setupV2(t)
	oldEnv, oldMode := config.Env, config.TestMode
	defer func() { config.Env, config.TestMode = oldEnv, oldMode }()
	config.Env, config.TestMode = config.EnvTest, true
	app := fiber.New()
	app.Post("/revoke", h.RevokeChallenge)
	app.Get("/code", h.GetTestCode)
	for _, path := range []string{"/revoke", "/code"} {
		method := "POST"
		if path == "/code" {
			method = "GET"
		}
		resp, err := app.Test(httptest.NewRequest(method, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("%s status=%d", path, resp.StatusCode)
		}
	}
	h.challengeManager = revokeBackend{ManagerInterface: h.challengeManager}
	app.Post("/revoke/:id", h.RevokeChallenge)
	resp, err := app.Test(httptest.NewRequest("POST", "/revoke/ch", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("revoke status=%d", resp.StatusCode)
	}
}

type revokeBackend struct{ challengekit.ManagerInterface }

func (revokeBackend) Revoke(context.Context, string) error { return context.DeadlineExceeded }
