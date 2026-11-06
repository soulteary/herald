package serviceauth

import "testing"

func TestParseMode(t *testing.T) {
	for raw, want := range map[string]Mode{" HMAC_V2 ": ModeHMACV2, "hmac-v2": ModeHMACV2, "hmac": ModeHMACV2, "api_key": ModeAPIKey, "APIKEY": ModeAPIKey, "none": ModeNone, "off": ModeNone, "disabled": ModeNone} {
		got, err := ParseMode(raw)
		if err != nil || got != want {
			t.Fatalf("ParseMode(%q)=%q,%v", raw, got, err)
		}
	}
	for _, raw := range []string{"", "hmc", "api-key"} {
		if _, err := ParseMode(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestParseServicePolicies(t *testing.T) {
	p, err := ParseServicePolicies(" ")
	if err != nil || p != nil {
		t.Fatalf("empty=%v,%v", p, err)
	}
	raw := `{"key":{"service":"gateway","actions":["otp:create","otp:verify","otp:revoke","totp:status","totp:verify","totp:enroll","totp:revoke"]}}`
	p, err = ParseServicePolicies(raw)
	if err != nil || len(p["key"].Actions) != 7 {
		t.Fatalf("valid=%v,%v", p, err)
	}
	for _, raw := range []string{
		`{`, `null`, `{}`, `[]`, raw + ` {}`, `{"key":{"service":"s","actions":["otp:verify"],"typo":true}}`,
		`{"":{"service":"s","actions":["otp:verify"]}}`, `{" key":{"service":"s","actions":["otp:verify"]}}`,
		`{"x|kid:y":{"service":"s","actions":["otp:verify"]}}`, `{"k":{"service":"","actions":["otp:verify"]}}`,
		`{"k":{"service":" s","actions":["otp:verify"]}}`, `{"k":{"service":"s|kid:x","actions":["otp:verify"]}}`,
		`{"k":{"service":"s","actions":[]}}`, `{"k":{"service":"s","actions":["otp:verify","otp:verify"]}}`,
		`{"k":{"service":"s","actions":["unknown"]}}`,
	} {
		if _, err := ParseServicePolicies(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
