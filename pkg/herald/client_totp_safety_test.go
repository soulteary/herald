package herald

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTOTPRevokeRequiresConfirmation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"subject":"user-1"}`))
	}))
	defer server.Close()
	client := newTOTPTestClient(t, server.URL)

	resp, err := client.TOTPRevoke(context.Background(), "user-1")
	var heraldErr *HeraldError
	if !errors.As(err, &heraldErr) || heraldErr.Reason != "revoke_not_confirmed" {
		t.Fatalf("TOTPRevoke error = %v, want reason revoke_not_confirmed", err)
	}
	if resp == nil || resp.OK {
		t.Fatalf("TOTPRevoke response = %+v, want the decoded unconfirmed response", resp)
	}
}

// An undecodable TOTP response must not be quoted in the error: an enrollment
// response contains the shared secret.
func TestTOTPUndecodableResponseIsNotQuoted(t *testing.T) {
	const secret = "otpauth://totp/Work:user-1?secret=JBSWY3DPEHPK3PXP"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"otpauth_uri":"` + secret + `"`)) // truncated JSON
	}))
	defer server.Close()
	client := newTOTPTestClient(t, server.URL)
	ctx := context.Background()

	calls := map[string]func() error{
		"status": func() error { _, err := client.TOTPStatus(ctx, "user-1"); return err },
		"enroll start": func() error {
			_, err := client.TOTPEnrollStart(ctx, &TOTPEnrollStartRequest{Subject: "user-1"})
			return err
		},
		"enroll confirm": func() error {
			_, err := client.TOTPEnrollConfirm(ctx, &TOTPEnrollConfirmRequest{EnrollID: "e", Code: "123456"})
			return err
		},
		"revoke": func() error { _, err := client.TOTPRevoke(ctx, "user-1"); return err },
	}
	for name, call := range calls {
		err := call()
		var heraldErr *HeraldError
		if !errors.As(err, &heraldErr) || heraldErr.Reason != "invalid_response" {
			t.Fatalf("%s: error = %v, want reason invalid_response", name, err)
		}
		if strings.Contains(err.Error(), "JBSWY3DPEHPK3PXP") || strings.Contains(heraldErr.Message, "otpauth") {
			t.Fatalf("%s: error quotes the response body: %v", name, err)
		}
	}
}
