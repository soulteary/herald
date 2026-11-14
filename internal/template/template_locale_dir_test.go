package template

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemplate(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRegionTaggedTemplateDirectoryIsSelected stores templates under
// "zh-CN", the layout the package documents, and requests them with the
// locale spellings callers send. Lookups normalize the locale to its language
// ("zh"), so the directory name has to be normalized the same way.
func TestRegionTaggedTemplateDirectoryIsSelected(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "zh-CN/sms/login.txt", "自定义短信 {{.Code}}")
	writeTemplate(t, dir, "zh-CN/email/login.txt", "自定义主题\n自定义正文 {{.Code}}")
	writeTemplate(t, dir, "en_US/sms/login.txt", "custom sms {{.Code}}")

	m := NewManager(dir)
	data := TemplateData{Code: "246810", ExpiresIn: 300, Purpose: "login"}

	for _, locale := range []string{"zh-CN", "zh", "zh_CN", "zh-cn"} {
		body, err := m.RenderSMS(locale, "login", data)
		if err != nil || body != "自定义短信 246810" {
			t.Errorf("RenderSMS(%q) = %q, %v; want the custom zh-CN template", locale, body, err)
		}
		subject, emailBody, err := m.RenderEmail(locale, "login", data)
		if err != nil || subject != "自定义主题" || emailBody != "自定义正文 246810" {
			t.Errorf("RenderEmail(%q) = %q, %q, %v; want the custom zh-CN template", locale, subject, emailBody, err)
		}
	}
	for _, locale := range []string{"en", "en-US", ""} {
		if body, err := m.RenderSMS(locale, "login", data); err != nil || body != "custom sms 246810" {
			t.Errorf("RenderSMS(%q) = %q, %v; want the custom en_US template", locale, body, err)
		}
	}
}
