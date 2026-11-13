package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/metrics"
)

func authFailureCount(t *testing.T, reason string) float64 {
	t.Helper()
	families, err := metrics.Registry.Gatherer().Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "herald_auth_failure_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "reason" && label.GetValue() == reason {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// TestUnknownKeyIDAnswersLikeBadSignature proves a request naming an unknown
// key ID gets the same status and body as one with a wrong signature for a
// configured key, so the response cannot be used to find out which key IDs
// exist. The metric keeps the two causes apart for operators.
func TestUnknownKeyIDAnswersLikeBadSignature(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cfg := Config{
		Mode: ModeHMACV2, Clock: func() time.Time { return now }, MaxDrift: time.Minute,
		KeyProvider: func(id string) string {
			if id == "key-1" {
				return "secret"
			}
			return ""
		},
		NonceStore: newNonceStore(t),
	}
	app := fiber.New()
	app.Use(New(cfg))
	app.Post("/resource", func(c fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })
	send := func(headers map[string]string) (int, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/resource?scope=write", strings.NewReader(middlewareTestBody))
		for name, value := range headers {
			req.Header.Set(name, value)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(body)
	}

	unknownBefore, signatureBefore := authFailureCount(t, "invalid_key_id"), authFailureCount(t, "invalid_signature")

	// Signed with the real secret, so only the key ID is wrong.
	unknownStatus, unknownBody := send(signedHeaders(now, "nonce-unknown-key", "no-such-key", "secret"))
	// Known key ID, wrong secret.
	badStatus, badBody := send(signedHeaders(now, "nonce-bad-signature", "key-1", "not-the-secret"))

	require.Equal(t, http.StatusUnauthorized, badStatus)
	require.JSONEq(t, `{"ok":false,"reason":"invalid_signature"}`, badBody)
	require.Equal(t, badStatus, unknownStatus)
	require.Equal(t, badBody, unknownBody, "an unknown key ID must be indistinguishable from a bad signature")

	require.InDelta(t, unknownBefore+1, authFailureCount(t, "invalid_key_id"), 0)
	require.InDelta(t, signatureBefore+1, authFailureCount(t, "invalid_signature"), 0)

	// A valid request still passes.
	okStatus, _ := send(signedHeaders(now, "nonce-valid", "key-1", "secret"))
	require.Equal(t, http.StatusNoContent, okStatus)
}
