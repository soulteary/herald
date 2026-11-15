package config

import (
	"fmt"
	"strings"

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

// CodeHashPreviousKeysMax is the largest number of keys
// HERALD_CODE_HASH_KEY_PREVIOUS may list. It is the limit of the HMAC code
// hasher in challenge-kit.
const CodeHashPreviousKeysMax = challengekit.HMACCodeHasherMaxPreviousKeys

// CodeHashPreviousKeys returns the keys listed in
// HERALD_CODE_HASH_KEY_PREVIOUS: the value split at commas, each item with
// surrounding whitespace removed, empty items dropped. A key that contains a
// comma, or begins or ends with whitespace, can therefore not be listed.
func CodeHashPreviousKeys() []string {
	var keys []string
	for _, item := range strings.Split(CodeHashKeyPrevious, ",") {
		if key := strings.TrimSpace(item); key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// codeHashKeyProblems rejects a HERALD_CODE_HASH_KEY that is set but too
// short, and a HERALD_CODE_HASH_KEY_PREVIOUS that lists a key that is too
// short or lists too many keys. The messages carry positions and lengths,
// never a value.
func codeHashKeyProblems() []string {
	var problems []string
	if CodeHashKey != "" && len([]byte(CodeHashKey)) < CodeHashKeyMinBytes {
		problems = append(problems, fmt.Sprintf("HERALD_CODE_HASH_KEY must be at least %d bytes (got %d); unset it to store codes with the unkeyed Argon2 hash", CodeHashKeyMinBytes, len([]byte(CodeHashKey))))
	}
	previous := CodeHashPreviousKeys()
	if len(previous) > CodeHashPreviousKeysMax {
		problems = append(problems, fmt.Sprintf("HERALD_CODE_HASH_KEY_PREVIOUS lists %d keys, at most %d are accepted; a key is needed only until the challenges created under it have expired", len(previous), CodeHashPreviousKeysMax))
	}
	for i, key := range previous {
		if len([]byte(key)) < CodeHashKeyMinBytes {
			problems = append(problems, fmt.Sprintf("HERALD_CODE_HASH_KEY_PREVIOUS key %d must be at least %d bytes (got %d); keys are separated by commas", i+1, CodeHashKeyMinBytes, len([]byte(key))))
		}
	}
	return problems
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
