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
// This is a placeholder implementation following external API protocol
func (p *SMSProvider) Send(ctx context.Context, msg *Message) error {
	if err := p.Validate(); err != nil {
		return err
	}

	// Build params map
	params := make(map[string]interface{})
	if msg.Params != nil {
		params = msg.Params
	}
	if msg.Code != "" {
		params["code"] = msg.Code
	}
	if msg.Body != "" {
		params["message"] = msg.Body
	}

	// Determine template
	template := msg.Template
	if template == "" {
		template = "verification_sms"
	}

	payload := struct {
		Channel        string                 `json:"channel"`
		To             string                 `json:"to"`
		Template       string                 `json:"template"`
		Params         map[string]interface{} `json:"params"`
		Locale         string                 `json:"locale,omitempty"`
		IdempotencyKey string                 `json:"idempotency_key,omitempty"`
		TimeoutSeconds int                    `json:"timeout_seconds,omitempty"`
	}{
		Channel:        "sms",
		To:             msg.To,
		Template:       template,
		Params:         params,
		Locale:         msg.Locale,
		IdempotencyKey: msg.IdempotencyKey,
		TimeoutSeconds: int(config.ProviderTimeout.Seconds()),
	}

	if p.client == nil {
		p.client = newHTTPClient(config.ProviderTimeout)
	}

	return postJSON(ctx, p.client, p.endpoint, p.apiKey, payload, msg.Traceparent, msg.Tracestate)
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
