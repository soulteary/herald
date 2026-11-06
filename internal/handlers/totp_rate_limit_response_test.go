package handlers

import (
	"context"
	"errors"
	herald "github.com/soulteary/herald/pkg/herald"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTOTPConfirmPreservesBoundedUpstreamRateLimit(t *testing.T) {
	for _, scenario := range []string{"complete", "truncated", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "untrusted-retry-value")
				w.Header().Set("X-Upstream-Secret", "private-upstream-header")
				if scenario == "truncated" {
					w.Header().Set("Content-Length", "1024")
				}
				w.WriteHeader(http.StatusTooManyRequests)
				body := "private-upstream-body"
				if scenario == "oversized" {
					body = strings.Repeat("x", 257)
				}
				_, _ = io.WriteString(w, body)
			}))
			defer upstream.Close()
			app := newTOTPSecurityApp(t, upstream.URL, 256)
			request := httptest.NewRequest(http.MethodPost, "/enroll/confirm", strings.NewReader(`{"enroll_id":"synthetic-enrollment","code":"123456"}`))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantReason := 429, "rate_limited"
			if scenario != "complete" {
				wantStatus, wantReason = 502, "proxy_failed"
			}
			if response.StatusCode != wantStatus || !strings.Contains(string(body), `"reason":"`+wantReason+`"`) {
				t.Fatalf("status=%d body=%s", response.StatusCode, body)
			}
			if scenario == "complete" {
				bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.URL.Path = "/enroll/confirm"
					proxied, err := app.Test(r)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					defer func() { _ = proxied.Body.Close() }()
					w.WriteHeader(proxied.StatusCode)
					_, _ = io.Copy(w, proxied.Body)
				}))
				defer bridge.Close()
				client, err := herald.NewClient(herald.DefaultOptions().WithBaseURL(bridge.URL).WithAPIKey("synthetic-client-key"))
				if err != nil {
					t.Fatal(err)
				}
				_, err = client.TOTPEnrollConfirm(context.Background(), &herald.TOTPEnrollConfirmRequest{EnrollID: "synthetic-enrollment", Code: "123456"})
				var typed *herald.HeraldError
				if !errors.As(err, &typed) || typed.StatusCode != 429 {
					t.Fatalf("SDK error lost 429: %v", err)
				}
			}

			if response.Header.Get("Retry-After") != "" || response.Header.Get("X-Upstream-Secret") != "" || strings.Contains(string(body), "private-upstream") {
				t.Fatal("untrusted upstream response leaked")
			}
		})
	}
}
