package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"
)

func TestValidChallengeID(t *testing.T) {
	valid := []string{
		"ch_AbCdEfGhIjKlMnOpQrStUv",            // generated form
		"ch_-_-_-_-_-_-_-_-_-_-_-_",            // URL-safe alphabet extremes
		"00000000-0000-0000-0000-000000000000", // UUID-shaped
		"a",
		strings.Repeat("a", maxChallengeIDBytes),
	}
	for _, id := range valid {
		require.True(t, validChallengeID(id), "%q", id)
	}
	invalid := []string{
		"",
		strings.Repeat("a", maxChallengeIDBytes+1),
		"ch_abc:def", "ch_abc def", "ch_abc*", "ch_abc/def", "ch_abc.def",
		"ch_abc\ndef", "ch_abc\x00def", "ch_abc+def", "ch_abc=", "ch_验证码",
	}
	for _, id := range invalid {
		require.False(t, validChallengeID(id), "%q", id)
	}
}

// TestMalformedChallengeIDIsRefusedBeforeAnyBackendUse sends challenge IDs
// that no release could have generated to both verification routes and the
// revoke route. Each is refused with 400/invalid_challenge_id and Redis sees
// no command: no verification lock key, no challenge lookup and no delete
// built from the value.
func TestMalformedChallengeIDIsRefusedBeforeAnyBackendUse(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.app.Post("/v1/otp/verifications", f.h.VerifyChallenge)
	f.app.Post("/v2/otp/verifications", f.h.VerifyChallengeV2)
	f.app.Post("/v1/otp/challenges/:id/revoke", f.h.RevokeChallenge)

	post := func(path string, body any) (int, string) {
		t.Helper()
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			require.NoError(t, err)
		}
		req := httptest.NewRequest("POST", path, bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.app.Test(req)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		var out struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out.Reason
	}

	malformed := map[string]string{
		"oversized":       strings.Repeat("a", 4096),
		"key separator":   "ch_abc:otp2:ch:other",
		"glob":            "ch_*",
		"space":           "ch_abc def",
		"non-ASCII":       "ch_验证码",
		"padded base64":   "ch_AbCdEfGhIjKlMnOpQrSt==",
		"standard base64": "ch_AbCd+fGhIjKlMnOpQrStUv",
	}
	for name, id := range malformed {
		t.Run(name, func(t *testing.T) {
			before := f.mr.CommandCount()

			status, reason := post("/v1/otp/verifications", map[string]string{"challenge_id": id, "code": "123456"})
			require.Equal(t, fiber.StatusBadRequest, status)
			require.Equal(t, "invalid_challenge_id", reason)

			status, reason = post("/v2/otp/verifications", map[string]string{
				"challenge_id": id, "code": "123456",
				"expected_user_id": "u1", "expected_purpose": "login", "expected_channel": "email",
			})
			require.Equal(t, fiber.StatusBadRequest, status)
			require.Equal(t, "invalid_challenge_id", reason)

			status, reason = post("/v1/otp/challenges/"+url.PathEscape(id)+"/revoke", nil)
			require.Equal(t, fiber.StatusBadRequest, status)
			require.Equal(t, "invalid_challenge_id", reason)

			require.Equal(t, before, f.mr.CommandCount(), "a malformed challenge ID reached Redis")
			for _, key := range f.mr.Keys() {
				require.NotContains(t, key, id)
			}
		})
	}
}

// TestWellFormedChallengeIDsStillReachVerification checks the validation does
// not refuse IDs a deployment can hold: a generated ID verifies, and an
// unknown ID of the generated or UUID shape is answered as before.
func TestWellFormedChallengeIDsStillReachVerification(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.app.Post("/v1/otp/verifications", f.h.VerifyChallenge)
	f.app.Post("/v1/otp/challenges/:id/revoke", f.h.RevokeChallenge)

	verify := func(id, code string) (int, string) {
		t.Helper()
		payload, err := json.Marshal(map[string]string{"challenge_id": id, "code": code})
		require.NoError(t, err)
		req := httptest.NewRequest("POST", "/v1/otp/verifications", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := f.app.Test(req)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		var out struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out.Reason
	}

	ch, code, err := f.h.challengeManager.Create(context.Background(), challengekit.CreateRequest{
		UserID: "well-formed-user", Channel: challengekit.ChannelEmail, Purpose: "login", Destination: "well-formed@example.com",
	})
	require.NoError(t, err)
	_, err = f.h.challengeManager.SwapActive(context.Background(), ch)
	require.NoError(t, err)
	require.True(t, validChallengeID(ch.ID), "generated ID %q must be accepted", ch.ID)
	status, _ := verify(ch.ID, code)
	require.Equal(t, fiber.StatusOK, status)

	for _, id := range []string{"ch_AbCdEfGhIjKlMnOpQrStUv", "00000000-0000-0000-0000-000000000000"} {
		status, reason := verify(id, "123456")
		require.Equal(t, fiber.StatusUnauthorized, status, id)
		require.Equal(t, "expired", reason, id)

		req := httptest.NewRequest("POST", "/v1/otp/challenges/"+id+"/revoke", nil)
		resp, err := f.app.Test(req)
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, resp.StatusCode, id)
	}
}
