package auth

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSingleSecretKeyIDCannotSplitNonceIdentity(t *testing.T) {
	require.False(t, config.HasHMACKeys())
	old := config.HMACSecret
	config.HMACSecret = "single-secret"
	t.Cleanup(func() { config.HMACSecret = old })
	for _, version := range []string{SignatureVersionV2, SignatureVersionV3} {
		t.Run(version, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = rc.Close() })
			cfg := Config{Mode: ModeHMACV2, DefaultKeyID: "default", KeyProvider: config.GetHMACSecret,
				NonceStore: NewNonceStore(rc, "nonce:", []byte("nonce-secret"), time.Minute), FailClosed: true}
			now := time.Now()
			headers := func(keyID string) map[string]string {
				h := signedHeaders(now, "same-nonce", keyID, "single-secret")
				if version == SignatureVersionV3 {
					h[HeaderSignatureVersion] = version
					h[HeaderSignature] = SignV3("single-secret", CanonicalRequest{
						Method: "POST", Path: "/resource", Query: "scope=write", Timestamp: strconv.FormatInt(now.Unix(), 10),
						Nonce: "same-nonce", Service: "caller", KeyID: keyID, Body: []byte(middlewareTestBody)})
				}
				return h
			}
			status, reason := runMiddlewareRequest(t, cfg, headers("unconfigured-key"))
			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, "invalid_key_id", reason)
			require.Empty(t, mr.Keys(), "reject an unknown key ID before consuming a nonce")
			status, _ = runMiddlewareRequest(t, cfg, headers(""))
			require.Equal(t, http.StatusNoContent, status)
			status, reason = runMiddlewareRequest(t, cfg, headers("default"))
			require.Equal(t, http.StatusUnauthorized, status)
			require.Equal(t, "replayed_nonce", reason)
			require.Len(t, mr.Keys(), 1)
		})
	}
}
