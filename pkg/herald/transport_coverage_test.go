package herald

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func sdkErrorCalls(c *Client, ctx context.Context) map[string]func() error {
	return map[string]func() error{
		"create":       func() error { _, e := c.CreateChallenge(ctx, &CreateChallengeRequest{}); return e },
		"verify":       func() error { _, e := c.VerifyChallenge(ctx, &VerifyChallengeRequest{}); return e },
		"v2":           func() error { _, e := c.VerifyChallengeV2(ctx, &VerifyChallengeV2Request{}); return e },
		"revoke":       func() error { _, e := c.RevokeChallenge(ctx, "challenge"); return e },
		"TOTP status":  func() error { _, e := c.TOTPStatus(ctx, "user"); return e },
		"TOTP verify":  func() error { _, e := c.TOTPVerify(ctx, &TOTPVerifyRequest{}); return e },
		"TOTP start":   func() error { _, e := c.TOTPEnrollStart(ctx, &TOTPEnrollStartRequest{}); return e },
		"TOTP confirm": func() error { _, e := c.TOTPEnrollConfirm(ctx, &TOTPEnrollConfirmRequest{}); return e },
		"TOTP revoke":  func() error { _, e := c.TOTPRevoke(ctx, "user"); return e },
	}
}

func TestSDKCancellationAndInterruptedBodies(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer s.Close()
	c, err := NewClient(&Options{BaseURL: s.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, call := range sdkErrorCalls(c, ctx) {
		t.Run(name+" canceled", func(t *testing.T) {
			var e *HeraldError
			if err := call(); !errors.As(err, &e) || e.Reason != "connection_failed" {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
	for name, call := range sdkErrorCalls(c, context.Background()) {
		t.Run(name+" interrupted", func(t *testing.T) {
			var e *HeraldError
			if err := call(); !errors.As(err, &e) || e.Reason != "response_read_failed" {
				t.Fatalf("interrupted response accepted: %v", err)
			}
		})
	}
}

func TestSDKV2AndRevokeRejectMalformedAndFailedResponses(t *testing.T) {
	for _, body := range []string{"not JSON", `{"ok":false,"reason":"locked"}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503); _, _ = io.WriteString(w, body) }))
		c, err := NewClient(DefaultOptions().WithBaseURL(s.URL))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"v2", "revoke"} {
			if err := sdkErrorCalls(c, context.Background())[name](); err == nil {
				t.Fatalf("%s accepted failure", name)
			}
		}
		s.Close()
	}
}

func TestSDKDefaultsAndConstructionFailures(t *testing.T) {
	if c, e := NewClient(nil); c != nil || e == nil {
		t.Fatal("nil options accepted without URL")
	}
	if e := (&Options{BaseURL: "https://example.com", Timeout: -time.Second}).Validate(); e == nil {
		t.Fatal("negative timeout accepted")
	}
	if c, e := NewClient(&Options{BaseURL: "https://example.com", TLSCACertFile: "missing-ca.pem"}); c != nil || e == nil {
		t.Fatal("missing CA accepted")
	}
	c, e := NewClient((&Options{BaseURL: "http://example.com"}).WithKeyID("key"))
	if e != nil {
		t.Fatal(e)
	}
	if c.sigVersion != SignatureV2 || c.maxResponseBytes != defaultMaxResponseBytes || c.keyID != "key" {
		t.Fatalf("defaults missing: %+v", c)
	}
	// A caller can pass an idempotency key through the documented context key.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "existing-key" {
			t.Error("idempotency header missing")
		}
		_, _ = io.WriteString(w, `{"challenge_id":"challenge"}`)
	}))
	defer s.Close()
	c, e = NewClient(DefaultOptions().WithBaseURL(s.URL))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.CreateChallenge(context.WithValue(context.Background(), IdempotencyKeyContextKey, "existing-key"), &CreateChallengeRequest{}); e != nil {
		t.Fatal(e)
	}
}
