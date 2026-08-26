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
