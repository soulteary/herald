package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// TemplateData represents the data available in templates
type TemplateData struct {
	Code      string
	ExpiresIn int // seconds
	Purpose   string
	Locale    string
}

// Manager handles template loading and rendering
type Manager struct {
	templates map[string]*template.Template // key: "locale:channel:purpose"
	dir       string
}

// NewManager creates a new template manager
func NewManager(templateDir string) *Manager {
	m := &Manager{
		templates: make(map[string]*template.Template),
		dir:       templateDir,
	}
	m.loadTemplates()
	return m
}

// loadTemplates loads templates from the template directory
func (m *Manager) loadTemplates() {
	if m.dir == "" {
		// Use built-in templates
		return
	}

	// Load templates from directory
	// Expected structure: templates/{locale}/{channel}/{purpose}.txt
	// Example: templates/zh-CN/email/login.txt
	_ = filepath.Walk(m.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		// Parse file path: {locale}/{channel}/{purpose}.txt
		relPath, _ := filepath.Rel(m.dir, path)
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) != 3 {
			return nil
		}

		locale := parts[0]
		channel := parts[1]
		purpose := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))

		key := fmt.Sprintf("%s:%s:%s", locale, channel, purpose)
		tmpl, err := template.ParseFiles(path)
		if err == nil {
			m.templates[key] = tmpl
		}

		return nil
	})
}

// Render renders a template with the given data
func (m *Manager) Render(locale, channel, purpose string, data TemplateData) (string, error) {
	// Try to find template: locale:channel:purpose
	key := fmt.Sprintf("%s:%s:%s", locale, channel, purpose)
	if tmpl, ok := m.templates[key]; ok {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String(), nil
		}
	}

	// Fallback to locale:channel:*
	key = fmt.Sprintf("%s:%s:*", locale, channel)
	if tmpl, ok := m.templates[key]; ok {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String(), nil
		}
	}

	// Fallback to built-in templates
	return m.renderBuiltIn(locale, channel, purpose, data)
}

// RenderEmail renders an email template and returns subject and body
func (m *Manager) RenderEmail(locale, purpose string, data TemplateData) (subject, body string, err error) {
	// Try to find template: locale:email:purpose
	key := fmt.Sprintf("%s:email:%s", locale, purpose)
	if tmpl, ok := m.templates[key]; ok {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err == nil {
			// For email, we expect the template to contain both subject and body
			// For simplicity, we'll use the full content as body and extract subject if possible
			content := buf.String()
			lines := strings.SplitN(content, "\n", 2)
			if len(lines) >= 2 {
				return lines[0], lines[1], nil
			}
			return "Verification Code", content, nil
		}
	}

	// Fallback to built-in templates
	subject, body = m.renderEmailBuiltIn(locale, purpose, data)
	return subject, body, nil
}

// RenderSMS renders an SMS template and returns the message body
func (m *Manager) RenderSMS(locale, purpose string, data TemplateData) (body string, err error) {
	// Try to find template: locale:sms:purpose
	key := fmt.Sprintf("%s:sms:%s", locale, purpose)
	if tmpl, ok := m.templates[key]; ok {
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err == nil {
			return buf.String(), nil
		}
	}

	// Fallback to built-in templates
	body = m.renderSMSBuiltIn(locale, purpose, data)
	return body, nil
}

// renderBuiltIn renders built-in templates
func (m *Manager) renderBuiltIn(locale, channel, purpose string, data TemplateData) (string, error) {
	switch channel {
	case "email":
		_, body := m.renderEmailBuiltIn(locale, purpose, data)
		return body, nil
	case "sms":
		return m.renderSMSBuiltIn(locale, purpose, data), nil
	default:
		return "", fmt.Errorf("unsupported channel: %s", channel)
	}
}

// renderEmailBuiltIn renders built-in email templates
func (m *Manager) renderEmailBuiltIn(locale, purpose string, data TemplateData) (subject, body string) {
	if strings.HasPrefix(locale, "zh") {
		subject = "验证码"
		body = fmt.Sprintf("您的验证码是：%s\n\n此验证码将在 %d 分钟后过期。", data.Code, data.ExpiresIn/60)
		if purpose != "login" {
			body = fmt.Sprintf("您的%s验证码是：%s\n\n此验证码将在 %d 分钟后过期。", getPurposeName(locale, purpose), data.Code, data.ExpiresIn/60)
		}
	} else {
		subject = "Verification Code"
		body = fmt.Sprintf("Your verification code is: %s\n\nThis code will expire in %d minutes.", data.Code, data.ExpiresIn/60)
		if purpose != "login" {
			body = fmt.Sprintf("Your %s verification code is: %s\n\nThis code will expire in %d minutes.", getPurposeName(locale, purpose), data.Code, data.ExpiresIn/60)
		}
	}
	return subject, body
}

// renderSMSBuiltIn renders built-in SMS templates
func (m *Manager) renderSMSBuiltIn(locale, purpose string, data TemplateData) string {
	if strings.HasPrefix(locale, "zh") {
		message := fmt.Sprintf("您的验证码是：%s，%d分钟内有效。", data.Code, data.ExpiresIn/60)
		if purpose != "login" {
			message = fmt.Sprintf("您的%s验证码是：%s，%d分钟内有效。", getPurposeName(locale, purpose), data.Code, data.ExpiresIn/60)
		}
		return message
	}
	message := fmt.Sprintf("Your verification code is: %s. Valid for %d minutes.", data.Code, data.ExpiresIn/60)
	if purpose != "login" {
		message = fmt.Sprintf("Your %s verification code is: %s. Valid for %d minutes.", getPurposeName(locale, purpose), data.Code, data.ExpiresIn/60)
	}
	return message
}

// getPurposeName returns the localized name for a purpose
func getPurposeName(locale, purpose string) string {
	if strings.HasPrefix(locale, "zh") {
		switch purpose {
		case "reset":
			return "重置密码"
		case "bind":
			return "绑定"
		case "stepup":
			return "二次验证"
		default:
			return "登录"
		}
	}
	switch purpose {
	case "reset":
		return "password reset"
	case "bind":
		return "binding"
	case "stepup":
		return "step-up authentication"
	default:
		return "login"
	}
}
