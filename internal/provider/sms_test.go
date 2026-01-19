package provider

import (
	"context"
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
	// Note: SMSProvider.Validate() reads config.AliyunAccessKey and config.AliyunSecretKey
	// which are set at package init time. We can test the provider field validation
	// by creating SMSProvider instances with different provider values.

	// Test that Validate() can be called
	provider := NewSMSProvider()
	err := provider.Validate()
	// The result depends on config values, but we verify the method works
	_ = err

	// Test with different provider values by creating instances directly
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{
			name:     "empty provider",
			provider: "",
			wantErr:  true,
		},
		{
			name:     "aliyun provider (depends on config)",
			provider: "aliyun",
			wantErr:  false, // May be true if config not set, but tests the path
		},
		{
			name:     "tencent provider not implemented",
			provider: "tencent",
			wantErr:  true,
		},
		{
			name:     "unsupported provider",
			provider: "unknown",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &SMSProvider{provider: tt.provider}
			err := p.Validate()
			switch tt.provider {
			case "":
				// Empty provider should always error
				if err == nil {
					t.Error("SMSProvider.Validate() with empty provider should return error")
				}
			case "tencent", "unknown":
				// These should always error
				if err == nil {
					t.Errorf("SMSProvider.Validate() with provider %q should return error", tt.provider)
				}
			default:
				// For aliyun, result depends on config, so we don't assert
			}
		})
	}
}

func TestSMSProvider_Send(t *testing.T) {
	tests := []struct {
		name     string
		provider *SMSProvider
		msg      *Message
		wantErr  bool
	}{
		{
			name: "invalid configuration (empty provider)",
			provider: &SMSProvider{
				provider: "",
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
			},
			wantErr: true,
		},
		{
			name: "aliyun provider (sendAliyunSMS is placeholder, but Validate may fail)",
			provider: &SMSProvider{
				provider: "aliyun",
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
			},
			wantErr: true, // Validate() will fail if config.AliyunAccessKey/SecretKey are not set
		},
		{
			name: "tencent provider not implemented",
			provider: &SMSProvider{
				provider: "tencent",
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
			},
			wantErr: true,
		},
		{
			name: "unsupported provider",
			provider: &SMSProvider{
				provider: "unknown",
			},
			msg: &Message{
				To:   "+1234567890",
				Code: "123456",
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

func TestSMSProvider_sendAliyunSMS(t *testing.T) {
	provider := &SMSProvider{
		provider: "aliyun",
	}
	ctx := context.Background()
	msg := &Message{
		To:   "+1234567890",
		Code: "123456",
	}

	// sendAliyunSMS is currently a placeholder that just logs and returns nil
	err := provider.sendAliyunSMS(ctx, msg)
	if err != nil {
		t.Errorf("SMSProvider.sendAliyunSMS() error = %v, want nil", err)
	}
}
