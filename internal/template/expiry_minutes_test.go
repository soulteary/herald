package template

import (
	"strings"
	"testing"
)

// TestBuiltInMessagesStateSubMinuteExpiryAsOneMinute renders every built-in
// text for a 30-second code. The login texts used to say 5 minutes and the
// other purposes 0 minutes.
func TestBuiltInMessagesStateSubMinuteExpiryAsOneMinute(t *testing.T) {
	m := NewManager("")
	for _, locale := range []string{"en", "zh-CN"} {
		for _, purpose := range []string{"login", "reset"} {
			data := TemplateData{Code: "864209", ExpiresIn: 30, Purpose: purpose, Locale: locale}
			sms, err := m.RenderSMS(locale, purpose, data)
			if err != nil {
				t.Fatal(err)
			}
			_, email, err := m.RenderEmail(locale, purpose, data)
			if err != nil {
				t.Fatal(err)
			}
			for kind, text := range map[string]string{"sms": sms, "email": email} {
				// Drop the code so its digits cannot satisfy or trip the checks.
				rest := strings.ReplaceAll(text, "864209", "")
				if !strings.Contains(rest, "1") {
					t.Errorf("%s/%s/%s does not state 1 minute: %q", locale, purpose, kind, text)
				}
				if strings.ContainsAny(rest, "05") {
					t.Errorf("%s/%s/%s states 0 or 5 minutes for a 30-second code: %q", locale, purpose, kind, text)
				}
			}
		}
	}
}

// TestBuiltInMessagesKeepWholeMinutes checks the usual five-minute lifetime
// still renders as 5 for every purpose.
func TestBuiltInMessagesKeepWholeMinutes(t *testing.T) {
	m := NewManager("")
	for _, purpose := range []string{"login", "reset"} {
		data := TemplateData{Code: "864209", ExpiresIn: 300, Purpose: purpose, Locale: "en"}
		sms, err := m.RenderSMS("en", purpose, data)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sms, "5 minutes") {
			t.Errorf("%s: %q", purpose, sms)
		}
	}
}

func TestExpiryMinutes(t *testing.T) {
	for seconds, want := range map[int]int{0: 1, 1: 1, 30: 1, 59: 1, 60: 1, 90: 1, 119: 1, 120: 2, 300: 5, 600: 10} {
		if got := expiryMinutes(seconds); got != want {
			t.Errorf("expiryMinutes(%d) = %d, want %d", seconds, got, want)
		}
	}
}
