package config

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestBooleanValueProblems(t *testing.T) {
	env := map[string]string{
		"REDIS_TLS_ENABLED":      "yes",
		"AUDIT_MASK_DESTINATION": "true ",
		"OTLP_ENABLED":           "false",
		"HMAC_V1_ENABLED":        "1",
	}
	problems := booleanValueProblems(func(name string) string { return env[name] })
	if len(problems) != 2 {
		t.Fatalf("got %d problems, want 2: %v", len(problems), problems)
	}
	if problems := booleanValueProblems(func(string) string { return "" }); len(problems) != 0 {
		t.Fatalf("unset variables must not be reported: %v", problems)
	}
}

// booleanEnvNames must list every env.GetBool call so a new boolean cannot be
// added without the strict check.
func TestBooleanEnvNamesCoversEveryGetBool(t *testing.T) {
	re := regexp.MustCompile(`env\.GetBool\("([A-Z0-9_]+)"`)
	found := map[string]bool{}
	for _, file := range []string{"config.go", "redis.go", "strict.go"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			found[m[1]] = true
		}
	}
	var got []string
	for name := range found {
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), booleanEnvNames...)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("env.GetBool names %v, booleanEnvNames %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("env.GetBool names %v, booleanEnvNames %v", got, want)
		}
	}
}

func TestNumericValueProblems(t *testing.T) {
	env := map[string]string{
		"HMAC_MAX_DRIFT":      "60",
		"RATE_LIMIT_PER_IP":   "5/m",
		"MAX_ATTEMPTS":        "5 ",
		"CHALLENGE_EXPIRY":    "5m",
		"RATE_LIMIT_PER_USER": "20",
		"RESEND_COOLDOWN":     "0",
	}
	problems := numericValueProblems(func(name string) string { return env[name] })
	if len(problems) != 3 {
		t.Fatalf("got %d problems, want 3: %v", len(problems), problems)
	}
	if problems := numericValueProblems(func(string) string { return "" }); len(problems) != 0 {
		t.Fatalf("unset variables must not be reported: %v", problems)
	}
}

// The name lists must cover every env.GetInt and env.GetDuration call so a new
// setting cannot be added without the strict check.
func TestNumericEnvNamesCoverEveryRead(t *testing.T) {
	for helper, names := range map[string][]string{
		"GetInt":      integerEnvNames,
		"GetDuration": durationEnvNames,
	} {
		re := regexp.MustCompile(`env\.` + helper + `\("([A-Z0-9_]+)"`)
		found := map[string]bool{}
		for _, file := range []string{"config.go", "redis.go", "strict.go"} {
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				found[m[1]] = true
			}
		}
		var got []string
		for name := range found {
			got = append(got, name)
		}
		sort.Strings(got)
		want := append([]string(nil), names...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("env.%s names %v, listed %v", helper, got, want)
		}
	}
}

func TestZeroResendCooldownIsRejected(t *testing.T) {
	previous := ResendCooldown
	t.Cleanup(func() { ResendCooldown = previous })
	ResendCooldown = 0
	found := false
	for _, problem := range strictDeliveryProblems() {
		if problem == "RESEND_COOLDOWN must be positive" {
			found = true
		}
	}
	if !found {
		t.Fatal("RESEND_COOLDOWN=0 must be reported")
	}
}
