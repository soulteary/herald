package provider

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/soulteary/herald/internal/config"
)

// SMTPProvider implements email sending via SMTP
type SMTPProvider struct {
	host     string
	port     int
	username string
	password string
	from     string
}

// NewSMTPProvider creates a new SMTP provider
func NewSMTPProvider() *SMTPProvider {
	return &SMTPProvider{
		host:     config.SMTPHost,
		port:     config.SMTPPort,
		username: config.SMTPUser,
		password: config.SMTPPassword,
		from:     config.SMTPFrom,
	}
}

// Channel returns the channel type
func (p *SMTPProvider) Channel() Channel {
	return ChannelEmail
}

// Validate checks if the provider is properly configured
func (p *SMTPProvider) Validate() error {
	if p.host == "" {
		return fmt.Errorf("SMTP_HOST is not configured")
	}
	if p.port <= 0 || p.port > 65535 {
		return fmt.Errorf("SMTP_PORT is invalid")
	}
	if p.from == "" {
		return fmt.Errorf("SMTP_FROM is not configured")
	}
	return nil
}

// Send sends an email via SMTP
func (p *SMTPProvider) Send(ctx context.Context, msg *Message) error {
	if err := p.Validate(); err != nil {
		return err
	}

	// Build email message
	emailBody := fmt.Sprintf("From: %s\r\n", p.from)
	emailBody += fmt.Sprintf("To: %s\r\n", msg.To)
	emailBody += fmt.Sprintf("Subject: %s\r\n", msg.Subject)
	emailBody += "Content-Type: text/plain; charset=UTF-8\r\n"
	emailBody += "\r\n"
	emailBody += msg.Body

	// SMTP authentication
	auth := smtp.PlainAuth("", p.username, p.password, p.host)

	// Send email
	addr := fmt.Sprintf("%s:%d", p.host, p.port)
	err := smtp.SendMail(addr, auth, p.from, []string{msg.To}, []byte(emailBody))
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
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
