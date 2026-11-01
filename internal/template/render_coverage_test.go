package template

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	texttemplate "text/template"
)

func TestCustomWildcardRenderingAndBounds(t *testing.T) {
	m := NewManager("")
	data := TemplateData{Code: "123456", ExpiresIn: 300, Purpose: "login"}
	m.templates["en:sms:*"] = texttemplate.Must(texttemplate.New("wildcard").Parse("code={{.Code}}"))
	if got, err := m.Render("en", "sms", "custom", data); err != nil || got != "code=123456" {
		t.Fatalf("wildcard = %q %v", got, err)
	}
	m.templates["en:sms:login"] = texttemplate.Must(texttemplate.New("sms").Parse("SMS {{.Code}}"))
	if got, err := m.RenderSMS("en", "login", data); err != nil || got != "SMS 123456" {
		t.Fatalf("SMS = %q %v", got, err)
	}
	m.templates["en:email:login"] = texttemplate.Must(texttemplate.New("email").Parse("single line {{.Code}}"))
	if subject, body, err := m.RenderEmail("en", "login", data); err != nil || subject != "Verification Code" || body != "single line 123456" {
		t.Fatalf("email = %q %q %v", subject, body, err)
	}
	large := texttemplate.Must(texttemplate.New("large").Parse(strings.Repeat("x", maxRenderBytes+1)))
	m.templates["en:sms:*"] = large
	if _, err := m.Render("en", "sms", "custom", data); !errors.Is(err, errRenderTooLarge) {
		t.Fatalf("wildcard overflow = %v", err)
	}
	m.templates["en:sms:login"], m.templates["en:email:login"] = large, large
	if _, err := m.RenderSMS("en", "login", data); !errors.Is(err, errRenderTooLarge) {
		t.Fatalf("SMS overflow = %v", err)
	}
	if _, _, err := m.RenderEmail("en", "login", data); !errors.Is(err, errRenderTooLarge) {
		t.Fatalf("email overflow = %v", err)
	}
	bad := texttemplate.Must(texttemplate.New("bad").Parse("{{.MissingField}}"))
	if _, err := boundedExecute(bad, data); err == nil {
		t.Fatal("invalid template field accepted")
	}
	dir := filepath.Join(t.TempDir(), "missing-directory")
	if manager := NewManager(dir); manager == nil {
		t.Fatal("missing templates should retain built-in fallback")
	}
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if manager := NewManager(file); manager == nil {
		t.Fatal("invalid path should retain fallback")
	}
}
