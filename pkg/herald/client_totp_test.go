package herald

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTOTPTestClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	client, err := NewClient(DefaultOptions().WithBaseURL(baseURL).WithAPIKey("test-key"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestTOTPClientHappyPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-API-Key"); got != "test-key" {
			t.Errorf("X-API-Key = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/totp/status":
			if r.Method != http.MethodGet || r.URL.Query().Get("subject") != "user+1@example.com" {
				t.Errorf("unexpected status request: %s %s", r.Method, r.URL.String())
			}
			_ = json.NewEncoder(w).Encode(TOTPStatusResponse{Subject: "user+1@example.com", TotpEnabled: true})
		case "/v1/totp/verify":
			var req TOTPVerifyRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Subject != "user-1" || req.Code != "123456" {
				t.Errorf("verify request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(TOTPVerifyResponse{OK: true})
		case "/v1/totp/enroll/start":
			var req TOTPEnrollStartRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Subject != "user-1" || req.Label != "Work" {
				t.Errorf("start request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(TOTPEnrollStartResponse{EnrollID: "enroll-1", SecretBase32: "SECRET", OtpauthURI: "otpauth://totp/test"})
		case "/v1/totp/enroll/confirm":
			var req TOTPEnrollConfirmRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.EnrollID != "enroll-1" || req.Code != "654321" {
				t.Errorf("confirm request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(TOTPEnrollConfirmResponse{Subject: "user-1", TotpEnabled: true, BackupCodes: []string{"backup"}})
		case "/v1/totp/revoke":
			var req struct {
				Subject string `json:"subject"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Subject != "user-1" {
				t.Errorf("revoke request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(TOTPRevokeResponse{OK: true, Subject: "user-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTOTPTestClient(t, server.URL)
	ctx := context.Background()

	status, err := client.TOTPStatus(ctx, "user+1@example.com")
	if err != nil || !status.TotpEnabled {
		t.Fatalf("TOTPStatus: resp=%+v err=%v", status, err)
	}
	verify, err := client.TOTPVerify(ctx, &TOTPVerifyRequest{Subject: "user-1", Code: "123456"})
	if err != nil || !verify.OK {
		t.Fatalf("TOTPVerify: resp=%+v err=%v", verify, err)
	}
	start, err := client.TOTPEnrollStart(ctx, &TOTPEnrollStartRequest{Subject: "user-1", Label: "Work"})
	if err != nil || start.EnrollID != "enroll-1" {
		t.Fatalf("TOTPEnrollStart: resp=%+v err=%v", start, err)
	}
	confirm, err := client.TOTPEnrollConfirm(ctx, &TOTPEnrollConfirmRequest{EnrollID: "enroll-1", Code: "654321"})
	if err != nil || !confirm.TotpEnabled {
		t.Fatalf("TOTPEnrollConfirm: resp=%+v err=%v", confirm, err)
	}
	revoke, err := client.TOTPRevoke(ctx, "user-1")
	if err != nil || !revoke.OK {
		t.Fatalf("TOTPRevoke: resp=%+v err=%v", revoke, err)
	}
}

func TestTOTPClientProxyErrors(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
		reason string
	}{
		{"status", func(c *Client) error { _, err := c.TOTPStatus(context.Background(), "u"); return err }, "totp_proxy_failed"},
		{"verify", func(c *Client) error { _, err := c.TOTPVerify(context.Background(), &TOTPVerifyRequest{}); return err }, "denied"},
		{"enroll start", func(c *Client) error {
			_, err := c.TOTPEnrollStart(context.Background(), &TOTPEnrollStartRequest{})
			return err
		}, "totp_proxy_failed"},
		{"enroll confirm", func(c *Client) error {
			_, err := c.TOTPEnrollConfirm(context.Background(), &TOTPEnrollConfirmRequest{})
			return err
		}, "invalid"},
		{"revoke", func(c *Client) error { _, err := c.TOTPRevoke(context.Background(), "u"); return err }, "proxy_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`{"ok":false,"reason":"denied"}`))
			}))
			defer server.Close()
			err := tt.invoke(newTOTPTestClient(t, server.URL))
			var heraldErr *HeraldError
			if !errors.As(err, &heraldErr) {
				t.Fatalf("error = %v, want HeraldError", err)
			}
			if heraldErr.StatusCode != http.StatusBadGateway || heraldErr.Reason != tt.reason {
				t.Fatalf("error = %+v, want status 502 reason %q", heraldErr, tt.reason)
			}
		})
	}
}

func TestTOTPClientInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		invoke func(*Client) error
	}{
		{"status", func(c *Client) error { _, err := c.TOTPStatus(context.Background(), "u"); return err }},
		{"enroll start", func(c *Client) error {
			_, err := c.TOTPEnrollStart(context.Background(), &TOTPEnrollStartRequest{})
			return err
		}},
		{"enroll confirm", func(c *Client) error {
			_, err := c.TOTPEnrollConfirm(context.Background(), &TOTPEnrollConfirmRequest{})
			return err
		}},
		{"revoke", func(c *Client) error { _, err := c.TOTPRevoke(context.Background(), "u"); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("not-json"))
			}))
			defer server.Close()
			err := tt.invoke(newTOTPTestClient(t, server.URL))
			var heraldErr *HeraldError
			if !errors.As(err, &heraldErr) || heraldErr.Reason != "invalid_response" {
				t.Fatalf("error = %v, want invalid_response HeraldError", err)
			}
		})
	}
}

func TestTOTPClientResponseLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"subject":"a response that is too large"}`))
	}))
	defer server.Close()
	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL).WithMaxResponseBytes(8))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = client.TOTPStatus(context.Background(), "u")
	var heraldErr *HeraldError
	if !errors.As(err, &heraldErr) || heraldErr.Reason != "response_too_large" {
		t.Fatalf("error = %v, want response_too_large", err)
	}
}
