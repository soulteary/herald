package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
)

func TestHTTPEmailAndTOTPInitializationModes(t *testing.T) {
	oldSMTP, oldTOTP, oldBase, oldSecret, oldEnv := config.HeraldSMTPAPIURL, config.TOTPEnabled, config.TOTPBaseURL, config.TOTPHMACSecret, config.Env
	defer func() {
		config.HeraldSMTPAPIURL, config.TOTPEnabled, config.TOTPBaseURL, config.TOTPHMACSecret, config.Env = oldSMTP, oldTOTP, oldBase, oldSecret, oldEnv
	}()
	rc := testRedisClient(t)
	defer func() { _ = rc.Close() }()
	config.HeraldSMTPAPIURL = "https://smtp.example.com"
	config.TOTPEnabled, config.TOTPBaseURL, config.TOTPHMACSecret = true, "https://totp.example.com/", "totp-secret"
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if h.totpClient == nil {
		t.Fatal("TOTP not initialized")
	}
	if _, err := h.providerRegistry.Get(provider.ChannelEmail); err != nil {
		t.Fatal("HTTP email provider not registered")
	}
	config.TOTPBaseURL = "https://%zz"
	for _, env := range []config.Environment{config.EnvDevelopment, config.EnvProduction} {
		config.Env = env
		h, err := NewHandlersWithError(rc, testLogger())
		if env == config.EnvProduction {
			if err == nil || h != nil || !strings.Contains(err.Error(), "herald-totp client init failed") {
				t.Fatalf("broken production TOTP accepted: %v", err)
			}
		} else if err != nil || h == nil || h.totpClient != nil {
			t.Fatalf("development fallback: handlers=%v err=%v", h, err)
		}
	}
}

func TestTOTPTransportConstructionAndReadFailures(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	c, err := newTOTPProxyClient(heraldtotp.DefaultOptions().WithBaseURL(upstream.URL), 1024, false)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	for name, call := range map[string]func() error{
		"unsupported payload":  func() error { return c.call(context.Background(), "POST", "/", nil, make(chan int), &result) },
		"invalid method":       func() error { return c.call(context.Background(), "bad method", "/", nil, nil, &result) },
		"interrupted response": func() error { return c.call(context.Background(), "GET", "/", nil, nil, &result) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("invalid transport operation accepted")
			}
		})
	}
}
