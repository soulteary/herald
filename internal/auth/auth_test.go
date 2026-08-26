package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// TestCanonical_GoldenFormat locks the exact v2 canonical string bytes. The SDK
// (pkg/herald.Client.computeHMACv2) has an identical golden test; if either
// side changes the canonical, one of the two golden tests will fail, catching
// drift before it becomes an interop break.
func TestCanonical_GoldenFormat(t *testing.T) {
	body := []byte(`{"user_id":"u1"}`)
	r := CanonicalRequest{
		Method:    "POST",
		Path:      "/v1/otp/challenges",
		Query:     "a=1&b=2",
		Timestamp: "1700000000",
		Nonce:     "nonce-xyz",
		Service:   "stargate",
		KeyID:     "key-1",
		Body:      body,
	}
	bodyHash := sha256.Sum256(body)
	want := "HERALD-HMAC-V2\n" +
		"POST\n" +
		"/v1/otp/challenges\n" +
		"a=1&b=2\n" +
		"1700000000\n" +
		"nonce-xyz\n" +
		"stargate\n" +
		"key-1\n" +
		hex.EncodeToString(bodyHash[:])
	if got := r.Canonical(); got != want {
		t.Fatalf("canonical format drift:\n got=%q\nwant=%q", got, want)
	}

	// And the signature over that canonical must match a hand-rolled HMAC.
	mac := hmac.New(sha256.New, []byte("test-secret"))
	mac.Write([]byte(want))
	if SignV2("test-secret", r) != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatal("SignV2 does not match hand-computed HMAC over canonical")
	}
}

func TestCanonical_TamperChangesSignature(t *testing.T) {
	base := CanonicalRequest{
		Method: "POST", Path: "/v1/otp/challenges", Query: "",
		Timestamp: "1000", Nonce: "n1", Service: "svc", KeyID: "k1",
		Body: []byte(`{"a":1}`),
	}
	sig := SignV2("secret", base)

	mutations := []func(r *CanonicalRequest){
		func(r *CanonicalRequest) { r.Method = "GET" },
		func(r *CanonicalRequest) { r.Path = "/v1/otp/verifications" },
		func(r *CanonicalRequest) { r.Query = "x=1" },
		func(r *CanonicalRequest) { r.Timestamp = "1001" },
		func(r *CanonicalRequest) { r.Nonce = "n2" },
		func(r *CanonicalRequest) { r.Service = "other" },
		func(r *CanonicalRequest) { r.KeyID = "k2" },
		func(r *CanonicalRequest) { r.Body = []byte(`{"a":2}`) },
	}
	for i, m := range mutations {
		r := base
		m(&r)
		if VerifyV2("secret", sig, r) {
			t.Errorf("mutation %d: signature still verified after tamper", i)
		}
	}
	if !VerifyV2("secret", sig, base) {
		t.Errorf("unmutated request should verify")
	}
}

func TestTimestampWithinDrift(t *testing.T) {
	now := time.Unix(1000, 0)
	if !TimestampWithinDrift("1000", now, 60*time.Second) {
		t.Errorf("exact timestamp should be within drift")
	}
	if !TimestampWithinDrift("1030", now, 60*time.Second) {
		t.Errorf("future within drift should pass")
	}
	if TimestampWithinDrift("900", now, 60*time.Second) {
		t.Errorf("stale timestamp should fail")
	}
	if TimestampWithinDrift("not-a-number", now, 60*time.Second) {
		t.Errorf("invalid timestamp should fail")
	}
}

func newNonceStore(t *testing.T) *NonceStore {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewNonceStore(client, "test:nonce:", []byte("secret"), time.Minute)
}

func TestNonceStore_SingleUse(t *testing.T) {
	s := newNonceStore(t)
	ctx := context.Background()
	fresh, err := s.Consume(ctx, "svc", "k1", "n1")
	if err != nil || !fresh {
		t.Fatalf("first consume fresh=%v err=%v", fresh, err)
	}
	fresh2, err := s.Consume(ctx, "svc", "k1", "n1")
	if err != nil {
		t.Fatalf("second consume err=%v", err)
	}
	if fresh2 {
		t.Errorf("nonce should be consumed on replay")
	}
}

// TestNonceStore_ConcurrentSameNonce proves exactly one concurrent caller sees
// the nonce as fresh.
func TestNonceStore_ConcurrentSameNonce(t *testing.T) {
	s := newNonceStore(t)
	ctx := context.Background()
	const n = 50
	var fresh int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ok, err := s.Consume(ctx, "svc", "k1", "shared-nonce")
			if err == nil && ok {
				atomic.AddInt64(&fresh, 1)
			}
		}()
	}
	wg.Wait()
	if fresh != 1 {
		t.Fatalf("fresh consumers = %d, want exactly 1", fresh)
	}
}

func TestNonceStore_DifferentServicesNoCollision(t *testing.T) {
	s := newNonceStore(t)
	ctx := context.Background()
	if ok, _ := s.Consume(ctx, "svcA", "k1", "n"); !ok {
		t.Fatalf("svcA nonce should be fresh")
	}
	if ok, _ := s.Consume(ctx, "svcB", "k1", "n"); !ok {
		t.Fatalf("svcB same nonce must not collide with svcA")
	}
}
