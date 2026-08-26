package config

import (
	"sync"
	"testing"
)

func TestParseHMACKeysRejectsEmptyKeyMaterial(t *testing.T) {
	originalJSON := HMACKeysJSON
	defer func() {
		HMACKeysJSON = originalJSON
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	for _, raw := range []string{`{"":"valid-secret"}`, `{"key-id":""}`, `{"key-id":"   "}`} {
		HMACKeysJSON = raw
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
		if err := parseHMACKeys(); err == nil {
			t.Fatalf("parseHMACKeys(%s) should reject empty key material", raw)
		}
	}
}

func TestValidateProductionSecretMaterialRejectsShortSecrets(t *testing.T) {
	originalAPIKey, originalHMACSecret := APIKey, HMACSecret
	originalIdempotency, originalPepper := IdempotencySecret, PIIPepper
	originalMap := hmacKeysMap
	defer func() {
		APIKey, HMACSecret = originalAPIKey, originalHMACSecret
		IdempotencySecret, PIIPepper = originalIdempotency, originalPepper
		hmacKeysMap = originalMap
	}()

	APIKey = "short"
	HMACSecret = ""
	IdempotencySecret = "01234567890123456789012345678901"
	PIIPepper = "01234567890123456789012345678901"
	hmacKeysMap = map[string]string{"rotation-key": "tiny"}

	if got := validateProductionSecretMaterial(); len(got) != 2 {
		t.Fatalf("got %d secret-strength problems, want 2: %v", len(got), got)
	}
}
