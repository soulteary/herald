package destination

import "testing"

func TestValidate_Email(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"User@Example.COM", "user@example.com", false},
		{"  a@b.co  ", "a@b.co", false},
		{"no-at-sign", "", true},
		{"@nolocal.com", "", true},
		{"local@", "", true},
		{"local@nodot", "", true},
		{"Foo <a@b.com>", "", true}, // display-name form rejected
		{"", "", true},
	}
	for _, c := range cases {
		got, err := Validate(ChannelEmail, c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Validate(email,%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Validate(email,%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Validate(email,%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidate_SMS(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"+1 (555) 123-4567", "+15551234567", false},
		{"+8613800138000", "+8613800138000", false},
		{"13800138000", "13800138000", false},
		{"12", "", true},                // too short
		{"+1234567890123456", "", true}, // too long
		{"+1-abc-def", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := Validate(ChannelSMS, c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Validate(sms,%q) expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Validate(sms,%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("Validate(sms,%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalize_VariantsCollapse proves that trivial variants of the same
// destination normalize to a single value so they cannot bypass rate limits or
// idempotency dedup.
func TestNormalize_VariantsCollapse(t *testing.T) {
	emailVariants := []string{"User@Example.com", "user@example.com", "  USER@EXAMPLE.COM "}
	first := Normalize(ChannelEmail, emailVariants[0])
	for _, v := range emailVariants[1:] {
		if got := Normalize(ChannelEmail, v); got != first {
			t.Errorf("email variant %q normalized to %q, want %q", v, got, first)
		}
	}

	smsVariants := []string{"+1 555 123 4567", "+1-555-123-4567", "+1(555)1234567"}
	firstSMS := Normalize(ChannelSMS, smsVariants[0])
	for _, v := range smsVariants[1:] {
		if got := Normalize(ChannelSMS, v); got != firstSMS {
			t.Errorf("sms variant %q normalized to %q, want %q", v, got, firstSMS)
		}
	}
}

func TestValidate_DingTalk(t *testing.T) {
	if _, err := Validate(ChannelDingTalk, "someuser"); err != nil {
		t.Errorf("unexpected error for valid dingtalk: %v", err)
	}
	long := make([]byte, MaxDingTalkLen+1)
	for i := range long {
		long[i] = 'a'
	}
	if _, err := Validate(ChannelDingTalk, string(long)); err == nil {
		t.Errorf("expected error for oversized dingtalk destination")
	}
}
