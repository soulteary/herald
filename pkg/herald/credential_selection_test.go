package herald

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/soulteary/herald/internal/auth"
)

func TestSDKSelectsOneCredentialScheme(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		hmac          bool
		status        int
	}{
		{"API key", SignatureV3, false, 200},
		{"legacy HMAC", SignatureV1, true, 200},
		{"HMAC v2", SignatureV2, true, 200},
		{"HMAC v3", SignatureV3, true, 200},
		{"rejected HMAC v2", SignatureV2, true, 401},
		{"rejected HMAC v3", SignatureV3, true, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if tc.hmac {
					if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
						t.Error("HMAC request exposed an API key or bearer credential")
					}
					canonical := auth.CanonicalRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Timestamp: r.Header.Get("X-Timestamp"), Nonce: r.Header.Get("X-Nonce"), Service: r.Header.Get("X-Service"), KeyID: r.Header.Get("X-Key-Id"), Body: body}
					signature := r.Header.Get("X-Signature")
					if tc.version == SignatureV2 && !auth.VerifyV2("signing-secret", signature, canonical) {
						t.Error("invalid v2 signature")
					}
					if tc.version == SignatureV3 && !auth.VerifyV3("signing-secret", signature, canonical) {
						t.Error("invalid v3 signature")
					}
					if signature == "" || canonical.Timestamp == "" || canonical.Service != "caller" {
						t.Error("missing HMAC identity or signature")
					}
				} else if r.Header.Get("X-API-Key") != "configured-api-key" || r.Header.Get("X-Signature") != "" {
					t.Error("API-key-only request changed its authentication scheme")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status != http.StatusOK {
					_, _ = io.WriteString(w, `{"ok":false,"reason":"invalid_signature"}`)
					return
				}
				_, _ = io.WriteString(w, `{"ok":true,"challenge_id":"challenge","user_id":"user","purpose":"login","channel":"email"}`)
			}))
			defer server.Close()
			opts := DefaultOptions().WithBaseURL(server.URL).WithAPIKey("configured-api-key").WithService("caller").WithKeyID("caller-key").WithSignatureVersion(tc.version)
			if tc.hmac {
				opts.WithHMACSecret("signing-secret")
			}
			client, err := NewClient(opts)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.VerifyChallengeV2(context.Background(), &VerifyChallengeV2Request{ChallengeID: "challenge", Code: "123456", ExpectedUserID: "user", ExpectedPurpose: "login", ExpectedChannel: "email"})
			if (err != nil) != (tc.status != http.StatusOK) {
				t.Fatalf("verification error=%v, HTTP status=%d", err, tc.status)
			}
			if calls.Load() != 1 {
				t.Fatalf("made %d requests; expected one without credential fallback", calls.Load())
			}
		})
	}
}
