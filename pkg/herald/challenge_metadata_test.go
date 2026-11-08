package herald

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestChallengeRetryMetadata(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		header []string
		want   int
	}{
		{"old server", 429, `{"reason":"resend_cooldown"}`, nil, 0},
		{"body only", 429, `{"next_resend_in":120}`, nil, 120},
		{"header only", 429, `{}`, []string{"30"}, 30},
		{"matching", 429, `{"next_resend_in":60}`, []string{"60"}, 60},
		{"conflicting", 429, `{"next_resend_in":60}`, []string{"30"}, 0},
		{"negative", 429, `{"next_resend_in":-1}`, nil, 0},
		{"null", 429, `{"next_resend_in":null}`, nil, 0},
		{"fraction", 429, `{"next_resend_in":1.5}`, nil, 0},
		{"string", 429, `{"next_resend_in":"60"}`, nil, 0},
		{"oversized", 429, `{"next_resend_in":86401}`, nil, 0},
		{"integer overflow", 429, `{"next_resend_in":999999999999999999999}`, nil, 0},
		{"truncated", 429, `{"next_resend_in":`, []string{"60"}, 0},
		{"signed header", 429, `{}`, []string{"+60"}, 0},
		{"date header", 429, `{}`, []string{"Wed, 21 Oct 2026 07:28:00 GMT"}, 0},
		{"duplicate headers", 429, `{}`, []string{"60", "60"}, 0},
		{"other failure", 401, `{"next_resend_in":60}`, []string{"60"}, 0},
		{"success", 200, `{"next_resend_in":60}`, []string{"60"}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			headers := http.Header{}
			for _, value := range test.header {
				headers.Add("Retry-After", value)
			}
			got := challengeRetryMetadata(test.status, headers, []byte(test.body)).RetryAfterSeconds
			if test.want == 0 {
				if got != nil {
					t.Fatalf("unknown retry should remain absent: %d", *got)
				}
			} else if got == nil || *got != test.want {
				t.Fatalf("retry = %v, want %d", got, test.want)
			}
		})
	}
}

func TestCreateChallengeMetadataPreservesErrorsAndDoesNotRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/otp/challenges" {
			t.Error("unexpected creation request")
		}
		if r.Header.Get("X-API-Key") != "" || r.Header.Get("X-Signature-Version") != SignatureV3 {
			t.Error("HMAC credential selection changed")
		}
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"reason":"resend_cooldown","next_resend_in":120}`))
	}))
	defer server.Close()
	client, err := NewClient(DefaultOptions().WithBaseURL(server.URL).WithAPIKey("unused-api-key").WithHMACSecret("synthetic-hmac-secret"))
	if err != nil {
		t.Fatal(err)
	}
	request := &CreateChallengeRequest{UserID: "synthetic-user", Channel: "email", Destination: "test@example.com"}
	response, metadata, err := client.CreateChallengeWithMetadata(context.Background(), request)
	var structured *HeraldError
	if response != nil || !errors.As(err, &structured) || structured.StatusCode != 429 || structured.Reason != "resend_cooldown" || metadata.RetryAfterSeconds == nil || *metadata.RetryAfterSeconds != 120 {
		t.Fatalf("creation error contract lost: response=%v metadata=%v error=%v", response, metadata, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("creation retried %d times", calls.Load())
	}
	_, oldErr := client.CreateChallenge(context.Background(), request)
	if _, ok := oldErr.(*HeraldError); !ok || oldErr.Error() != err.Error() || calls.Load() != 2 {
		t.Fatal("legacy creation error or request count changed")
	}
}
