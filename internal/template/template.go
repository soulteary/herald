package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	i18n "github.com/soulteary/i18n-kit"
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
	bundle    *i18n.Bundle
	formatter *i18n.Formatter
}

// NewManager creates a new template manager
func NewManager(templateDir string) *Manager {
	bundle := i18n.NewBundle(i18n.LangEN)

	m := &Manager{
		templates: make(map[string]*template.Template),
		dir:       templateDir,
		bundle:    bundle,
		formatter: i18n.NewFormatter(bundle),
	}
	m.loadTranslations()
	m.loadTemplates()
	return m
}

// loadTranslations loads translations from the locales directory or uses built-in translations
func (m *Manager) loadTranslations() {
	// Try to load from locales directory
	localesDir := "locales"
	if m.dir != "" {
		// Check for locales relative to template directory
		parentDir := filepath.Dir(m.dir)
		localesDir = filepath.Join(parentDir, "locales")
	}

	if err := m.bundle.LoadDirectory(localesDir); err != nil {
		// Fallback to built-in translations if locales directory not found
		m.loadBuiltInTranslations()
	}
}

// loadBuiltInTranslations adds built-in translations as fallback
func (m *Manager) loadBuiltInTranslations() {
	// English translations
	m.bundle.AddTranslations(i18n.LangEN, map[string]string{
		"purpose.login":           "login",
		"purpose.reset":           "password reset",
		"purpose.bind":            "binding",
		"purpose.stepup":          "step-up authentication",
		"email.subject":           "Verification Code",
		"email.body":              "Your verification code is: {code}\n\nThis code will expire in {minutes} minutes.",
		"email.body_with_purpose": "Your {purpose} verification code is: {code}\n\nThis code will expire in {minutes} minutes.",
		"sms.body":                "Your verification code is: {code}. Valid for {minutes} minutes.",
		"sms.body_with_purpose":   "Your {purpose} verification code is: {code}. Valid for {minutes} minutes.",
	})

	// Chinese translations
	m.bundle.AddTranslations(i18n.LangZH, map[string]string{
		"purpose.login":           "登录",
		"purpose.reset":           "重置密码",
		"purpose.bind":            "绑定",
		"purpose.stepup":          "二次验证",
		"email.subject":           "验证码",
		"email.body":              "您的验证码是：{code}\n\n此验证码将在 {minutes} 分钟后过期。",
		"email.body_with_purpose": "您的{purpose}验证码是：{code}\n\n此验证码将在 {minutes} 分钟后过期。",
		"sms.body":                "您的验证码是：{code}，{minutes}分钟内有效。",
		"sms.body_with_purpose":   "您的{purpose}验证码是：{code}，{minutes}分钟内有效。",
	})
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
	lang := m.parseLanguage(locale)
	minutes := data.ExpiresIn / 60

	subject = m.bundle.GetTranslation(lang, "email.subject")

	params := map[string]interface{}{
		"code":    data.Code,
		"minutes": minutes,
	}

	if purpose != "login" {
		purposeName := m.bundle.GetTranslation(lang, "purpose."+purpose)
		params["purpose"] = purposeName
		body = m.formatter.Format(lang, "email.body_with_purpose", params)
	} else {
		body = m.formatter.Format(lang, "email.body", params)
	}

	return subject, body
}

// renderSMSBuiltIn renders built-in SMS templates
func (m *Manager) renderSMSBuiltIn(locale, purpose string, data TemplateData) string {
	lang := m.parseLanguage(locale)
	minutes := data.ExpiresIn / 60

	params := map[string]interface{}{
		"code":    data.Code,
		"minutes": minutes,
	}

	if purpose != "login" {
		purposeName := m.bundle.GetTranslation(lang, "purpose."+purpose)
		params["purpose"] = purposeName
		return m.formatter.Format(lang, "sms.body_with_purpose", params)
	}

	return m.formatter.Format(lang, "sms.body", params)
}

// parseLanguage parses a locale string to i18n.Language
func (m *Manager) parseLanguage(locale string) i18n.Language {
	lang, ok := i18n.ParseLanguage(locale)
	if !ok {
		return i18n.LangEN
	}
	return lang
}

// GetPurposeName returns the localized name for a purpose
func (m *Manager) GetPurposeName(locale, purpose string) string {
	lang := m.parseLanguage(locale)
	return m.bundle.GetTranslation(lang, "purpose."+purpose)
}

// GetBundle returns the i18n bundle for external use
func (m *Manager) GetBundle() *i18n.Bundle {
	return m.bundle
}
