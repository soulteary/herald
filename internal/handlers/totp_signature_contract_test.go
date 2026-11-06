package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
)

func TestTOTPProxySigningFailureDoesNotSend(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	client, err := newTOTPProxyClient(&heraldtotp.Options{BaseURL: server.URL, HMACSecret: "test-secret"}, 1024, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Status(context.Background(), "test-user"); err == nil || !strings.Contains(err.Error(), "sign TOTP request") {
		t.Fatalf("unexpected signing result: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("unsigned request reached upstream")
	}
}
func TestTOTPProxyAPIKeyOnlyRemainsIndependent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" || r.Header.Get("X-Signature") != "" || r.Header.Get("X-Nonce") != "" {
			t.Error("API-key-only request contract changed")
		}
		_, _ = io.WriteString(w, `{"subject":"test-user","totp_enabled":false}`)
	}))
	defer server.Close()
	client, err := newTOTPProxyClient(heraldtotp.DefaultOptions().WithBaseURL(server.URL).WithAPIKey("test-key"), 1024, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Status(context.Background(), "test-user"); err != nil {
		t.Fatal(err)
	}
}
