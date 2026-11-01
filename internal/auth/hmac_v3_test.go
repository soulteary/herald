package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCanonicalV3BindsIdempotencyHeader(t *testing.T) {
	r := CanonicalRequest{Method: "post", Path: "/resource", Query: "scope=write", Timestamp: "1700000000", Nonce: "nonce", Service: "service", KeyID: "key", IdempotencyKey: "client-key", Body: []byte(middlewareTestBody)}
	keyHash, bodyHash := sha256.Sum256([]byte(r.IdempotencyKey)), sha256.Sum256(r.Body)
	expected := "HERALD-HMAC-V3\nPOST\n/resource\nscope=write\n1700000000\nnonce\nservice\nkey\n" + hex.EncodeToString(keyHash[:]) + "\n" + hex.EncodeToString(bodyHash[:])
	if got := r.CanonicalV3(); got != expected {
		t.Fatalf("canonical=%q want %q", got, expected)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(expected))
	signature := hex.EncodeToString(mac.Sum(nil))
	if !VerifyV3("secret", signature, r) || VerifyV2("secret", signature, r) {
		t.Fatal("signature does not have an independent v3 domain")
	}
	for _, changed := range []string{"", "different", "client-key\nextra"} {
		r.IdempotencyKey = changed
		if VerifyV3("secret", signature, r) {
			t.Fatalf("altered header %q accepted", changed)
		}
	}
}

func TestV3HeaderTamperingDoesNotConsumeNonce(t *testing.T) {
	for _, tc := range []struct{ name, signed, wire string }{{"changed", "client-key", "other"}, {"removed", "client-key", ""}, {"added", "", "injected"}} {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			defer func() { _ = rc.Close() }()
			now := time.Now()
			nonceStore := NewNonceStore(rc, "v3:", []byte("pepper"), time.Minute)
			cfg := Config{Mode: ModeHMACV2, RequireV3: true, FailClosed: true, Clock: func() time.Time { return now }, KeyProvider: func(string) string { return "secret" }, NonceStore: nonceStore}
			headers := signedHeaders(now, "nonce", "key", "secret")
			headers[HeaderSignatureVersion] = SignatureVersionV3
			headers["Idempotency-Key"] = tc.wire
			headers[HeaderSignature] = SignV3("secret", CanonicalRequest{Method: "POST", Path: "/resource", Query: "scope=write", Timestamp: strconv.FormatInt(now.Unix(), 10), Nonce: "nonce", Service: headers[HeaderService], KeyID: "key", IdempotencyKey: tc.signed, Body: []byte(middlewareTestBody)})
			status, reason := runMiddlewareRequest(t, cfg, headers)
			if status != 401 || reason != "invalid_signature" || len(mr.Keys()) != 0 {
				t.Fatalf("tamper accepted or consumed nonce: status=%d reason=%s keys=%v", status, reason, mr.Keys())
			}
			headers["Idempotency-Key"] = tc.signed
			if status, reason = runMiddlewareRequest(t, cfg, headers); status != http.StatusNoContent {
				t.Fatalf("original request rejected: %d %s", status, reason)
			}
			if status, reason = runMiddlewareRequest(t, cfg, headers); status != 401 || reason != "replayed_nonce" {
				t.Fatalf("replay accepted: %d %s", status, reason)
			}
		})
	}
}
