package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-totp/pkg/heraldtotp"
	"github.com/soulteary/herald/internal/config"
)

func newTOTPSecurityApp(t *testing.T, baseURL string, limit int) *fiber.App {
	t.Helper()
	oldEnabled, oldURL, oldKey, oldSecret := config.TOTPEnabled, config.TOTPBaseURL, config.TOTPAPIKey, config.TOTPHMACSecret
	oldEnv, oldLimit := config.Env, config.ProviderMaxResponseBytes
	oldSMTP, oldSMS, oldSMTPURL, oldDingURL := config.SMTPHost, config.SMSProvider, config.HeraldSMTPAPIURL, config.HeraldDingtalkAPIURL
	t.Cleanup(func() {
		config.TOTPEnabled, config.TOTPBaseURL, config.TOTPAPIKey, config.TOTPHMACSecret = oldEnabled, oldURL, oldKey, oldSecret
		config.Env, config.ProviderMaxResponseBytes = oldEnv, oldLimit
		config.SMTPHost, config.SMSProvider, config.HeraldSMTPAPIURL, config.HeraldDingtalkAPIURL = oldSMTP, oldSMS, oldSMTPURL, oldDingURL
	})
	config.Env = config.EnvDevelopment
	config.TOTPEnabled, config.TOTPBaseURL = true, baseURL
	config.TOTPAPIKey, config.TOTPHMACSecret = "totp-api-secret", "totp-hmac-secret"
	config.ProviderMaxResponseBytes = limit
	config.SMTPHost, config.SMSProvider, config.HeraldSMTPAPIURL, config.HeraldDingtalkAPIURL = "", "", "", ""
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Post("/verify", h.TOTPVerify)
	app.Get("/status", h.TOTPStatus)
	app.Post("/enroll/start", h.TOTPEnrollStart)
	app.Post("/enroll/confirm", h.TOTPEnrollConfirm)
	app.Post("/revoke", h.TOTPRevoke)
	return app
}

var totpSecurityEndpoints = []struct {
	method, path, body, response string
}{
	{http.MethodGet, "/status?subject=user%2Btag%40example.com", "", `{"subject":"user+tag@example.com","totp_enabled":true}`},
	{http.MethodPost, "/verify", `{"subject":"user","code":"123456","challenge_id":"challenge"}`, `{"ok":false,"reason":"invalid","subject":"user"}`},
	{http.MethodPost, "/enroll/start", `{"subject":"user","label":"account"}`, `{"enroll_id":"enrollment","secret_base32":"secret","otpauth_uri":"otpauth://totp/account"}`},
	{http.MethodPost, "/enroll/confirm", `{"enroll_id":"enrollment","code":"123456"}`, `{"subject":"user","totp_enabled":true,"backup_codes":["backup"]}`},
	{http.MethodPost, "/revoke", `{"subject":"user"}`, `{"ok":true,"subject":"user"}`},
}

func TestTOTPSecurityAllEndpointsEnforceResponseBoundary(t *testing.T) {
	for _, endpoint := range totpSecurityEndpoints {
		for _, mode := range []string{"HTTP error", "unexpected 201", "oversized", "truncated", "invalid JSON", "trailing JSON", "null"} {
			t.Run(endpoint.path+"/"+mode, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body := endpoint.response
					switch mode {
					case "HTTP error":
						w.WriteHeader(http.StatusInternalServerError)
					case "unexpected 201":
						w.WriteHeader(http.StatusCreated)
					case "oversized":
						body += strings.Repeat(" ", 256)
					case "truncated":
						w.Header().Set("Content-Length", "1024")
					case "invalid JSON":
						body = "secret upstream response"
					case "trailing JSON":
						body += `{}`
					case "null":
						body = "null"
					}
					_, _ = io.WriteString(w, body)
				}))
				defer upstream.Close()
				app := newTOTPSecurityApp(t, upstream.URL, 256)
				status, body := totpSecurityRequest(t, app, endpoint.method, endpoint.path, endpoint.body)
				if status != http.StatusBadGateway || !strings.Contains(body, `"reason":"proxy_failed"`) || strings.Contains(body, "secret") {
					t.Fatalf("unsafe response: status=%d body=%s", status, body)
				}
			})
		}
	}
}

func TestTOTPSecurityAllEndpointsDenyRedirects(t *testing.T) {
	for _, endpoint := range totpSecurityEndpoints {
		for _, status := range []int{301, 302, 303, 307, 308} {
			t.Run(endpoint.path+"/"+http.StatusText(status), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/redirected" {
						calls.Add(1)
						_, _ = io.WriteString(w, endpoint.response)
						return
					}
					http.Redirect(w, r, "/redirected", status)
				}))
				defer upstream.Close()
				app := newTOTPSecurityApp(t, upstream.URL, 1024)
				got, body := totpSecurityRequest(t, app, endpoint.method, endpoint.path, endpoint.body)
				if got != http.StatusBadGateway || calls.Load() != 0 {
					t.Fatalf("redirect followed: status=%d calls=%d body=%s", got, calls.Load(), body)
				}
			})
		}
	}
}

func TestTOTPSecurityPreservesSuccessfulWireContract(t *testing.T) {
	for _, endpoint := range totpSecurityEndpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			type observed struct{ method, path, query, subject, contentType, apiKey, service, timestamp, signature, version, keyID, nonce, body string }
			requests := make(chan observed, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				requests <- observed{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.URL.Query().Get("subject"), r.Header.Get("Content-Type"), r.Header.Get("X-API-Key"), r.Header.Get("X-Service"), r.Header.Get("X-Timestamp"), r.Header.Get("X-Signature"), r.Header.Get("X-Signature-Version"), r.Header.Get("X-Key-Id"), r.Header.Get("X-Nonce"), string(body)}
				_, _ = io.WriteString(w, endpoint.response)
			}))
			defer upstream.Close()
			// Accept an exact-size response and retain a configured base path.
			app := newTOTPSecurityApp(t, upstream.URL+"/proxy/", len(endpoint.response))
			status, body := totpSecurityRequest(t, app, endpoint.method, endpoint.path, endpoint.body)
			if status != http.StatusOK {
				t.Fatalf("valid response rejected: %d %s", status, body)
			}
			var got, want interface{}
			require.NoError(t, json.Unmarshal([]byte(body), &got))
			require.NoError(t, json.Unmarshal([]byte(endpoint.response), &want))
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("response changed: got %s want %s", gotJSON, wantJSON)
			}
			r := <-requests
			path := strings.Split(endpoint.path, "?")[0]
			if r.path != "/proxy/v1"+path || r.method != endpoint.method || r.apiKey != "" || r.service != "stargate" || r.timestamp == "" {
				t.Fatalf("request changed: %+v", r)
			}
			expected := heraldtotp.SignV2("totp-hmac-secret", heraldtotp.RequestSignature{Method: r.method, Path: r.path, Query: r.query, KeyID: r.keyID, Timestamp: r.timestamp, Service: r.service, Nonce: r.nonce, Body: []byte(r.body)})
			if r.signature != expected || r.version != "v2" || len(r.nonce) != 32 {
				t.Fatal("operation-bound TOTP signature missing")
			}

			if endpoint.method == http.MethodGet {
				if r.subject != "user+tag@example.com" || r.body != "" {
					t.Fatalf("query changed: %+v", r)
				}
			} else if r.contentType != "application/json" || r.body != endpoint.body {
				t.Fatalf("JSON request changed: %+v", r)
			}
		})
	}
}

func TestTOTPSecurityEnrollment400RemainsInvalid(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"reason":"invalid","secret":"not for caller"}`)
	}))
	defer upstream.Close()
	app := newTOTPSecurityApp(t, upstream.URL, 1024)
	status, body := totpSecurityRequest(t, app, http.MethodPost, "/enroll/confirm", `{"enroll_id":"enrollment","code":"123456"}`)
	if status != http.StatusBadRequest || !strings.Contains(body, `"reason":"invalid"`) || strings.Contains(body, "secret") {
		t.Fatalf("enrollment error changed: %d %s", status, body)
	}
}

func TestTOTPSecurityRejectsUntrustedCertificate(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	app := newTOTPSecurityApp(t, upstream.URL, 1024)
	status, body := totpSecurityRequest(t, app, http.MethodPost, "/verify", `{"subject":"user","code":"123456"}`)
	if status != http.StatusBadGateway || calls.Load() != 0 {
		t.Fatalf("untrusted TLS accepted: %d %s calls=%d", status, body, calls.Load())
	}
}

func TestTOTPSecurityValidatesClientConfiguration(t *testing.T) {
	for _, base := range []string{"", "%", "file:///tmp/totp", "https:///missing-host", "https://user:secret@example.com", "https://example.com?key=secret", "https://example.com#fragment", "http://example.com"} {
		t.Run(base, func(t *testing.T) {
			if c, err := newTOTPProxyClient(heraldtotp.DefaultOptions().WithBaseURL(base), 1024, true); c != nil || err == nil {
				t.Fatalf("invalid config accepted: %v %v", c, err)
			}
		})
	}
}

func TestTOTPSecurityTLSRedirectCannotDowngrade(t *testing.T) {
	var targetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls.Add(1)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer target.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	c, err := newTOTPProxyClient(heraldtotp.DefaultOptions().WithBaseURL(origin.URL).WithAPIKey("secret"), 1024, true)
	if err != nil {
		t.Fatal(err)
	}
	// Trust only the test origin's certificate; keep the production redirect guard.
	c.client.Transport = origin.Client().Transport
	result, err := c.Verify(context.Background(), &heraldtotp.VerifyRequest{Subject: "user", Code: "123456"})
	if err == nil || result != nil || targetCalls.Load() != 0 {
		t.Fatalf("TLS downgrade followed: result=%v err=%v calls=%d", result, err, targetCalls.Load())
	}
}

func TestTOTPSecurityCopiesOptionsAndPreservesCancellation(t *testing.T) {
	if c, err := newTOTPProxyClient(nil, 0, false); c != nil || err == nil {
		t.Fatal("nil options accepted")
	}
	var calls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer origin.Close()
	opts := heraldtotp.DefaultOptions().WithBaseURL(origin.URL).WithAPIKey("original").WithTimeout(0)
	c, err := newTOTPProxyClient(opts, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	opts.APIKey, opts.BaseURL = "changed", "http://other.invalid"
	if c.options.APIKey != "original" || c.baseURL.String() != origin.URL || c.client.Timeout != 10*time.Second || c.maxResponseBytes != 1<<20 {
		t.Fatal("constructor retained mutable options or omitted safety defaults")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := c.Verify(ctx, &heraldtotp.VerifyRequest{Subject: "user", Code: "123456"})
	if !errors.Is(err, context.Canceled) || result != nil || calls.Load() != 0 {
		t.Fatalf("cancellation lost: result=%v err=%v calls=%d", result, err, calls.Load())
	}
}

func totpSecurityRequest(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func TestTOTPSecurityRejectsFalseSuccess(t *testing.T) {
	for _, mode := range []string{"HTTP error", "oversized body", "truncated body"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "HTTP error":
					w.WriteHeader(http.StatusInternalServerError)
				case "truncated body":
					w.Header().Set("Content-Length", "100")
				}
				_, _ = io.WriteString(w, `{"ok":true}`)
				if mode == "oversized body" {
					_, _ = io.WriteString(w, strings.Repeat(" ", 128))
				}
			}))
			defer upstream.Close()
			app := newTOTPSecurityApp(t, upstream.URL, 32)
			status, body := totpSecurityRequest(t, app, http.MethodPost, "/verify", `{"subject":"user","code":"123456"}`)
			if status != http.StatusBadGateway || strings.Contains(body, `"ok":true`) {
				t.Fatalf("untrusted success accepted: status=%d body=%s", status, body)
			}
		})
	}
}

func TestTOTPSecurityRedirectCannotForwardCredentials(t *testing.T) {
	captured := make(chan http.Header, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured <- r.Header.Clone()
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/stolen", http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	app := newTOTPSecurityApp(t, upstream.URL, 1024)
	status, body := totpSecurityRequest(t, app, http.MethodPost, "/verify", `{"subject":"user","code":"123456"}`)
	select {
	case h := <-captured:
		t.Errorf("redirect reached another origin: API key sent=%v, signature sent=%v", h.Get("X-API-Key") != "", h.Get("X-Signature") != "")
	default:
	}
	if status != http.StatusBadGateway {
		t.Fatalf("redirect response: status=%d body=%s", status, body)
	}
}
