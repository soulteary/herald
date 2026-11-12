package config

import (
	"os"
	"regexp"
	"sort"
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
