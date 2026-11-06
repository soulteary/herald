package config

import "fmt"

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
