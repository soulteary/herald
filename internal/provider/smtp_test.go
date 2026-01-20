package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSMTPProvider(t *testing.T) {
	// NewSMTPProvider reads from config package which is initialized at package load time
	// So we test that it returns a non-nil provider
	provider := NewSMTPProvider()
	if provider == nil {
		t.Fatal("NewSMTPProvider() returned nil")
	}

	// Verify it's an SMTPProvider by checking the type
	if provider.Channel() != ChannelEmail {
		t.Errorf("NewSMTPProvider() channel = %v, want %v", provider.Channel(), ChannelEmail)
	}
}

func TestSMTPProvider_Channel(t *testing.T) {
	provider := NewSMTPProvider()
	if provider.Channel() != ChannelEmail {
		t.Errorf("SMTPProvider.Channel() = %v, want %v", provider.Channel(), ChannelEmail)
	}
}

func TestSMTPProvider_Validate(t *testing.T) {
	tests := []struct {
		name     string
		provider *SMTPProvider
		wantErr  bool
	}{
		{
			name: "valid configuration",
			provider: &SMTPProvider{
				endpoint: "https://email.example.com/send",
				from:     "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing EMAIL_API_URL",
			provider: &SMTPProvider{
				endpoint: "",
				from:     "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid endpoint scheme",
			provider: &SMTPProvider{
				endpoint: "ftp://email.example.com/send",
				from:     "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid endpoint format",
			provider: &SMTPProvider{
				endpoint: "://bad-url",
				from:     "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing EMAIL_FROM",
			provider: &SMTPProvider{
				endpoint: "https://email.example.com/send",
				from:     "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SMTPProvider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMTPProvider_Send(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %q", auth)
		}

		var payload struct {
			To      string `json:"to"`
			From    string `json:"from,omitempty"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
			Code    string `json:"code,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if payload.To != "recipient@example.com" || payload.Subject != "Test" {
			t.Errorf("unexpected payload: %+v", payload)
		}

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer okServer.Close()

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error":"server error"}`))
	}))
	defer errorServer.Close()

	tests := []struct {
		name     string
		provider *SMTPProvider
		msg      *Message
		wantErr  bool
	}{
		{
			name: "invalid configuration",
			provider: &SMTPProvider{
				endpoint: "",
				from:     "test@example.com",
			},
			msg: &Message{
				To:      "recipient@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantErr: true,
		},
		{
			name: "external API success",
			provider: &SMTPProvider{
				endpoint: okServer.URL,
				apiKey:   "test-key",
				from:     "test@example.com",
				client:   okServer.Client(),
			},
			msg: &Message{
				To:      "recipient@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantErr: false,
		},
		{
			name: "external API failure",
			provider: &SMTPProvider{
				endpoint: errorServer.URL,
				from:     "test@example.com",
				client:   errorServer.Client(),
			},
			msg: &Message{
				To:      "recipient@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.provider.Send(ctx, tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("SMTPProvider.Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
