package config

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	logger "github.com/soulteary/logger-kit/v3"
)

// The three settings below are accepted and change nothing on their own.
// Each must produce one startup warning that names the setting and what is
// missing, and none when the setting is complete or off.

func TestTestModeAdvisoryOutsideTheTestEnvironment(t *testing.T) {
	oldEnv, oldMode := Env, TestMode
	t.Cleanup(func() { Env, TestMode = oldEnv, oldMode })

	Env, TestMode = EnvDevelopment, true
	message := advisoryFor("config.test_mode")
	for _, want := range []string{"HERALD_TEST_MODE=true", "no effect", "ENVIRONMENT=test", "debug_code"} {
		if !strings.Contains(message, want) {
			t.Errorf("advisory %q does not contain %q", message, want)
		}
	}

	for name, set := range map[string]func(){
		"test environment":       func() { Env, TestMode = EnvTest, true },
		"flag off":               func() { Env, TestMode = EnvDevelopment, false },
		"production is an error": func() { Env, TestMode = EnvProduction, true },
	} {
		set()
		if message := advisoryFor("config.test_mode"); message != "" {
			t.Errorf("%s: unexpected advisory %q", name, message)
		}
	}
}

func TestTOTPAdvisoryWithoutBaseURL(t *testing.T) {
	oldEnabled, oldURL := TOTPEnabled, TOTPBaseURL
	t.Cleanup(func() { TOTPEnabled, TOTPBaseURL = oldEnabled, oldURL })

	TOTPEnabled, TOTPBaseURL = true, "  "
	message := advisoryFor("config.totp")
	for _, want := range []string{"HERALD_TOTP_ENABLED=true", "HERALD_TOTP_BASE_URL", "totp_not_configured"} {
		if !strings.Contains(message, want) {
			t.Errorf("advisory %q does not contain %q", message, want)
		}
	}

	TOTPEnabled, TOTPBaseURL = true, "https://herald-totp.internal:8085"
	if message := advisoryFor("config.totp"); message != "" {
		t.Errorf("unexpected advisory with a base URL: %q", message)
	}
	TOTPEnabled, TOTPBaseURL = false, ""
	if message := advisoryFor("config.totp"); message != "" {
		t.Errorf("unexpected advisory with TOTP off: %q", message)
	}
}

func TestOTLPAdvisoryWithoutEndpoint(t *testing.T) {
	oldEnabled, oldEndpoint := OTLPEnabled, OTLPEndpoint
	t.Cleanup(func() { OTLPEnabled, OTLPEndpoint = oldEnabled, oldEndpoint })

	OTLPEnabled, OTLPEndpoint = true, ""
	message := advisoryFor("config.otlp")
	for _, want := range []string{"OTLP_ENABLED=true", "OTLP_ENDPOINT is empty", "no spans are exported"} {
		if !strings.Contains(message, want) {
			t.Errorf("advisory %q does not contain %q", message, want)
		}
	}

	OTLPEnabled, OTLPEndpoint = true, "localhost:4318"
	if message := advisoryFor("config.otlp"); message != "" {
		t.Errorf("unexpected advisory with an endpoint: %q", message)
	}
	OTLPEnabled, OTLPEndpoint = false, ""
	if message := advisoryFor("config.otlp"); message != "" {
		t.Errorf("unexpected advisory with tracing off: %q", message)
	}
}

// TestInitializeLogsIneffectiveSettings checks the three warnings reach the
// startup log once each, and that a default configuration logs none of them.
func TestInitializeLogsIneffectiveSettings(t *testing.T) {
	defer withProdEnv(t)()
	oldMap, oldDefault := hmacKeysMap, hmacDefaultKeyID
	oldTOTP, oldTOTPURL, oldOTLP, oldEndpoint := TOTPEnabled, TOTPBaseURL, OTLPEnabled, OTLPEndpoint
	defer func() {
		hmacKeysMap, hmacDefaultKeyID = oldMap, oldDefault
		hmacKeysMapOnce = sync.Once{}
		TOTPEnabled, TOTPBaseURL, OTLPEnabled, OTLPEndpoint = oldTOTP, oldTOTPURL, oldOTLP, oldEndpoint
	}()
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	Env = EnvDevelopment
	checks := []string{`"check":"config.test_mode"`, `"check":"config.totp"`, `"check":"config.otlp"`}

	initialize := func() string {
		t.Helper()
		var logs bytes.Buffer
		if err := Initialize(logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})); err != nil {
			t.Fatalf("these settings must stay warnings, got error: %v", err)
		}
		return logs.String()
	}

	TestMode, TOTPEnabled, TOTPBaseURL, OTLPEnabled, OTLPEndpoint = true, true, "", true, ""
	out := initialize()
	for _, check := range checks {
		if got := strings.Count(out, check); got != 1 {
			t.Errorf("want exactly one %s warning, got %d: %s", check, got, out)
		}
	}

	TestMode, TOTPEnabled, OTLPEnabled = false, false, false
	out = initialize()
	for _, check := range checks {
		if strings.Contains(out, check) {
			t.Errorf("%s logged for a default configuration: %s", check, out)
		}
	}
}
