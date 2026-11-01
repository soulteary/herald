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
	HMACKeysJSON = "broken"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	HMACSecret = strings.Repeat("h", 32)
	if err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel})); err != nil {
		t.Fatal(err)
	}
	if got := GetHMACSecret("unknown"); got != HMACSecret {
		t.Fatal("single-secret fallback unavailable")
	}
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
