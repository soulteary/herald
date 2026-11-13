package config

import (
	"strings"
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

func TestValidateStrictSettingsAcceptsLongOTPCodeLength(t *testing.T) {
	originalCodeLength, originalMaxBodyBytes := CodeLength, MaxBodyBytes
	defer func() {
		CodeLength, MaxBodyBytes = originalCodeLength, originalMaxBodyBytes
	}()

	CodeLength = 64
	MaxBodyBytes = 65536
	if err := validateStrictSettings(); err != nil {
		t.Fatalf("long OTP code lengths should be accepted: %v", err)
	}
}

func TestValidateStrictSettingsRequiresCodeToFitContextBoundV2Body(t *testing.T) {
	originalCodeLength, originalMaxBodyBytes := CodeLength, MaxBodyBytes
	defer func() {
		CodeLength, MaxBodyBytes = originalCodeLength, originalMaxBodyBytes
	}()

	MaxBodyBytes = 256
	CodeLength = MaxBodyBytes - contextBoundVerificationRequestEnvelopeBytes + 1
	if err := validateStrictSettings(); err == nil {
		t.Fatal("code that cannot fit in a context-bound v2 verification request must be rejected")
	}

	CodeLength--
	if err := validateStrictSettings(); err != nil {
		t.Fatalf("largest code that fits the context-bound v2 verification request should pass: %v", err)
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

func TestIdempotencyUnresolvedTTLBounds(t *testing.T) {
	origUnresolved, origKeyTTL, origExpiry, origTimeout := IdempotencyUnresolvedTTL, IdempotencyKeyTTL, ChallengeExpiry, ProviderTimeout
	t.Cleanup(func() {
		IdempotencyUnresolvedTTL, IdempotencyKeyTTL, ChallengeExpiry, ProviderTimeout = origUnresolved, origKeyTTL, origExpiry, origTimeout
	})
	ProviderTimeout = 10 * time.Second
	ChallengeExpiry = 5 * time.Minute

	for _, tc := range []struct {
		name               string
		unresolved, keyTTL time.Duration
		want               string
	}{
		{"zero", 0, time.Minute, "IDEMPOTENCY_UNRESOLVED_TTL must be positive"},
		{"negative", -time.Hour, time.Minute, "IDEMPOTENCY_UNRESOLVED_TTL must be positive"},
		{"equal to key TTL", time.Minute, time.Minute, "IDEMPOTENCY_UNRESOLVED_TTL must be greater than"},
		{"below key TTL", 30 * time.Second, time.Minute, "IDEMPOTENCY_UNRESOLVED_TTL must be greater than"},
		{"equal to challenge expiry when key TTL is 0", 5 * time.Minute, 0, "IDEMPOTENCY_UNRESOLVED_TTL must be greater than"},
		{"above key TTL", time.Minute + time.Second, time.Minute, ""},
		{"above challenge expiry when key TTL is 0", 6 * time.Minute, 0, ""},
		{"default", 24 * time.Hour, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			IdempotencyUnresolvedTTL, IdempotencyKeyTTL = tc.unresolved, tc.keyTTL
			problems := strings.Join(strictDeliveryProblems(), "; ")
			if tc.want == "" {
				if strings.Contains(problems, "IDEMPOTENCY_UNRESOLVED_TTL") {
					t.Fatalf("unexpected problem: %s", problems)
				}
				return
			}
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("problems=%q, want %q", problems, tc.want)
			}
		})
	}
}
