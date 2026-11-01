package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gofiber/fiber/v3"
	logger "github.com/soulteary/logger-kit/v3"
	provider "github.com/soulteary/provider-kit"
	tracingkit "github.com/soulteary/tracing-kit/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestProviderFailureDoesNotExportSensitiveMessage(t *testing.T) {
	const credential = "upstream-private-api-key"
	const destination = "+15551234567"
	type delivery struct{ code, id string }
	captured := make(chan delivery, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req provider.HTTPSendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		code := regexp.MustCompile(`[0-9]{6}`).FindString(req.Body)
		captured <- delivery{code, req.IdempotencyKey}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(provider.HTTPSendResponse{OK: false, ErrorCode: "timeout", ErrorMessage: fmt.Sprintf("otp=%s destination=%s credential=%s", code, req.To, r.Header.Get("X-API-Key"))})
	}))
	defer upstream.Close()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracingkit.Install(tp, "provider-privacy-test")
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); tracingkit.Uninstall() })
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	var logs bytes.Buffer
	h.log = logger.New(logger.Config{Output: &logs, Format: logger.FormatJSON})
	p, err := provider.NewHTTPProvider(&provider.HTTPConfig{BaseURL: upstream.URL, APIKey: credential, ChannelType: provider.ChannelSMS, ProviderName: "privacy-test"})
	require.NoError(t, err)
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	resp, err := idemRequest(app, "privacy-key", `{"user_id":"privacy-user","channel":"sms","destination":"+15551234567","purpose":"login"}`)
	require.NoError(t, err)
	require.Equal(t, 500, resp.status)
	sent := <-captured
	code := sent.code
	require.NotEmpty(t, code)
	require.EqualValues(t, 0, rc.Exists(context.Background(), "otp:ch:"+sent.id).Val())
	require.Contains(t, logs.String(), `"reason":"timeout"`)
	var spans bytes.Buffer
	for _, span := range recorder.Ended() {
		if span.Name() == "otp.provider.send" {
			require.Equal(t, codes.Error, span.Status().Code)
			require.Equal(t, "provider send failed: timeout", span.Status().Description)
			_, _ = fmt.Fprintf(&spans, "%v %v %v", span.Status(), span.Events(), span.Attributes())
		}
	}
	require.NotEmpty(t, spans.String())
	for _, secret := range []string{"otp=" + code, destination, credential} {
		assert.NotContains(t, logs.String(), secret, "provider error leaked to logs")
		assert.NotContains(t, spans.String(), secret, "provider error leaked to tracing")
		assert.NotContains(t, string(resp.body), secret)
	}
	retry, err := idemRequest(app, "privacy-key", `{"user_id":"privacy-user","channel":"sms","destination":"+15551234567","purpose":"login"}`)
	require.NoError(t, err)
	require.Equal(t, 409, retry.status)
	require.Contains(t, string(retry.body), "delivery_unknown")
}
