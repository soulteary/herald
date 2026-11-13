package config

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"testing"

	logger "github.com/soulteary/logger-kit/v3"
)

// initializeDevelopment runs Initialize in development with the given
// secrets and returns the warnings it logged and the resulting idempotency
// secret and PII pepper.
func initializeDevelopment(t *testing.T, apiKey, hmacSecret, idempotencySecret string) (logged, secret, pepper string) {
	t.Helper()
	defer withProdEnv(t)()
	oldMap, oldDefault := hmacKeysMap, hmacDefaultKeyID
	defer func() {
		hmacKeysMap, hmacDefaultKeyID = oldMap, oldDefault
		hmacKeysMapOnce = sync.Once{}
	}()
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	Env = EnvDevelopment
	APIKey, HMACSecret, IdempotencySecret, PIIPepper = apiKey, hmacSecret, idempotencySecret, ""

	var logs bytes.Buffer
	if err := Initialize(logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	return logs.String(), IdempotencySecret, PIIPepper
}

func TestInitializeWarnsWhenEnvironmentIsUnset(t *testing.T) {
	const marker = "ENVIRONMENT is not set"
	strong := strings.Repeat("k", 32)

	for name, value := range map[string]*string{"absent": nil, "empty": new(string)} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", "")
			if value == nil {
				if err := os.Unsetenv("ENVIRONMENT"); err != nil {
					t.Fatal(err)
				}
			}
			out, _, _ := initializeDevelopment(t, strong, "", strong)
			if strings.Count(out, marker) != 1 {
				t.Fatalf("want exactly one warning, got: %s", out)
			}
			for _, want := range []string{"defaulting to development", "production validation is NOT enforced", "set ENVIRONMENT explicitly"} {
				if !strings.Contains(out, want) {
					t.Errorf("warning does not contain %q: %s", want, out)
				}
			}
		})
	}

	for _, explicit := range []string{"development", "dev", "test"} {
		t.Run("explicit "+explicit, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", explicit)
			if out, _, _ := initializeDevelopment(t, strong, "", strong); strings.Contains(out, marker) {
				t.Fatalf("explicit ENVIRONMENT=%s still warns: %s", explicit, out)
			}
		})
	}
}

func TestInitializeGeneratesProcessSecretWithoutConfiguredSecret(t *testing.T) {
	const retired = "herald-idempotency-fallback"
	t.Setenv("ENVIRONMENT", "development")

	var seen []string
	for i := 0; i < 2; i++ {
		out, secret, pepper := initializeDevelopment(t, "", "", "")
		raw, err := hex.DecodeString(secret)
		if err != nil || len(raw) < 32 {
			t.Fatalf("generated secret is not at least 32 random bytes: %q", secret)
		}
		if secret == retired || pepper == retired {
			t.Fatal("the fixed fallback secret is still in use")
		}
		if pepper != secret {
			t.Fatalf("PII pepper = %q, want the generated secret", pepper)
		}
		for _, want := range []string{"not shared between instances", "does not survive a restart", "HERALD_IDEMPOTENCY_SECRET", "HERALD_PII_PEPPER"} {
			if !strings.Contains(out, want) {
				t.Errorf("warning does not contain %q: %s", want, out)
			}
		}
		if strings.Contains(out, secret) {
			t.Fatal("generated secret was written to the log")
		}
		seen = append(seen, secret)
	}
	if seen[0] == seen[1] {
		t.Fatal("two initialisations produced the same secret")
	}
}

func TestInitializeDoesNotGenerateSecretWhenOneIsConfigured(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")
	strong := strings.Repeat("s", 32)
	for name, secrets := range map[string][3]string{
		"api key":            {strong, "", ""},
		"hmac secret":        {"", strong, ""},
		"idempotency secret": {"", "", strong},
	} {
		t.Run(name, func(t *testing.T) {
			out, secret, pepper := initializeDevelopment(t, secrets[0], secrets[1], secrets[2])
			if secret != strong || pepper != strong {
				t.Fatal("configured secret not used")
			}
			if strings.Contains(out, "generated a random secret") {
				t.Fatalf("unexpected generated-secret warning: %s", out)
			}
		})
	}
}

// Production still refuses to start without secret material, so the generated
// secret is never reached there.
func TestProductionStillRequiresSecrets(t *testing.T) {
	defer withProdEnv(t)()
	oldMap := hmacKeysMap
	defer func() { hmacKeysMap = oldMap; hmacKeysMapOnce = sync.Once{} }()
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	APIKey, HMACSecret, IdempotencySecret, PIIPepper = "", "", "", ""
	err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel}))
	if err == nil || !strings.Contains(err.Error(), "production config validation failed") {
		t.Fatalf("production started without secrets: %v", err)
	}
}

// A key map is stable secret material: instances that share it must agree on
// the derived secret, and it must not depend on map iteration order.
func TestSecretFromKeyMapIsStable(t *testing.T) {
	keys := map[string]string{"gateway": strings.Repeat("a", 32), "worker": strings.Repeat("b", 32)}
	first := secretFromKeyMap(keys)
	for i := 0; i < 20; i++ {
		if got := secretFromKeyMap(map[string]string{"worker": keys["worker"], "gateway": keys["gateway"]}); got != first {
			t.Fatal("derived secret depends on map order")
		}
	}
	if len(first) < 64 {
		t.Fatalf("derived secret too short: %d", len(first))
	}
	if other := secretFromKeyMap(map[string]string{"gateway": strings.Repeat("c", 32)}); other == first {
		t.Fatal("different key maps produced the same secret")
	}
	// An ID/secret boundary shift must not collide.
	if secretFromKeyMap(map[string]string{"ab": "c"}) == secretFromKeyMap(map[string]string{"a": "bc"}) {
		t.Fatal("key ID and secret are not separated")
	}
}

func TestInitializeDerivesSecretFromKeyMapOutsideProduction(t *testing.T) {
	defer withProdEnv(t)()
	oldMap, oldJSON := hmacKeysMap, HMACKeysJSON
	defer func() { hmacKeysMap, HMACKeysJSON = oldMap, oldJSON; hmacKeysMapOnce = sync.Once{} }()
	keys := map[string]string{"gateway": strings.Repeat("k", 32)}
	hmacKeysMap = keys
	Env = EnvDevelopment
	APIKey, HMACSecret, IdempotencySecret, PIIPepper = "", "", "", ""

	var logs bytes.Buffer
	log = logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})
	if err := initializeSecretsAndPrivacy(); err != nil {
		t.Fatalf("initializeSecretsAndPrivacy() error = %v", err)
	}
	if IdempotencySecret != secretFromKeyMap(keys) {
		t.Fatal("secret was not derived from the key map")
	}
	if strings.Contains(logs.String(), "generated a random secret") {
		t.Fatalf("unexpected generated-secret warning: %s", logs.String())
	}
}
