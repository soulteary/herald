package template

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	i18n "github.com/soulteary/i18n-kit/v4"
	"github.com/soulteary/provider-kit"
)

// maxRenderBytes bounds the size of any single rendered message so a malformed
// or hostile template cannot produce an unbounded string that is then handed to
// a provider (SMS/email) or logged.
const maxRenderBytes = 16 * 1024

// errRenderTooLarge is returned when a template renders more than maxRenderBytes.
var errRenderTooLarge = fmt.Errorf("template: rendered output exceeds %d bytes", maxRenderBytes)

// boundedExecute executes a template into a size-capped buffer. It uses
// missingkey=error semantics via the caller's parsed template options so
// undefined fields fail instead of emitting "<no value>".
func boundedExecute(tmpl *template.Template, data TemplateData) (string, error) {
	lw := &limitedBuilder{limit: maxRenderBytes}
	if err := tmpl.Execute(lw, data); err != nil {
		if lw.overflow {
			return "", errRenderTooLarge
		}
		return "", err
	}
	return lw.b.String(), nil
}

// limitedBuilder is an io.Writer that stops accepting bytes past a limit and
// records that an overflow happened so callers can distinguish a size error
// from other execution errors.
type limitedBuilder struct {
	b        strings.Builder
	limit    int
	overflow bool
}

func (w *limitedBuilder) Write(p []byte) (int, error) {
	if w.b.Len()+len(p) > w.limit {
		w.overflow = true
		return 0, errRenderTooLarge
	}
	return w.b.Write(p)
}

// normalizeLocale canonicalizes a locale/alias to the canonical language tag
// string used for template keys, closing gaps where "zh", "zh_CN", "zh-cn" and
// "zh-CN" would otherwise miss the same template.
func normalizeLocale(locale string) string {
	lang, ok := i18n.ParseLanguage(locale)
	if !ok {
		return string(i18n.LangEN)
	}
	return string(lang)
}

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
	// loadProblems describes template files that were not loaded.
	loadProblems []string
}

// LoadProblems returns one description for every file or directory under the
// template directory that could not be loaded or can never be selected. The
// manager still works without them (it falls back to the built-in text), so
// the caller is expected to log these.
func (m *Manager) LoadProblems() []string {
	return append([]string(nil), m.loadProblems...)
}

func (m *Manager) loadProblem(format string, args ...interface{}) {
	m.loadProblems = append(m.loadProblems, fmt.Sprintf(format, args...))
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
		"email.body":              "您的验证码是： {code} \n\n此验证码将在 {minutes} 分钟后过期。",
		"email.body_with_purpose": "您的{purpose}验证码是： {code} \n\n此验证码将在 {minutes} 分钟后过期。",
		"sms.body":                "您的验证码是： {code} ，{minutes}分钟内有效。",
		"sms.body_with_purpose":   "您的{purpose}验证码是： {code} ，{minutes}分钟内有效。",
	})
}

// loadTemplates loads templates from the template directory
func (m *Manager) loadTemplates() {
	if m.dir == "" {
		// Use built-in templates
		return
	}

	// Load templates from directory
	// Expected structure: {TEMPLATE_DIR}/{locale}/{channel}/{purpose}.txt
	// Example: templates/zh-CN/email/login.txt
	//
	// The locale directory may be any spelling a request locale may use
	// ("zh", "zh-CN", "zh_cn"). It is stored under the canonical language,
	// the same form lookups use, so a region-tagged directory is found.
	_ = filepath.Walk(m.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			m.loadProblem("%s: %v", path, err)
			return nil
		}
		if info.IsDir() {
			return nil
		}

		// Parse file path: {locale}/{channel}/{purpose}.txt
		relPath, _ := filepath.Rel(m.dir, path)
		parts := strings.Split(relPath, string(filepath.Separator))
		if len(parts) != 3 {
			m.loadProblem("%s: ignored, expected {locale}/{channel}/{purpose}.txt", relPath)
			return nil
		}

		lang, ok := i18n.ParseLanguage(parts[0])
		if !ok {
			m.loadProblem("%s: ignored, %q is not a recognized locale", relPath, parts[0])
			return nil
		}
		channel := parts[1]
		if channel != "email" && channel != "sms" {
			m.loadProblem("%s: ignored, channel directory must be email or sms (dingtalk messages use the sms templates)", relPath)
			return nil
		}
		purpose := strings.TrimSuffix(parts[2], filepath.Ext(parts[2]))

		key := string(lang) + ":" + channel + ":" + purpose
		if _, exists := m.templates[key]; exists {
			m.loadProblem("%s: ignored, another directory already provides the %s template for language %q", relPath, channel+"/"+purpose, lang)
			return nil
		}
		// Strict mode: undefined fields error instead of silently emitting
		// "<no value>", surfacing template mistakes at render time.
		tmpl, err := template.New(filepath.Base(path)).Option("missingkey=error").ParseFiles(path)
		if err != nil {
			m.loadProblem("%s: not loaded, %v", relPath, err)
			return nil
		}
		if channel == "email" {
			if problem := emailSubjectProblem(tmpl); problem != "" {
				m.loadProblem("%s: not loaded, %s", relPath, problem)
				return nil
			}
		}
		m.templates[key] = tmpl

		return nil
	})
}

// splitEmailTemplate separates rendered email template output into the
// subject (the first line) and the body (the rest). A file saved with CRLF
// line endings ends its first line with a carriage return, which is not part
// of the subject. ok is false when the output is a single line, which has no
// subject.
func splitEmailTemplate(content string) (subject, body string, ok bool) {
	lines := strings.SplitN(content, "\n", 2)
	if len(lines) < 2 {
		return "", content, false
	}
	return strings.TrimSuffix(lines[0], "\r"), lines[1], true
}

// emailSubjectProblem renders an email template once with sample values and
// describes a subject line the SMTP provider refuses: one that contains a
// carriage return inside the line or a NUL. Such a template would fail on
// every send, so it is reported at startup and not loaded. A template that fails to render is not judged here; it falls back
// to the built-in text at request time, as before.
func emailSubjectProblem(tmpl *template.Template) string {
	content, err := boundedExecute(tmpl, TemplateData{Code: "000000", ExpiresIn: 300, Purpose: "login", Locale: "en"})
	if err != nil {
		return ""
	}
	subject, _, ok := splitEmailTemplate(content)
	if !ok {
		return ""
	}
	for _, r := range subject {
		if r == '\r' || r == 0 {
			return fmt.Sprintf("the subject (first line) contains the control character %U", r)
		}
	}
	return ""
}

// Render renders a template with the given data
func (m *Manager) Render(locale, channel, purpose string, data TemplateData) (string, error) {
	locale = normalizeLocale(locale)
	// Try to find template: locale:channel:purpose
	key := locale + ":" + channel + ":" + purpose
	if tmpl, ok := m.templates[key]; ok {
		if out, err := boundedExecute(tmpl, data); err == nil {
			return out, nil
		} else if err == errRenderTooLarge {
			return "", err
		}
	}

	// Fallback to locale:channel:*
	key = locale + ":" + channel + ":*"
	if tmpl, ok := m.templates[key]; ok {
		if out, err := boundedExecute(tmpl, data); err == nil {
			return out, nil
		} else if err == errRenderTooLarge {
			return "", err
		}
	}

	// Fallback to built-in templates
	return m.renderBuiltIn(locale, channel, purpose, data)
}

// RenderEmail renders an email template and returns subject and body
func (m *Manager) RenderEmail(locale, purpose string, data TemplateData) (subject, body string, err error) {
	locale = normalizeLocale(locale)
	// Try to find template: locale:email:purpose
	key := locale + ":email:" + purpose
	if tmpl, ok := m.templates[key]; ok {
		if content, rerr := boundedExecute(tmpl, data); rerr == nil {
			// For email, we expect the template to contain both subject and body
			// For simplicity, we'll use the full content as body and extract subject if possible
			if subject, body, ok := splitEmailTemplate(content); ok {
				return subject, body, nil
			}
			return "Verification Code", content, nil
		} else if rerr == errRenderTooLarge {
			return "", "", rerr
		}
	}

	// Fallback to built-in templates
	subject, body = m.renderEmailBuiltIn(locale, purpose, data)
	return subject, body, nil
}

// RenderSMS renders an SMS template and returns the message body
func (m *Manager) RenderSMS(locale, purpose string, data TemplateData) (body string, err error) {
	locale = normalizeLocale(locale)
	// Try to find template: locale:sms:purpose
	key := locale + ":sms:" + purpose
	if tmpl, ok := m.templates[key]; ok {
		if out, rerr := boundedExecute(tmpl, data); rerr == nil {
			return out, nil
		} else if rerr == errRenderTooLarge {
			return "", rerr
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
	case "sms", "dingtalk":
		// DingTalk uses same body style as SMS (plain text); herald-dingtalk receives body or params.code
		return m.renderSMSBuiltIn(locale, purpose, data), nil
	default:
		return "", fmt.Errorf("unsupported channel: %s", channel)
	}
}

// expiryMinutes converts a code lifetime in seconds to the whole minutes the
// built-in messages state. The texts only have a minutes placeholder, so a
// lifetime under one minute is stated as one minute: the smallest value that
// reads correctly, where truncation gave "0 minutes" and the login text
// substituted a fixed 5. Longer lifetimes are rounded down, never up.
func expiryMinutes(seconds int) int {
	if seconds < 60 {
		return 1
	}
	return seconds / 60
}

// renderEmailBuiltIn renders built-in email templates.
// For purpose "login", reuses provider-kit FormatVerificationEmailWithMinutes for simple verification copy.
func (m *Manager) renderEmailBuiltIn(locale, purpose string, data TemplateData) (subject, body string) {
	minutes := expiryMinutes(data.ExpiresIn)
	if purpose == "login" {
		return provider.FormatVerificationEmailWithMinutes(data.Code, locale, minutes)
	}
	lang := m.parseLanguage(locale)
	subject = m.bundle.GetTranslation(lang, "email.subject")
	params := map[string]interface{}{
		"code":    data.Code,
		"minutes": minutes,
	}
	purposeName := m.bundle.GetTranslation(lang, "purpose."+purpose)
	params["purpose"] = purposeName
	body = m.formatter.Format(lang, "email.body_with_purpose", params)
	return subject, body
}

// renderSMSBuiltIn renders built-in SMS templates.
// For purpose "login", reuses provider-kit FormatVerificationSMSWithMinutes for simple verification copy.
func (m *Manager) renderSMSBuiltIn(locale, purpose string, data TemplateData) string {
	minutes := expiryMinutes(data.ExpiresIn)
	if purpose == "login" {
		return provider.FormatVerificationSMSWithMinutes(data.Code, locale, minutes)
	}
	lang := m.parseLanguage(locale)
	params := map[string]interface{}{
		"code":    data.Code,
		"minutes": minutes,
	}
	purposeName := m.bundle.GetTranslation(lang, "purpose."+purpose)
	params["purpose"] = purposeName
	return m.formatter.Format(lang, "sms.body_with_purpose", params)
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
