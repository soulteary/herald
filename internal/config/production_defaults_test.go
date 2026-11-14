package config

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/soulteary/cli-kit/env"
	logger "github.com/soulteary/logger-kit/v3"
)

// initializeProduction runs Initialize on the valid production baseline with
// ProviderFailurePolicy holding the value the given environment would have
// produced at process start.
func initializeProduction(t *testing.T, policyEnv string, set bool) (string, error) {
	t.Helper()
	restore := withProdEnv(t)
	oldMap := hmacKeysMap
	t.Cleanup(func() { hmacKeysMap = oldMap; hmacKeysMapOnce = sync.Once{}; restore() })
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}

	t.Setenv("PROVIDER_FAILURE_POLICY", policyEnv)
	if !set {
		if err := os.Unsetenv("PROVIDER_FAILURE_POLICY"); err != nil {
			t.Fatal(err)
		}
	}
	// Same expression as the package-level initializer.
	ProviderFailurePolicy = env.Get("PROVIDER_FAILURE_POLICY", "soft")

	var logs bytes.Buffer
	err := Initialize(logger.New(logger.Config{Level: logger.InfoLevel, Format: logger.FormatJSON, Output: &logs}))
	return logs.String(), err
}

// TestProductionDefaultsProviderFailurePolicyToStrict covers an operator who
// sets ENVIRONMENT=production and leaves PROVIDER_FAILURE_POLICY alone. The
// built-in default is soft, which production forbids, so startup used to fail
// on a value the operator never chose.
func TestProductionDefaultsProviderFailurePolicyToStrict(t *testing.T) {
	for name, set := range map[string]bool{"absent": false, "empty": true} {
		t.Run(name, func(t *testing.T) {
			out, err := initializeProduction(t, "", set)
			if err != nil {
				t.Fatalf("production with PROVIDER_FAILURE_POLICY unset must start, got: %v", err)
			}
			if ProviderFailurePolicy != "strict" {
				t.Fatalf("ProviderFailurePolicy = %q, want strict", ProviderFailurePolicy)
			}
			if !strings.Contains(out, "Production default: PROVIDER_FAILURE_POLICY=strict") {
				t.Fatalf("the applied default is not logged: %s", out)
			}
		})
	}
}

func TestProductionStillRejectsExplicitSoftProviderFailurePolicy(t *testing.T) {
	for _, value := range []string{"soft", "SOFT", " soft "} {
		t.Run(value, func(t *testing.T) {
			_, err := initializeProduction(t, value, true)
			if err == nil || !strings.Contains(err.Error(), "PROVIDER_FAILURE_POLICY=soft is forbidden in production") {
				t.Fatalf("explicit %q must be rejected in production, got: %v", value, err)
			}
		})
	}
}

func TestDevelopmentKeepsSoftProviderFailurePolicyDefault(t *testing.T) {
	t.Setenv("PROVIDER_FAILURE_POLICY", "")
	oldPolicy := ProviderFailurePolicy
	t.Cleanup(func() { ProviderFailurePolicy = oldPolicy })
	ProviderFailurePolicy = env.Get("PROVIDER_FAILURE_POLICY", "soft")
	strong := strings.Repeat("k", 32)
	initializeDevelopment(t, strong, "", strong)
	if ProviderFailurePolicy != "soft" {
		t.Fatalf("ProviderFailurePolicy = %q outside production, want soft", ProviderFailurePolicy)
	}
}

// TestPlaintextProviderURLWordingMatchesEnvironment checks the https check
// names production only when the process is in production. Outside it the
// text is logged as a warning and must not suggest the wrong environment.
func TestPlaintextProviderURLWordingMatchesEnvironment(t *testing.T) {
	defer withProdEnv(t)()
	HeraldSMTPAPIURL = "http://herald-smtp:8084"

	problems := strings.Join(providerTransportProblems(), "; ")
	if !strings.Contains(problems, "HERALD_SMTP_API_URL must use https:// in production") {
		t.Fatalf("production wording changed: %q", problems)
	}
	if err := Validate(); err == nil {
		t.Fatal("production must still refuse a plaintext provider URL")
	}

	Env = EnvDevelopment
	problems = strings.Join(providerTransportProblems(), "; ")
	if strings.Contains(problems, "must use https:// in production") {
		t.Fatalf("development warning reads as a production requirement: %q", problems)
	}
	for _, want := range []string{"HERALD_SMTP_API_URL", "does not use https://", "accepted in this environment"} {
		if !strings.Contains(problems, want) {
			t.Errorf("development warning %q does not contain %q", problems, want)
		}
	}
	if err := Validate(); err != nil {
		t.Fatalf("development must accept a plaintext provider URL, got: %v", err)
	}
}
