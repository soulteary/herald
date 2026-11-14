package template

import (
	"path/filepath"
	"strings"
	"testing"
)

func problemFor(problems []string, fragment string) string {
	for _, p := range problems {
		if strings.Contains(p, fragment) {
			return p
		}
	}
	return ""
}

// TestTemplateLoadProblemsAreReported checks that every file the loader
// skips is described, instead of being dropped without a trace while the
// built-in text is sent.
func TestTemplateLoadProblemsAreReported(t *testing.T) {
	dir := t.TempDir()
	writeTemplate(t, dir, "en/sms/login.txt", "ok {{.Code}}")
	writeTemplate(t, dir, "en/sms/reset.txt", "broken {{.Code")                // parse error
	writeTemplate(t, dir, "en/login.txt", "wrong depth")                       // layout
	writeTemplate(t, dir, "klingon/sms/login.txt", "unknown locale {{.Code}}") // locale
	writeTemplate(t, dir, "en/dingtalk/login.txt", "never selected")           // channel
	writeTemplate(t, dir, "en-US/sms/login.txt", "duplicate {{.Code}}")        // same language as en/

	m := NewManager(dir)
	problems := m.LoadProblems()
	for fragment, want := range map[string]string{
		filepath.Join("en", "sms", "reset.txt"):      "not loaded",
		filepath.Join("en", "login.txt"):             "expected {locale}/{channel}/{purpose}.txt",
		filepath.Join("klingon", "sms", "login.txt"): "not a recognized locale",
		filepath.Join("en", "dingtalk", "login.txt"): "channel directory must be email or sms",
		filepath.Join("en-US", "sms", "login.txt"):   "another directory already provides",
	} {
		problem := problemFor(problems, fragment)
		if problem == "" || !strings.Contains(problem, want) {
			t.Errorf("no problem for %s containing %q; got %q (all: %v)", fragment, want, problem, problems)
		}
	}
	if len(problems) != 5 {
		t.Errorf("want 5 problems, got %d: %v", len(problems), problems)
	}

	data := TemplateData{Code: "135790", ExpiresIn: 300, Purpose: "login"}
	// The valid template still loads, and "en" wins over "en-US".
	if body, err := m.RenderSMS("en", "login", data); err != nil || body != "ok 135790" {
		t.Errorf("RenderSMS = %q, %v", body, err)
	}
	// The broken one falls back to the built-in text.
	data.Purpose = "reset"
	if body, err := m.RenderSMS("en", "reset", data); err != nil || !strings.Contains(body, "135790") || strings.Contains(body, "broken") {
		t.Errorf("RenderSMS(reset) = %q, %v", body, err)
	}
}

func TestMissingTemplateDirectoryIsReported(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")
	m := NewManager(dir)
	problems := m.LoadProblems()
	if len(problems) != 1 || !strings.Contains(problems[0], dir) {
		t.Fatalf("want one problem naming %s, got %v", dir, problems)
	}
	if len(NewManager("").LoadProblems()) != 0 {
		t.Fatal("no template directory is not a problem")
	}
}
