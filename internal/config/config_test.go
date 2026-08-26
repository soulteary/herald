package config

import (
	"sync"
	"testing"
	"time"

	logger "github.com/soulteary/logger-kit/v2"
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

	// Phase 4 security policy: with MULTIPLE keys and no explicit
	// HERALD_HMAC_DEFAULT_KEY_ID, GetHMACSecret("") must NOT map to an arbitrary
	// key (no reliance on map iteration order). It returns empty so a request
	// without X-Key-Id is rejected.
	secret = GetHMACSecret("")
	if secret != "" {
		t.Errorf("Expected GetHMACSecret('') to return empty (no arbitrary default) with multiple keys, got %s", secret)
	}
	if got := GetHMACDefaultKeyID(); got != "" {
		t.Errorf("GetHMACDefaultKeyID() with multiple keys = %q, want empty", got)
	}

	// Test GetHMACSecret with invalid key ID
	secret = GetHMACSecret("invalid-key-id")
	if secret != "" {
		t.Errorf("Expected GetHMACSecret('invalid-key-id') to return empty string, got %s", secret)
	}
}

func TestParseHMACKeys_SingleKeyProvidesEffectiveDefault(t *testing.T) {
	originalHMACKeysJSON := HMACKeysJSON
	originalDefault := HMACDefaultKeyID
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		HMACDefaultKeyID = originalDefault
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	HMACKeysJSON = `{"only-key":"only-secret"}`
	HMACDefaultKeyID = ""
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	hmacDefaultKeyID = ""
	if err := parseHMACKeys(); err != nil {
		t.Fatalf("parseHMACKeys() failed: %v", err)
	}
	if got := GetHMACDefaultKeyID(); got != "only-key" {
		t.Errorf("GetHMACDefaultKeyID() = %q, want only-key", got)
	}
}

// TestParseHMACKeys_ExplicitDefaultKeyID verifies that an explicitly configured
// HERALD_HMAC_DEFAULT_KEY_ID is used deterministically for empty X-Key-Id, and
// an unknown default is rejected.
func TestParseHMACKeys_ExplicitDefaultKeyID(t *testing.T) {
	originalHMACKeysJSON := HMACKeysJSON
	originalDefault := HMACDefaultKeyID
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		HMACDefaultKeyID = originalDefault
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	HMACKeysJSON = `{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}`
	HMACDefaultKeyID = "key-id-2"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	hmacDefaultKeyID = ""
	if err := parseHMACKeys(); err != nil {
		t.Fatalf("parseHMACKeys() failed: %v", err)
	}
	if got := GetHMACSecret(""); got != "secret-key-2" {
		t.Errorf("GetHMACSecret('') with explicit default = %q, want secret-key-2", got)
	}
	if got := GetHMACDefaultKeyID(); got != "key-id-2" {
		t.Errorf("GetHMACDefaultKeyID() = %q, want key-id-2", got)
	}

	// Unknown default must be rejected.
	HMACDefaultKeyID = "does-not-exist"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	hmacDefaultKeyID = ""
	if err := parseHMACKeys(); err == nil {
		t.Errorf("parseHMACKeys() with unknown default key id should fail")
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

func TestParseHMACKeys_InvalidJSON(t *testing.T) {
	originalHMACKeysJSON := HMACKeysJSON
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	log := logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	_ = Initialize(log)

	HMACKeysJSON = `{invalid-json`
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	err := parseHMACKeys()
	if err == nil {
		t.Error("parseHMACKeys() with invalid JSON should return error")
	}
}

func TestParseHMACKeys_EmptyKeys(t *testing.T) {
	originalHMACKeysJSON := HMACKeysJSON
	defer func() {
		HMACKeysJSON = originalHMACKeysJSON
		hmacKeysMap = nil
		hmacKeysMapOnce = sync.Once{}
		hmacDefaultKeyID = ""
	}()

	_ = Initialize(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON}))

	HMACKeysJSON = `{}`
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	err := parseHMACKeys()
	if err == nil {
		t.Error("parseHMACKeys() with empty keys should return error")
	}
}

func TestInitialize(t *testing.T) {
	log := logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	err := Initialize(log)
	if err != nil {
		t.Errorf("Initialize() error = %v", err)
	}
}

func TestGetPort(t *testing.T) {
	originalPort := Port
	defer func() { Port = originalPort }()

	t.Run("with_colon_prefix", func(t *testing.T) {
		Port = ":8082"
		got := GetPort()
		if got != ":8082" {
			t.Errorf("GetPort() = %q, want :8082", got)
		}
	})

	t.Run("without_colon_prefix", func(t *testing.T) {
		Port = "8082"
		got := GetPort()
		if got != ":8082" {
			t.Errorf("GetPort() = %q, want :8082", got)
		}
	})
}

func TestInitialize_InvalidRedisAddr(t *testing.T) {
	origAddr := RedisAddr
	defer func() { RedisAddr = origAddr }()

	RedisAddr = "not-valid-host-port"
	log := logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	err := Initialize(log)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if RedisAddr != "localhost:6379" {
		t.Errorf("Initialize() with invalid REDIS_ADDR should reset to localhost:6379, got %q", RedisAddr)
	}
}

func TestInitialize_TLSCACertAlias(t *testing.T) {
	origCA, origClientCA := TLSCACertFile, TLSClientCAFile
	defer func() {
		TLSCACertFile = origCA
		TLSClientCAFile = origClientCA
	}()

	TLSCACertFile = ""
	TLSClientCAFile = "/path/to/ca.pem"
	log := logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	err := Initialize(log)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if TLSCACertFile != "/path/to/ca.pem" {
		t.Errorf("Initialize() should set TLSCACertFile from TLSClientCAFile alias, got %q", TLSCACertFile)
	}
}

func TestInitialize_IdempotencyKeyTTLDefault(t *testing.T) {
	origTTL := IdempotencyKeyTTL
	origExpiry := ChallengeExpiry
	defer func() {
		IdempotencyKeyTTL = origTTL
		ChallengeExpiry = origExpiry
	}()

	IdempotencyKeyTTL = 0
	ChallengeExpiry = 10 * time.Minute
	log := logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	err := Initialize(log)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if IdempotencyKeyTTL != 10*time.Minute {
		t.Errorf("Initialize() should set IdempotencyKeyTTL to ChallengeExpiry when 0, got %v", IdempotencyKeyTTL)
	}
}
