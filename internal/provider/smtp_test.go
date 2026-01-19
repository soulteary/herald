package provider

import (
	"context"
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
				host: "smtp.example.com",
				port: 587,
				from: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "missing SMTP_HOST",
			provider: &SMTPProvider{
				host: "",
				port: 587,
				from: "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid SMTP_PORT (zero)",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 0,
				from: "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "invalid SMTP_PORT (too large)",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 65536,
				from: "test@example.com",
			},
			wantErr: true,
		},
		{
			name: "missing SMTP_FROM",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 587,
				from: "",
			},
			wantErr: true,
		},
		{
			name: "valid port 25",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 25,
				from: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid port 465",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 465,
				from: "test@example.com",
			},
			wantErr: false,
		},
		{
			name: "valid port 65535",
			provider: &SMTPProvider{
				host: "smtp.example.com",
				port: 65535,
				from: "test@example.com",
			},
			wantErr: false,
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
	tests := []struct {
		name     string
		provider *SMTPProvider
		msg      *Message
		wantErr  bool
	}{
		{
			name: "invalid configuration",
			provider: &SMTPProvider{
				host: "",
				port: 587,
				from: "test@example.com",
			},
			msg: &Message{
				To:      "recipient@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantErr: true,
		},
		{
			name: "valid configuration but connection will fail",
			provider: &SMTPProvider{
				host:     "invalid-smtp-server.example.com",
				port:     587,
				username: "user",
				password: "pass",
				from:     "test@example.com",
			},
			msg: &Message{
				To:      "recipient@example.com",
				Subject: "Test",
				Body:    "Test body",
			},
			wantErr: true, // Connection will fail
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
