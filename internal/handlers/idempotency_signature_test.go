package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/auth"
	provider "github.com/soulteary/provider-kit"
)

func TestSignedIdempotencyKeyCannotBeRewrittenForSecondDelivery(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer func() { _ = rc.Close() }()
	h := newIdemHandlers(t, rc)
	p := &countingProvider{channel: provider.ChannelEmail}
	h.providerRegistry = provider.NewRegistry()
	if err := h.providerRegistry.Register(p); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/v1/otp/challenges", auth.New(auth.Config{Mode: auth.ModeHMACV2, RequireV3: true, KeyProvider: func(string) string { return "signing-secret" }, DefaultKeyID: "key", NonceStore: auth.NewNonceStore(rc, "tamper:nonce:", []byte("nonce-secret"), time.Minute)}), h.CreateChallenge)
	var statuses []int
	var ids []string
	for i, wireKey := range []string{"rewritten-key", "client-key"} {
		if i > 0 {
			mr.FastForward(61 * time.Second)
		}
		ts, nonce := strconv.FormatInt(time.Now().Unix(), 10), "nonce-"+strconv.Itoa(i)
		req := httptest.NewRequest(http.MethodPost, "/v1/otp/challenges", strings.NewReader(faultBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", wireKey)
		req.Header.Set(auth.HeaderSignatureVersion, auth.SignatureVersionV3)
		req.Header.Set(auth.HeaderTimestamp, ts)
		req.Header.Set(auth.HeaderNonce, nonce)
		req.Header.Set(auth.HeaderService, "caller")
		req.Header.Set(auth.HeaderKeyID, "key")
		// The caller intends client-key for both requests. Only the first request's
		// transport header is rewritten; the captured signature is unchanged.
		req.Header.Set(auth.HeaderSignature, auth.SignV3("signing-secret", auth.CanonicalRequest{Method: http.MethodPost, Path: "/v1/otp/challenges", Timestamp: ts, Nonce: nonce, Service: "caller", KeyID: "key", IdempotencyKey: "client-key", Body: []byte(faultBody)}))
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		statuses = append(statuses, resp.StatusCode)
		id, _ := body["challenge_id"].(string)
		ids = append(ids, id)
	}
	if statuses[0] != 401 || statuses[1] != 200 || atomic.LoadInt64(&p.calls) != 1 {
		t.Fatalf("header rewrite caused duplicate delivery: statuses=%v ids=%v provider_calls=%d", statuses, ids, atomic.LoadInt64(&p.calls))
	}
	keys, err := rc.Keys(context.Background(), "otp:idem2:*").Result()
	if err != nil || len(keys) != 1 {
		t.Fatalf("unexpected idempotency records: %v err=%v", keys, err)
	}
}
