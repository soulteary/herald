package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

func TestAmbiguousPrincipalHeadersRejectBeforeSideEffects(t *testing.T) {
	for _, mode := range []string{"hmac_v2", "api_key", "none", "legacy"} {
		for _, field := range []string{HeaderService, HeaderKeyID} {
			t.Run(mode+"/"+field, func(t *testing.T) {
				mr := miniredis.RunT(t)
				rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
				t.Cleanup(func() { _ = rc.Close() })
				now := time.Now()
				cfg := Config{Mode: Mode(mode), APIKey: "test-api-key", Clock: func() time.Time { return now },
					KeyProvider: func(string) string { return "test-secret" }, NonceStore: NewNonceStore(rc, "test:nonce:", []byte("test-nonce-secret"), time.Minute)}
				headers := signedHeaders(now, "nonce", "key", "test-secret")
				headers[field] = "first|kid:second"
				headers["X-API-Key"] = "test-api-key"
				headers[HeaderSignature] = SignV2("test-secret", CanonicalRequest{
					Method: http.MethodPost, Path: "/resource", Query: "scope=write", Timestamp: headers[HeaderTimestamp],
					Nonce: headers[HeaderNonce], Service: headers[HeaderService], KeyID: headers[HeaderKeyID], Body: []byte(middlewareTestBody),
				})
				legacyCalled := false
				if mode == "legacy" {
					cfg.Mode, cfg.V1Enabled = ModeHMACV2, true
					cfg.V1Handler = func(c fiber.Ctx) error { legacyCalled = true; return c.Next() }
					delete(headers, HeaderSignatureVersion)
				}
				status, reason := runMiddlewareRequest(t, cfg, headers)
				if status != http.StatusUnauthorized || reason != "invalid_auth_headers" || legacyCalled || len(mr.Keys()) != 0 {
					t.Fatalf("ambiguous headers accepted or caused side effects: status=%d reason=%s legacy=%v keys=%v", status, reason, legacyCalled, mr.Keys())
				}
			})
		}
	}
}

func TestOrdinaryPrincipalHeadersRemainAccepted(t *testing.T) {
	for _, service := range []string{"caller", "workers|kid-other", "workers|KID:other"} {
		t.Run(service, func(t *testing.T) {
			status, reason := runMiddlewareRequest(t, Config{Mode: ModeAPIKey, APIKey: "test-key"}, map[string]string{
				"X-API-Key": "test-key", HeaderService: service, HeaderKeyID: "key:version-1",
			})
			if status != http.StatusNoContent {
				t.Fatalf("ordinary metadata rejected: status=%d reason=%s", status, reason)
			}
		})
	}
}
