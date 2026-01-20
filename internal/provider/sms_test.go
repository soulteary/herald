package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewSMSProvider(t *testing.T) {
	// NewSMSProvider reads from config package which is initialized at package load time
	// So we test that it returns a non-nil provider
	provider := NewSMSProvider()
	if provider == nil {
		t.Fatal("NewSMSProvider() returned nil")
	}

	// Verify it's an SMSProvider by checking the type
	if provider.Channel() != ChannelSMS {
		t.Errorf("NewSMSProvider() channel = %v, want %v", provider.Channel(), ChannelSMS)
	}
}

func TestSMSProvider_Channel(t *testing.T) {
	provider := NewSMSProvider()
	if provider.Channel() != ChannelSMS {
		t.Errorf("SMSProvider.Channel() = %v, want %v", provider.Channel(), ChannelSMS)
	}
}

func TestSMSProvider_Validate(t *testing.T) {
	tests := []struct {
		name     string
		provider *SMSProvider
		wantErr  bool
	}{
		{
			name: "empty endpoint",
			provider: &SMSProvider{
				endpoint: "",
			},
			wantErr: true,
		},
		{
			name: "invalid endpoint scheme",
			provider: &SMSProvider{
				endpoint: "ftp://sms.example.com/send",
			},
			wantErr: true,
		},
		{
			name: "invalid endpoint format",
			provider: &SMSProvider{
				endpoint: "://bad-url",
			},
			wantErr: true,
		},
		{
			name: "valid endpoint",
			provider: &SMSProvider{
				endpoint: "https://sms.example.com/send",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.provider.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SMSProvider.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSMSProvider_Send(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %q", auth)
		}

		var payload struct {
			To      string `json:"to"`
			Message string `json:"message"`
			Code    string `json:"code,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if payload.To != "+1234567890" || payload.Code != "123456" {
			t.Errorf("unexpected payload: %+v", payload)
		}

		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer okServer.Close()

	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error":"bad request"}`))
	}))
	defer errorServer.Close()

	tests := []struct {
		name     string
		provider *SMSProvider
		msg      *Message
		wantErr  bool
	}{
		{
			name: "invalid configuration (empty endpoint)",
			provider: &SMSProvider{
				endpoint: "",
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
			},
			wantErr: true,
		},
		{
			name: "external API success",
			provider: &SMSProvider{
				endpoint: okServer.URL,
				apiKey:   "test-key",
				client:   okServer.Client(),
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
				Body: "Your verification code is: 123456",
			},
			wantErr: false,
		},
		{
			name: "external API failure",
			provider: &SMSProvider{
				endpoint: errorServer.URL,
				client:   errorServer.Client(),
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
				Body: "Your verification code is: 123456",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.provider.Send(ctx, tt.msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("SMSProvider.Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
