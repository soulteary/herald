package config

import (
	"fmt"

	challengekit "github.com/soulteary/challenge-kit"
)

const minimumProductionSecretBytes = 32

func validateProductionSecretMaterial() []string {
	var problems []string
	check := func(name, value string) {
		if value != "" && len([]byte(value)) < minimumProductionSecretBytes {
			problems = append(problems, fmt.Sprintf("%s must be at least %d bytes in production", name, minimumProductionSecretBytes))
		}
	}

	check("API_KEY", APIKey)
	check("HERALD_METRICS_API_KEY", MetricsAPIKey)
	check("HMAC_SECRET", HMACSecret)
	check("HERALD_IDEMPOTENCY_SECRET", IdempotencySecret)
	check("HERALD_PII_PEPPER", PIIPepper)
	for keyID, secret := range hmacKeysMap {
		if len([]byte(secret)) < minimumProductionSecretBytes {
			problems = append(problems, fmt.Sprintf("HERALD_HMAC_KEYS[%q] must be at least %d bytes in production", keyID, minimumProductionSecretBytes))
		}
	}
	return problems
}

// CodeHashKeyMinBytes is the shortest HERALD_CODE_HASH_KEY Herald accepts. It
// is the minimum of the HMAC code hasher in challenge-kit, and applies in
// every environment: a shorter key cannot be used at all.
const CodeHashKeyMinBytes = challengekit.HMACCodeHasherMinKeyLen

// codeHashKeyProblems rejects a HERALD_CODE_HASH_KEY that is set but too
// short. The message carries the length, never the value.
func codeHashKeyProblems() []string {
	if CodeHashKey == "" || len([]byte(CodeHashKey)) >= CodeHashKeyMinBytes {
		return nil
	}
	return []string{fmt.Sprintf("HERALD_CODE_HASH_KEY must be at least %d bytes (got %d); unset it to store codes with the unkeyed Argon2 hash", CodeHashKeyMinBytes, len([]byte(CodeHashKey)))}
}

// CodeHashMode names how new verification codes are stored, for the startup
// log: "argon2" without a key or in verify-only mode, "hmac-sha256" otherwise.
func CodeHashMode() string {
	switch {
	case CodeHashKey == "":
		return "argon2"
	case CodeHashVerifyOnly:
		return "argon2 (hmac-sha256 verify-only)"
	default:
		return "hmac-sha256"
	}
}

// strictCredentialProblems prevents a read-only scraping credential from also
// authenticating business requests, in every deployment environment.
func strictCredentialProblems() []string {
	if MetricsAPIKey == "" {
		return nil
	}
	reused := MetricsAPIKey == APIKey || MetricsAPIKey == HMACSecret
	for _, secret := range hmacKeysMap {
		reused = reused || MetricsAPIKey == secret
	}
	if reused {
		return []string{"HERALD_METRICS_API_KEY must differ from all business authentication secrets"}
	}
	return nil
}
