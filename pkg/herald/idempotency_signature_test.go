package herald

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/auth"
)

func TestDefaultSDKSignsIdempotencyHeaderForServer(t *testing.T) {
	for _, key := range []string{"client-key", "", " \tclient-key \t"} {
		t.Run(key, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
				canonical := auth.CanonicalRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Timestamp: r.Header.Get("X-Timestamp"), Nonce: r.Header.Get("X-Nonce"), Service: r.Header.Get("X-Service"), KeyID: r.Header.Get("X-Key-Id"), IdempotencyKey: r.Header.Get("Idempotency-Key"), Body: body}
				if r.Header.Get("X-Signature-Version") != "v3" || canonical.IdempotencyKey != strings.Trim(key, " \t") || !auth.VerifyV3("secret", r.Header.Get("X-Signature"), canonical) {
					t.Error("SDK signature does not match header-bound server contract")
					w.WriteHeader(401)
					return
				}
				_, _ = io.WriteString(w, `{"challenge_id":"challenge"}`)
			}))
			defer upstream.Close()
			c, err := NewClient(DefaultOptions().WithBaseURL(upstream.URL).WithHMACSecret("secret").WithKeyID("key"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.WithValue(context.Background(), IdempotencyKeyContextKey, key)
			if _, err := c.CreateChallenge(ctx, &CreateChallengeRequest{UserID: "u"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSDKV3GoldenCanonical(t *testing.T) {
	body := []byte(`{"user_id":"u"}`)
	bodyHash, keyHash := sha256.Sum256(body), sha256.Sum256([]byte("client-key"))
	canonical := strings.Join([]string{"HERALD-HMAC-V3", "POST", "/v1/otp/challenges", "a=1", "1700000000", "nonce", "service", "key", hex.EncodeToString(keyHash[:]), hex.EncodeToString(bodyHash[:])}, "\n")
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(canonical))
	want := hex.EncodeToString(mac.Sum(nil))
	c := &Client{hmacSecret: "secret"}
	got := c.computeHMACCanonical("post", "/v1/otp/challenges", "a=1", "1700000000", "nonce", "service", "key", "client-key", body, true)
	if got != want {
		t.Fatalf("SDK signature=%s want %s", got, want)
	}
	if err := DefaultOptions().WithBaseURL("https://example.com").WithSignatureVersion(SignatureV3).Validate(); err != nil {
		t.Fatal(err)
	}
}
