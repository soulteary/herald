package provider

import (
	"context"
	"fmt"

	"github.com/soulteary/herald/internal/config"
)

// SMSProvider is a placeholder for SMS providers
// This can be extended to support Aliyun, Tencent, etc.
type SMSProvider struct {
	provider string
}

// NewSMSProvider creates a new SMS provider
func NewSMSProvider() *SMSProvider {
	return &SMSProvider{
		provider: config.SMSProvider,
	}
}

// Channel returns the channel type
func (p *SMSProvider) Channel() Channel {
	return ChannelSMS
}

// Validate checks if the provider is properly configured
func (p *SMSProvider) Validate() error {
	if p.provider == "" {
		return fmt.Errorf("SMS_PROVIDER is not configured")
	}
	
	// Validate provider-specific configs
	switch p.provider {
	case "aliyun":
		if config.AliyunAccessKey == "" || config.AliyunSecretKey == "" {
			return fmt.Errorf("Aliyun SMS credentials are not configured")
		}
	case "tencent":
		// Add Tencent validation when implemented
		return fmt.Errorf("Tencent SMS provider not yet implemented")
	default:
		return fmt.Errorf("unsupported SMS provider: %s", p.provider)
	}
	
	return nil
}

// Send sends an SMS message
// This is a placeholder implementation
func (p *SMSProvider) Send(ctx context.Context, msg *Message) error {
	if err := p.Validate(); err != nil {
		return err
	}

	// TODO: Implement actual SMS sending based on provider
	// For now, this is a placeholder that logs the message
	switch p.provider {
	case "aliyun":
		return p.sendAliyunSMS(ctx, msg)
	case "tencent":
		return fmt.Errorf("Tencent SMS provider not yet implemented")
	default:
		return fmt.Errorf("unsupported SMS provider: %s", p.provider)
	}
}

// sendAliyunSMS sends SMS via Aliyun
// This is a placeholder - actual implementation would use Aliyun SDK
func (p *SMSProvider) sendAliyunSMS(ctx context.Context, msg *Message) error {
	// TODO: Implement Aliyun SMS sending
	// Example:
	// client := aliyun.NewClient(config.AliyunAccessKey, config.AliyunSecretKey)
	// return client.SendSMS(ctx, &aliyun.SendSMSRequest{
	//     PhoneNumbers: msg.To,
	//     SignName:     config.AliyunSignName,
	//     TemplateCode: config.AliyunTemplateCode,
	//     TemplateParam: fmt.Sprintf(`{"code":"%s"}`, msg.Code),
	// })
	
	// Placeholder: just log for now
	fmt.Printf("[PLACEHOLDER] Would send SMS to %s with code: %s\n", msg.To, msg.Code)
	return nil
}

// FormatVerificationSMS formats a verification code SMS
func FormatVerificationSMS(code string, locale string) string {
	// Simple template - can be enhanced with i18n
	message := fmt.Sprintf("Your verification code is: %s. Valid for 5 minutes.", code)
	
	if locale == "zh-CN" || locale == "zh" {
		message = fmt.Sprintf("您的验证码是：%s，5分钟内有效。", code)
	}
	
	return message
}
