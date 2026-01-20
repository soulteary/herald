package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/soulteary/herald/internal/config"
)

// SMSProvider sends messages via an external API placeholder.
type SMSProvider struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewSMSProvider creates a new SMS provider
func NewSMSProvider() *SMSProvider {
	return &SMSProvider{
		endpoint: config.SMSAPIURL,
		apiKey:   config.SMSAPIKey,
		client:   newHTTPClient(config.ProviderTimeout),
	}
}

// Channel returns the channel type
func (p *SMSProvider) Channel() Channel {
	return ChannelSMS
}

// Validate checks if the provider is properly configured
func (p *SMSProvider) Validate() error {
	return validateEndpoint(p.endpoint)
}

// Send sends an SMS message
// This is a placeholder implementation
func (p *SMSProvider) Send(ctx context.Context, msg *Message) error {
	if err := p.Validate(); err != nil {
		return err
	}

	payload := struct {
		To      string `json:"to"`
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	}{
		To:      msg.To,
		Message: msg.Body,
		Code:    msg.Code,
	}

	if p.client == nil {
		p.client = newHTTPClient(config.ProviderTimeout)
	}

	return postJSON(ctx, p.client, p.endpoint, p.apiKey, payload)
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
