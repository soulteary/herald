package handlers

import (
	"crypto/hmac"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
	"github.com/soulteary/herald/internal/config"
)

func TestTOTPProxyConfiguredSigningIdentity(t *testing.T) {
	oldID, oldService := config.TOTPHMACKeyID, config.TOTPHMACService
	t.Cleanup(func() { config.TOTPHMACKeyID, config.TOTPHMACService = oldID, oldService })
	for _, tc := range []struct{ name, id, service, wantService string }{
		{"explicit overlap key", "next", "herald-proxy", "herald-proxy"},
		{"preserved SDK defaults", "", "", "stargate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.TOTPHMACKeyID, config.TOTPHMACService = tc.id, tc.service
			secrets := map[string]string{"old": "different-old-secret", "next": "totp-hmac-secret"}
			if tc.id == "" {
				secrets = map[string]string{"only": "totp-hmac-secret"}
			}
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
					t.Error("HMAC request included API credentials")
				}
				id := r.Header.Get("X-Key-Id")
				if id != tc.id || r.Header.Get("X-Service") != tc.wantService {
					t.Error("unexpected identity headers")
				}
				selected := id
				if selected == "" && len(secrets) == 1 {
					for key := range secrets {
						selected = key
					}
				}
				secret, ok := secrets[selected]
				if !ok {
					t.Error("ambiguous or unknown key")
					w.WriteHeader(401)
					return
				}
				body, _ := io.ReadAll(r.Body)
				expected := heraldtotp.SignV2(secret, heraldtotp.RequestSignature{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, KeyID: id, Timestamp: r.Header.Get("X-Timestamp"), Service: r.Header.Get("X-Service"), Nonce: r.Header.Get("X-Nonce"), Body: body})
				if !hmac.Equal([]byte(expected), []byte(r.Header.Get("X-Signature"))) {
					t.Error("signature does not select configured key")
					w.WriteHeader(401)
					return
				}
				_, _ = io.WriteString(w, `{"subject":"user","totp_enabled":true}`)
			}))
			defer upstream.Close()
			app := newTOTPSecurityApp(t, upstream.URL, 1024)
			response, err := app.Test(httptest.NewRequest(http.MethodGet, "/status?subject=user", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != 200 || calls.Load() != 1 {
				t.Fatalf("status=%d calls=%d", response.StatusCode, calls.Load())
			}
		})
	}
}
