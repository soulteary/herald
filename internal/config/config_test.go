package config

import (
	"sync"
	"testing"
)

func TestParseHMACKeys(t *testing.T) {
	// Save original values
	originalHMACKeysJSON := HMACKeysJSON
	originalHMACSecret := HMACSecret
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		HMACSecret = originalHMACSecret
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	// Test valid JSON
	HMACKeysJSON = `{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}`
	// Reset state for testing
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	hmacDefaultKeyID = ""
	err := parseHMACKeys()
	if err != nil {
		t.Fatalf("parseHMACKeys() failed: %v", err)
	}

	if len(hmacKeysMap) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(hmacKeysMap))
	}

	if hmacKeysMap["key-id-1"] != "secret-key-1" {
		t.Errorf("Expected key-id-1 to be secret-key-1, got %s", hmacKeysMap["key-id-1"])
	}

	if hmacKeysMap["key-id-2"] != "secret-key-2" {
		t.Errorf("Expected key-id-2 to be secret-key-2, got %s", hmacKeysMap["key-id-2"])
	}

	// Test GetHMACSecret with key ID
	secret := GetHMACSecret("key-id-1")
	if secret != "secret-key-1" {
		t.Errorf("Expected GetHMACSecret('key-id-1') to return 'secret-key-1', got %s", secret)
	}

	// Test GetHMACSecret with default key ID (empty)
	// Note: default key ID is set to first key in map (order may vary)
	secret = GetHMACSecret("")
	// Should return one of the keys (first one)
	if secret == "" {
		t.Errorf("Expected GetHMACSecret('') to return a key, got empty string")
	}
	if secret != "secret-key-1" && secret != "secret-key-2" {
		t.Errorf("Expected GetHMACSecret('') to return one of the configured keys (secret-key-1 or secret-key-2), got %s", secret)
	}

	// Test GetHMACSecret with invalid key ID
	secret = GetHMACSecret("invalid-key-id")
	if secret != "" {
		t.Errorf("Expected GetHMACSecret('invalid-key-id') to return empty string, got %s", secret)
	}
}

func TestGetHMACSecret_FallbackToHMACSecret(t *testing.T) {
	// Save original values
	originalHMACKeysJSON := HMACKeysJSON
	originalHMACSecret := HMACSecret
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		HMACSecret = originalHMACSecret
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	// Test fallback to HMACSecret when no keys map
	HMACKeysJSON = ""
	HMACSecret = "fallback-secret"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}

	secret := GetHMACSecret("")
	if secret != "fallback-secret" {
		t.Errorf("Expected GetHMACSecret('') to return 'fallback-secret', got %s", secret)
	}
}

func TestHasHMACKeys(t *testing.T) {
	// Save original values
	originalHMACKeysJSON := HMACKeysJSON
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	// Test with keys
	HMACKeysJSON = `{"key-id-1":"secret-key-1"}`
	_ = parseHMACKeys()
	if !HasHMACKeys() {
		t.Error("Expected HasHMACKeys() to return true when keys are configured")
	}

	// Test without keys
	HMACKeysJSON = ""
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	if HasHMACKeys() {
		t.Error("Expected HasHMACKeys() to return false when no keys are configured")
	}
}
