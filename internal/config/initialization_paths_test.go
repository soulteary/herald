package config

import (
	"strings"
	"sync"
	"testing"

	logger "github.com/soulteary/logger-kit/v3"
)

func TestInitializeSecretDerivation(t *testing.T) {
	for _, source := range []string{"hmac", "api key", "development fallback", "production key map"} {
		t.Run(source, func(t *testing.T) {
			defer withProdEnv(t)()
			oldRedis, oldIdem, oldPepper, oldLog := RedisAddr, IdempotencySecret, PIIPepper, log
			oldMap, oldDefault := hmacKeysMap, hmacDefaultKeyID
			defer func() {
				RedisAddr, IdempotencySecret, PIIPepper, log = oldRedis, oldIdem, oldPepper, oldLog
				hmacKeysMap, hmacDefaultKeyID = oldMap, oldDefault
				hmacKeysMapOnce = sync.Once{}
			}()
			RedisAddr = ""
			IdempotencySecret = ""
			PIIPepper = ""
			APIKey = ""
			HMACSecret = ""
			hmacKeysMap = nil
			hmacKeysMapOnce = sync.Once{}
			Env = EnvDevelopment
			want := "herald-idempotency-fallback"
			switch source {
			case "hmac":
				HMACSecret = strings.Repeat("h", 32)
				want = HMACSecret
			case "api key":
				APIKey = strings.Repeat("a", 32)
				want = APIKey
			case "production key map":
				Env = EnvProduction
				HMACKeysJSON = `{"key":"01234567890123456789012345678901"}`
			}
			err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel}))
			if source == "production key map" {
				if err == nil || !strings.Contains(err.Error(), "HERALD_IDEMPOTENCY_SECRET") {
					t.Fatalf("implicit multi-key secret accepted: %v", err)
				}
				return
			}
			if err != nil || IdempotencySecret != want || PIIPepper != want {
				t.Fatalf("derived secrets: err=%v", err)
			}
		})
	}
}

func TestInitializeInvalidKeyMapUsesConfiguredSingleSecret(t *testing.T) {
	defer withProdEnv(t)()
	oldJSON, oldMap, oldDefault := HMACKeysJSON, hmacKeysMap, hmacDefaultKeyID
	defer func() {
		HMACKeysJSON, hmacKeysMap, hmacDefaultKeyID = oldJSON, oldMap, oldDefault
		hmacKeysMapOnce = sync.Once{}
	}()
	Env = EnvDevelopment
	HMACKeysJSON = "broken"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	HMACSecret = strings.Repeat("h", 32)
	if err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel})); err != nil {
		t.Fatal(err)
	}
	if got := GetHMACSecret("default"); got != HMACSecret {
		t.Fatal("single-secret fallback unavailable")
	}
	if GetHMACSecret("unknown") != "" {
		t.Fatal("invalid key map must not allow arbitrary single-secret aliases")
	}
}

// TestInitializeProductionRejectsMalformedKeyMap proves production does not
// fall back to HMAC_SECRET when HERALD_HMAC_KEYS is set but unusable, while an
// unset key map stays valid.
func TestInitializeProductionRejectsMalformedKeyMap(t *testing.T) {
	const validKey = "01234567890123456789012345678901"
	for name, test := range map[string]struct{ keys, defaultID string }{
		"invalid JSON":       {keys: "broken"},
		"empty object":       {keys: "{}"},
		"empty secret":       {keys: `{"key":""}`},
		"unknown default ID": {keys: `{"key":"` + validKey + `"}`, defaultID: "missing"},
	} {
		t.Run(name, func(t *testing.T) {
			defer withProdEnv(t)()
			oldJSON, oldMap, oldDefault, oldDefaultID := HMACKeysJSON, hmacKeysMap, hmacDefaultKeyID, HMACDefaultKeyID
			defer func() {
				HMACKeysJSON, hmacKeysMap, hmacDefaultKeyID, HMACDefaultKeyID = oldJSON, oldMap, oldDefault, oldDefaultID
				hmacKeysMapOnce = sync.Once{}
			}()
			HMACKeysJSON, HMACDefaultKeyID = test.keys, test.defaultID
			hmacKeysMap = nil
			hmacKeysMapOnce = sync.Once{}
			HMACSecret = strings.Repeat("h", 32)
			err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel}))
			if err == nil || !strings.Contains(err.Error(), "HERALD_HMAC_KEYS") {
				t.Fatalf("production accepted a malformed key map: %v", err)
			}
		})
	}

	t.Run("unset", func(t *testing.T) {
		defer withProdEnv(t)()
		oldMap := hmacKeysMap
		defer func() { hmacKeysMap = oldMap; hmacKeysMapOnce = sync.Once{} }()
		hmacKeysMapOnce = sync.Once{}
		if err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel})); err != nil {
			t.Fatalf("production without HERALD_HMAC_KEYS rejected: %v", err)
		}
	})
}

func TestValidateClientCertificateModes(t *testing.T) {
	defer withProdEnv(t)()
	oldMode := ClientCertMode
	defer func() { ClientCertMode = oldMode }()
	for _, mode := range []string{"optional", "require", "OFF"} {
		ClientCertMode = mode
		if err := Validate(); err == nil || !strings.Contains(err.Error(), "CLIENT_CERT_MODE") {
			t.Fatalf("mode %s without CA accepted: %v", mode, err)
		}
	}
}

func TestProductionLegacyHMACWarningDoesNotRejectValidConfig(t *testing.T) {
	defer withProdEnv(t)()
	old := HMACV1Enabled
	defer func() { HMACV1Enabled = old }()
	HMACV1Enabled = true
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
	if !IsProduction() || IsTestEnv() {
		t.Fatal("environment predicates disagree")
	}
}
