package provider

import (
	"context"
	"errors"
	"testing"
)

// mockProvider is a test implementation of Provider
type mockProvider struct {
	channel   Channel
	valid     bool
	sendError error
}

func (m *mockProvider) Send(ctx context.Context, msg *Message) error {
	return m.sendError
}

func (m *mockProvider) Channel() Channel {
	return m.channel
}

func (m *mockProvider) Validate() error {
	if !m.valid {
		return errors.New("provider validation failed")
	}
	return nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
	}
	if registry.providers == nil {
		t.Fatal("NewRegistry() providers map is nil")
	}
	if len(registry.providers) != 0 {
		t.Errorf("NewRegistry() providers map should be empty, got %d", len(registry.providers))
	}
}

func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name      string
		provider  Provider
		wantErr   bool
		wantCount int
	}{
		{
			name: "valid provider",
			provider: &mockProvider{
				channel: ChannelEmail,
				valid:   true,
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "invalid provider",
			provider: &mockProvider{
				channel: ChannelSMS,
				valid:   false,
			},
			wantErr:   true,
			wantCount: 0,
		},
		{
			name: "register multiple providers",
			provider: &mockProvider{
				channel: ChannelEmail,
				valid:   true,
			},
			wantErr:   false,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()

			err := registry.Register(tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("Register() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(registry.providers) != tt.wantCount {
				t.Errorf("Register() providers count = %d, want %d", len(registry.providers), tt.wantCount)
			}
		})
	}
}

func TestRegistry_Register_Overwrite(t *testing.T) {
	registry := NewRegistry()

	provider1 := &mockProvider{
		channel: ChannelEmail,
		valid:   true,
	}

	provider2 := &mockProvider{
		channel: ChannelEmail,
		valid:   true,
	}

	if err := registry.Register(provider1); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := registry.Register(provider2); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Should overwrite the first provider
	if len(registry.providers) != 1 {
		t.Errorf("Register() should overwrite existing provider, got %d providers", len(registry.providers))
	}
}

func TestRegistry_GetProvider(t *testing.T) {
	registry := NewRegistry()

	emailProvider := &mockProvider{
		channel: ChannelEmail,
		valid:   true,
	}

	smsProvider := &mockProvider{
		channel: ChannelSMS,
		valid:   true,
	}

	if err := registry.Register(emailProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if err := registry.Register(smsProvider); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	tests := []struct {
		name    string
		channel Channel
		wantErr bool
	}{
		{
			name:    "get email provider",
			channel: ChannelEmail,
			wantErr: false,
		},
		{
			name:    "get sms provider",
			channel: ChannelSMS,
			wantErr: false,
		},
		{
			name:    "get non-existent provider",
			channel: Channel("unknown"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := registry.GetProvider(tt.channel)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && provider == nil {
				t.Error("GetProvider() returned nil provider")
			}
			if !tt.wantErr && provider.Channel() != tt.channel {
				t.Errorf("GetProvider() channel = %v, want %v", provider.Channel(), tt.channel)
			}
		})
	}
}

func TestRegistry_Send(t *testing.T) {
	registry := NewRegistry()

	msg := &Message{
		To:   "test@example.com",
		Code: "123456",
		Body: "Test message",
	}

	tests := []struct {
		name      string
		channel   Channel
		sendError error
		wantErr   bool
	}{
		{
			name:      "send via registered provider",
			channel:   ChannelEmail,
			sendError: nil,
			wantErr:   false,
		},
		{
			name:      "send via non-existent provider",
			channel:   ChannelSMS,
			sendError: nil,
			wantErr:   true,
		},
		{
			name:      "provider send error",
			channel:   ChannelEmail,
			sendError: errors.New("send failed"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset registry for each test
			_ = NewRegistry()

			if tt.channel == ChannelEmail {
				provider := &mockProvider{
					channel:   ChannelEmail,
					valid:     true,
					sendError: tt.sendError,
				}
				if err := registry.Register(provider); err != nil {
					t.Fatalf("Register() error = %v", err)
				}
			}

			err := registry.Send(context.Background(), tt.channel, msg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFormatVerificationEmail(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		locale   string
		wantSubj string
		wantBody string
	}{
		{
			name:     "default locale",
			code:     "123456",
			locale:   "en",
			wantSubj: "Verification Code",
			wantBody: "Your verification code is: 123456\n\nThis code will expire in 5 minutes.",
		},
		{
			name:     "chinese locale",
			code:     "123456",
			locale:   "zh-CN",
			wantSubj: "验证码",
			wantBody: "您的验证码是：123456\n\n此验证码将在5分钟后过期。",
		},
		{
			name:     "zh locale prefix",
			code:     "654321",
			locale:   "zh",
			wantSubj: "验证码",
			wantBody: "您的验证码是：654321\n\n此验证码将在5分钟后过期。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subject, body := FormatVerificationEmail(tt.code, tt.locale)
			if subject != tt.wantSubj {
				t.Errorf("FormatVerificationEmail() subject = %v, want %v", subject, tt.wantSubj)
			}
			if body != tt.wantBody {
				t.Errorf("FormatVerificationEmail() body = %v, want %v", body, tt.wantBody)
			}
		})
	}
}

func TestFormatVerificationSMS(t *testing.T) {
	tests := []struct {
		name   string
		code   string
		locale string
		want   string
	}{
		{
			name:   "default locale",
			code:   "123456",
			locale: "en",
			want:   "Your verification code is: 123456. Valid for 5 minutes.",
		},
		{
			name:   "chinese locale",
			code:   "123456",
			locale: "zh-CN",
			want:   "您的验证码是：123456，5分钟内有效。",
		},
		{
			name:   "zh locale",
			code:   "654321",
			locale: "zh",
			want:   "您的验证码是：654321，5分钟内有效。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatVerificationSMS(tt.code, tt.locale); got != tt.want {
				t.Errorf("FormatVerificationSMS() = %v, want %v", got, tt.want)
			}
		})
	}
}
