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

func TestTOTPProxyIdentifierLimitsFailBeforeSend(t *testing.T) {
	for _, field := range []string{"service", "key ID"} {
		t.Run(field, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, `{"subject":"user","totp_enabled":true}`)
			}))
			defer upstream.Close()
			opts := &heraldtotp.Options{BaseURL: upstream.URL, HMACSecret: "synthetic-hmac-secret", APIKey: "synthetic-api-key", Service: "herald", HMACKeyID: "default"}
			if field == "service" {
				opts.Service = strings.Repeat("s", 129)
			} else {
				opts.HMACKeyID = strings.Repeat("k", 129)
			}
			client, err := newTOTPProxyClient(opts, 1024, false)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Status(context.Background(), "user")
			if err == nil || !strings.Contains(err.Error(), "sign TOTP request") || calls.Load() != 0 {
				t.Fatalf("error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestTOTPProxyAPIKeyIgnoresUnusedHMACIdentifiers(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-API-Key") != "synthetic-api-key" || r.Header.Get("X-Signature") != "" {
			t.Error("unexpected authentication headers")
		}
		_, _ = io.WriteString(w, `{"subject":"user","totp_enabled":true}`)
	}))
	defer upstream.Close()
	client, err := newTOTPProxyClient(&heraldtotp.Options{BaseURL: upstream.URL, APIKey: "synthetic-api-key", Service: strings.Repeat("s", 129), HMACKeyID: strings.Repeat("k", 129)}, 1024, false)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Status(context.Background(), "user")
	if err != nil || response == nil || calls.Load() != 1 {
		t.Fatalf("response=%+v error=%v calls=%d", response, err, calls.Load())
	}
}
