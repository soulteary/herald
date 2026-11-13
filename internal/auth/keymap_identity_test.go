package auth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestKeyMapRequiresSignedWireIdentityAcrossDefaults(t *testing.T) {
	for _, version := range []string{SignatureVersionV2, SignatureVersionV3} {
		t.Run(version, func(t *testing.T) {
			now := time.Unix(1_700_000_000, 0)
			store := newNonceStore(t)
			cfg := Config{
				Mode: ModeHMACV2, RequireKeyID: true, DefaultKeyID: "old",
				Clock: func() time.Time { return now }, NonceStore: store,
				KeyProvider: func(id string) string {
					if id == "old" || id == "alias" {
						return "same-secret"
					}
					return ""
				},
			}
			headers := signedHeaders(now, "same-nonce", "", "same-secret")
			if version == SignatureVersionV3 {
				headers[HeaderSignatureVersion] = version
				headers[HeaderSignature] = SignV3("same-secret", CanonicalRequest{Method: "POST", Path: "/resource", Query: "scope=write", Timestamp: headers[HeaderTimestamp], Nonce: headers[HeaderNonce], Service: "caller", Body: []byte(middlewareTestBody)})
			}
			// The exact omitted-ID packet cannot become a different principal
			// when an operator changes the configured default, even for aliases.
			for _, defaultID := range []string{"old", "alias"} {
				cfg.DefaultKeyID = defaultID
				if status, reason := runMiddlewareRequest(t, cfg, headers); status != http.StatusUnauthorized || reason != "key_id_required" {
					t.Fatalf("default %s: status=%d reason=%s", defaultID, status, reason)
				}
			}
			if keys, err := store.client.Keys(context.Background(), "*").Result(); err != nil || len(keys) != 0 {
				t.Fatalf("rejected omitted identity changed nonce state: keys=%v err=%v", keys, err)
			}
			// An explicit signed identity still uses the existing nonce record
			// when the configured default changes.
			headers = signedHeaders(now, "explicit-nonce", "old", "same-secret")
			if version == SignatureVersionV3 {
				headers[HeaderSignatureVersion] = version
				headers[HeaderSignature] = SignV3("same-secret", CanonicalRequest{Method: "POST", Path: "/resource", Query: "scope=write", Timestamp: headers[HeaderTimestamp], Nonce: headers[HeaderNonce], Service: "caller", KeyID: "old", Body: []byte(middlewareTestBody)})
			}
			cfg.DefaultKeyID = "old"
			if status, reason := runMiddlewareRequest(t, cfg, headers); status != http.StatusNoContent {
				t.Fatalf("explicit identity: status=%d reason=%s", status, reason)
			}
			cfg.DefaultKeyID = "alias"
			if status, reason := runMiddlewareRequest(t, cfg, headers); status != http.StatusUnauthorized || reason != "replayed_nonce" {
				t.Fatalf("explicit replay: status=%d reason=%s", status, reason)
			}
		})
	}
}
