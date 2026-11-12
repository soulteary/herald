package config

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	logger "github.com/soulteary/logger-kit/v3"
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

// TestInitialize_ProductionRejectsInvalidRedisAddr proves production keeps the
// configured address and refuses to start instead of using localhost.
func TestInitialize_ProductionRejectsInvalidRedisAddr(t *testing.T) {
	defer withProdEnv(t)()
	origAddr := RedisAddr
	defer func() { RedisAddr = origAddr }()

	RedisAddr = "not-valid-host-port"
	err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON}))
	if err == nil || !strings.Contains(err.Error(), "REDIS_ADDR") {
		t.Fatalf("production accepted an invalid REDIS_ADDR: %v", err)
	}
	if RedisAddr != "not-valid-host-port" {
		t.Errorf("production replaced the invalid REDIS_ADDR with %q", RedisAddr)
	}

	// An unset address still uses the client default and is not an error.
	RedisAddr = ""
	if err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})); err != nil {
		t.Fatalf("production with unset REDIS_ADDR rejected: %v", err)
	}
}

// TestInitialize_MissingCredentialWarning proves the startup warning states
// what the selected REQUEST_AUTH_MODE does without credentials: hmac_v2 and
// api_key reject requests, and only "none" disables authentication.
func TestInitialize_MissingCredentialWarning(t *testing.T) {
	const strongKey = "01234567890123456789012345678901"
	tests := []struct {
		name, mode, apiKey, hmacSecret string
		want                           string
	}{
		{name: "default mode without credentials", mode: "hmac_v2", want: "REQUEST_AUTH_MODE=hmac_v2 but neither HMAC_SECRET nor HERALD_HMAC_KEYS is set: all authenticated endpoints will reject requests"},
		{name: "hmac mode with only an API key", mode: "hmac_v2", apiKey: strongKey, want: "all authenticated endpoints will reject requests"},
		{name: "api key mode without API key", mode: "api_key", hmacSecret: strongKey, want: "REQUEST_AUTH_MODE=api_key but API_KEY is not set: all authenticated endpoints will reject requests"},
		{name: "none mode", mode: "none", want: "service-to-service authentication is disabled"},
		{name: "hmac mode with secret", mode: "hmac_v2", hmacSecret: strongKey},
		{name: "api key mode with key", mode: "api_key", apiKey: strongKey},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer withProdEnv(t)()
			oldMap := hmacKeysMap
			defer func() { hmacKeysMap = oldMap; hmacKeysMapOnce = sync.Once{} }()
			hmacKeysMapOnce = sync.Once{}
			Env = EnvDevelopment
			RequestAuthMode, APIKey, HMACSecret = test.mode, test.apiKey, test.hmacSecret

			var logs bytes.Buffer
			if err := Initialize(logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})); err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			out := logs.String()
			if strings.Contains(out, "authentication will be disabled") {
				t.Errorf("startup log still claims authentication will be disabled: %s", out)
			}
			if test.want == "" {
				if strings.Contains(out, "REQUEST_AUTH_MODE=") {
					t.Errorf("unexpected credential warning: %s", out)
				}
				return
			}
			if !strings.Contains(out, test.want) {
				t.Errorf("startup log = %s, want it to contain %q", out, test.want)
			}
			if test.mode != "none" && strings.Contains(out, "authentication is disabled") {
				t.Errorf("mode %s must not be reported as disabled: %s", test.mode, out)
			}
		})
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
