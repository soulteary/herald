package router

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// TestLegacyV1UnknownKeyIDAnswersLikeBadSignature sends requests that select
// the legacy v1 scheme (no X-Signature-Version) with HMAC_V1_ENABLED=true. A
// key ID that is not configured must get the same answer as a wrong signature
// for a configured key, so the response does not reveal which key IDs exist.
// In single-secret mode the only configured key ID is "default".
func TestLegacyV1UnknownKeyIDAnswersLikeBadSignature(t *testing.T) {
	defer withV2Config(t)()
	config.HMACV1Enabled = true
	rc, _ := testutil.NewTestRedisClient()
	defer func() { _ = rc.Close() }()
	app := NewRouterWithClientAndHandlers(rc, testLogger()).App

	send := func(keyID, timestamp string) (int, string, string) {
		t.Helper()
		req := httptest.NewRequest("POST", "/v1/otp/verifications", bytes.NewReader([]byte(`{"challenge_id":"ch_x","code":"123456"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(auth.HeaderSignature, "00")
		req.Header.Set(auth.HeaderTimestamp, timestamp)
		req.Header.Set(auth.HeaderService, "svc")
		req.Header.Set(auth.HeaderKeyID, keyID)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body), resp.Header.Get("Content-Length")
	}

	now := strconv.FormatInt(time.Now().Unix(), 10)
	stale := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	for _, tc := range []struct{ name, timestamp string }{
		{"current timestamp", now},
		{"stale timestamp", stale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			knownStatus, knownBody, knownLength := send("default", tc.timestamp)
			unknownStatus, unknownBody, unknownLength := send("no-such-key", tc.timestamp)
			if knownStatus != fiber.StatusUnauthorized {
				t.Fatalf("configured key with a wrong signature: status %d, body %s", knownStatus, knownBody)
			}
			if unknownStatus != knownStatus || unknownBody != knownBody || unknownLength != knownLength {
				t.Fatalf("unknown key ID is distinguishable from a wrong signature:\n configured: %d %s\n unknown:    %d %s",
					knownStatus, knownBody, unknownStatus, unknownBody)
			}
		})
	}
	if status, body, _ := send("default", now); body != `{"ok":false,"reason":"invalid_signature"}` {
		t.Fatalf("wrong signature for a configured key: %d %s", status, body)
	}
}
