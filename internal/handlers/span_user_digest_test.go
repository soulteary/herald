package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	provider "github.com/soulteary/provider-kit"
	tracingkit "github.com/soulteary/tracing-kit/v2"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/soulteary/herald/internal/config"
)

// TestSpansCarryUserDigestNotRawUserID proves the create, v1 verify and v2
// verify spans identify the user by peppered digest only.
func TestSpansCarryUserDigestNotRawUserID(t *testing.T) {
	const userID = "span-user-alice@example.com"

	oldEnv, oldTestMode, oldCooldown := config.Env, config.TestMode, config.ResendCooldown
	oldUser, oldIP, oldDest := config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination
	oldCodeLength, oldAttempts, oldExpiry := config.CodeLength, config.MaxAttempts, config.ChallengeExpiry
	oldPurposes, oldPolicy, oldSecret := config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret
	t.Cleanup(func() {
		config.Env, config.TestMode, config.ResendCooldown = oldEnv, oldTestMode, oldCooldown
		config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination = oldUser, oldIP, oldDest
		config.CodeLength, config.MaxAttempts, config.ChallengeExpiry = oldCodeLength, oldAttempts, oldExpiry
		config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret = oldPurposes, oldPolicy, oldSecret
	})

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracingkit.Install(tp, "span-user-digest-test")
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); tracingkit.Uninstall() })

	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	// debug_code is the only way to learn the code; it needs test env + test mode.
	config.Env, config.TestMode = config.EnvTest, true
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(&recordingSMSProvider{}))

	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	app.Post("/v1/otp/verifications", h.VerifyChallenge)
	app.Post("/v2/otp/verifications", h.VerifyChallengeV2)

	post := func(path, body string) map[string]any {
		t.Helper()
		req := httptest.NewRequest("POST", path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		require.NoError(t, err)
		raw, _ := io.ReadAll(resp.Body)
		require.Equal(t, fiber.StatusOK, resp.StatusCode, string(raw))
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}
	create := func(dest string) (string, string) {
		t.Helper()
		out := post("/v1/otp/challenges", smsChallengeBody(userID, dest))
		id, _ := out["challenge_id"].(string)
		code, _ := out["debug_code"].(string)
		require.NotEmpty(t, id)
		require.NotEmpty(t, code)
		return id, code
	}

	id, code := create("+15551230001")
	post("/v1/otp/verifications", fmt.Sprintf(`{"challenge_id":%q,"code":%q}`, id, code))
	id, code = create("+15551230002")
	post("/v2/otp/verifications", fmt.Sprintf(`{"challenge_id":%q,"code":%q,"expected_user_id":%q,"expected_purpose":"login","expected_channel":"sms"}`, id, code, userID))

	wantDigest := h.digester.Digest(userID)
	digests := map[string]int{}
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			require.NotEqual(t, "user_id", string(attr.Key), "span %s exports a raw user_id attribute", span.Name())
			require.NotContains(t, attr.Value.String(), userID, "span %s attribute %s leaks the user id", span.Name(), attr.Key)
			if attr.Key == "user_id_digest" {
				require.Equal(t, wantDigest, attr.Value.AsString(), "span %s", span.Name())
				digests[span.Name()]++
			}
		}
	}
	require.Equal(t, map[string]int{"otp.challenge.create": 2, "otp.verify": 2}, digests)
}
