package template

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	provider "github.com/soulteary/provider-kit"
)

func writeEmailTemplate(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, "en", "email")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestEmailTemplateWithCRLFLineEndings loads an email template saved with
// CRLF line endings. Its subject must not keep the carriage return: the SMTP
// provider refuses a subject with a control character before it connects, so
// every message built from such a template failed with validation_failed.
func TestEmailTemplateWithCRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	writeEmailTemplate(t, dir, "login.txt", "Your sign-in code\r\nCode: {{.Code}}\r\nValid for {{.ExpiresIn}} seconds.\r\n")
	m := NewManager(dir)
	if problems := m.LoadProblems(); len(problems) != 0 {
		t.Fatalf("a CRLF template is valid and must load, got problems: %v", problems)
	}

	subject, body, err := m.RenderEmail("en", "login", TemplateData{Code: "123456", ExpiresIn: 300, Purpose: "login", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Your sign-in code" {
		t.Fatalf("subject = %q, want %q", subject, "Your sign-in code")
	}
	if !strings.HasPrefix(body, "Code: 123456") {
		t.Fatalf("body = %q", body)
	}

	// The provider validates the message before it dials. With a clean
	// subject the attempt gets as far as the connection, which is refused
	// here; it must not stop at validation.
	smtp, err := provider.NewSMTPProvider(&provider.SMTPConfig{Host: "127.0.0.1", Port: 1, From: "herald@example.com", UseStartTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := smtp.Send(context.Background(), provider.NewMessage("user@example.com").WithCode("123456").WithSubject(subject).WithBody(body))
	if result != nil && result.Error != nil && result.Error.Reason == provider.ReasonValidationFailed {
		t.Fatalf("the provider refused the rendered message: %v", result.Error)
	}
}

// TestEmailTemplateSubjectWithControlCharacterIsReported covers a subject the
// SMTP provider refuses whatever the line endings are. It is reported at startup
// and the built-in text is used, instead of failing on every send.
func TestEmailTemplateSubjectWithControlCharacterIsReported(t *testing.T) {
	dir := t.TempDir()
	writeEmailTemplate(t, dir, "login.txt", "Your sign-in\rcode\nCode: {{.Code}}\n")
	m := NewManager(dir)

	problems := m.LoadProblems()
	if len(problems) != 1 || !strings.Contains(problems[0], filepath.Join("en", "email", "login.txt")) ||
		!strings.Contains(problems[0], "subject") || !strings.Contains(problems[0], "U+000D") {
		t.Fatalf("want one problem naming the file, the subject and the character, got %v", problems)
	}

	subject, body, err := m.RenderEmail("en", "login", TemplateData{Code: "123456", ExpiresIn: 300, Purpose: "login", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(subject, "\r\n") || !strings.Contains(body, "123456") {
		t.Fatalf("built-in text was not used: subject %q, body %q", subject, body)
	}
}

// A one-line email template has no subject line and is unaffected.
func TestOneLineEmailTemplateKeepsDefaultSubject(t *testing.T) {
	dir := t.TempDir()
	writeEmailTemplate(t, dir, "login.txt", "Code: {{.Code}}")
	m := NewManager(dir)
	if problems := m.LoadProblems(); len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	subject, body, _ := m.RenderEmail("en", "login", TemplateData{Code: "123456"})
	if subject != "Verification Code" || body != "Code: 123456" {
		t.Fatalf("subject %q, body %q", subject, body)
	}
}
