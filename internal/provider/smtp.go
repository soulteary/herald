package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/soulteary/herald/internal/config"
)

// SMTPProvider implements email sending via an external API placeholder.
type SMTPProvider struct {
	endpoint string
	apiKey   string
	from     string
	client   *http.Client
}

// NewSMTPProvider creates a new SMTP provider
func NewSMTPProvider() *SMTPProvider {
	return &SMTPProvider{
		endpoint: config.EmailAPIURL,
		apiKey:   config.EmailAPIKey,
		from:     config.EmailFrom,
		client:   newHTTPClient(config.ProviderTimeout),
	}
}

// Channel returns the channel type
func (p *SMTPProvider) Channel() Channel {
	return ChannelEmail
}

// Validate checks if the provider is properly configured
func (p *SMTPProvider) Validate() error {
	if err := validateEndpoint(p.endpoint); err != nil {
		return err
	}
	if p.from == "" {
		return fmt.Errorf("EMAIL_FROM is not configured")
	}
	return nil
}

// Send sends an email via SMTP
func (p *SMTPProvider) Send(ctx context.Context, msg *Message) error {
	if err := p.Validate(); err != nil {
		return err
	}

	payload := struct {
		To      string `json:"to"`
		From    string `json:"from,omitempty"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		Code    string `json:"code,omitempty"`
	}{
		To:      msg.To,
		From:    p.from,
		Subject: msg.Subject,
		Body:    msg.Body,
		Code:    msg.Code,
	}

	if p.client == nil {
		p.client = newHTTPClient(config.ProviderTimeout)
	}

	return postJSON(ctx, p.client, p.endpoint, p.apiKey, payload)
}

// FormatVerificationEmail formats a verification code email
func FormatVerificationEmail(code string, locale string) (subject, body string) {
	// Simple template - can be enhanced with i18n
	subject = "Verification Code"
	body = fmt.Sprintf("Your verification code is: %s\n\nThis code will expire in 5 minutes.", code)

	// Add locale-specific formatting if needed
	if strings.HasPrefix(locale, "zh") {
		subject = "验证码"
		body = fmt.Sprintf("您的验证码是：%s\n\n此验证码将在5分钟后过期。", code)
	}

	return subject, body
}
