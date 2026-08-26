package config

import (
	"testing"
	"time"
)

func TestValidateStrictSettingsRejectsUnknownEnums(t *testing.T) {
	origEnvironment := EnvironmentValue
	origAuthMode := RequestAuthMode
	origClientCertMode := ClientCertMode
	origProviderPolicy := ProviderFailurePolicy
	origRedirectPolicy := ProviderRedirectPolicy
	defer func() {
		EnvironmentValue = origEnvironment
		RequestAuthMode = origAuthMode
		ClientCertMode = origClientCertMode
		ProviderFailurePolicy = origProviderPolicy
		ProviderRedirectPolicy = origRedirectPolicy
	}()

	tests := []struct {
		name   string
		mutate func()
	}{
		{"environment", func() { EnvironmentValue = "prodution" }},
		{"request auth mode", func() { RequestAuthMode = "api-keey" }},
		{"client cert mode", func() { ClientCertMode = "required" }},
		{"provider failure policy", func() { ProviderFailurePolicy = "strcit" }},
		{"provider redirect policy", func() { ProviderRedirectPolicy = "follow" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			EnvironmentValue = "development"
			RequestAuthMode = "hmac_v2"
			ClientCertMode = "off"
			ProviderFailurePolicy = "soft"
			ProviderRedirectPolicy = "deny"
			tt.mutate()
			if err := validateStrictSettings(); err == nil {
				t.Fatal("unknown enum value must be rejected")
			}
		})
	}
}

func TestValidateStrictSettingsRejectsUnsafeBounds(t *testing.T) {
	origCodeLength := CodeLength
	origProviderTimeout := ProviderTimeout
	origRateLimit := RateLimitPerUser
	defer func() {
		CodeLength = origCodeLength
		ProviderTimeout = origProviderTimeout
		RateLimitPerUser = origRateLimit
	}()

	CodeLength = 3
	if err := validateStrictSettings(); err == nil {
		t.Fatal("short OTP codes must be rejected")
	}
	CodeLength = 6
	ProviderTimeout = 0
	if err := validateStrictSettings(); err == nil {
		t.Fatal("zero provider timeout must be rejected")
	}
	ProviderTimeout = 10 * time.Second
	RateLimitPerUser = 0
	if err := validateStrictSettings(); err == nil {
		t.Fatal("zero rate limit must be rejected")
	}
}

func TestValidateStrictSettingsRequiresCompleteTrustedProxyConfig(t *testing.T) {
	originalHeader, originalProxies := TrustedProxyHeader, TrustedProxies
	defer func() {
		TrustedProxyHeader, TrustedProxies = originalHeader, originalProxies
	}()

	TrustedProxyHeader = "X-Forwarded-For"
	TrustedProxies = nil
	if err := validateStrictSettings(); err == nil {
		t.Fatal("proxy header without an allowlist must be rejected")
	}

	TrustedProxies = []string{"10.0.0.0/8"}
	if err := validateStrictSettings(); err != nil {
		t.Fatalf("complete trusted proxy config should pass: %v", err)
	}
}

func TestValidateStrictSettingsRequiresLeaseBeyondProviderTimeout(t *testing.T) {
	originalTTL, originalTimeout := IdempotencyKeyTTL, ProviderTimeout
	defer func() {
		IdempotencyKeyTTL, ProviderTimeout = originalTTL, originalTimeout
	}()

	IdempotencyKeyTTL = 5 * time.Second
	ProviderTimeout = 10 * time.Second
	if err := validateStrictSettings(); err == nil {
		t.Fatal("idempotency lease shorter than provider timeout must be rejected")
	}

	IdempotencyKeyTTL = 11 * time.Second
	if err := validateStrictSettings(); err != nil {
		t.Fatalf("lease longer than provider timeout should pass: %v", err)
	}
}
