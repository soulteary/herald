package config

import (
	"fmt"
	"strings"
)

// verificationRequestEnvelopeBytes reserves the JSON field names, punctuation,
// and the currently issued challenge ID so every generated code can fit in at
// least the v1 verification request.
const verificationRequestEnvelopeBytes = 64

func normalizedOneOf(value string, allowed ...string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if v == candidate {
			return true
		}
	}
	return false
}

// validateStrictSettings rejects unknown enum values and unsafe numeric
// boundaries in every environment. These are operator mistakes, not relaxed
// development settings: silently guessing a value can select a weaker policy.
func validateStrictSettings() error {
	var problems []string

	if !normalizedOneOf(EnvironmentValue, "", "development", "dev", "test", "testing", "production", "prod") {
		problems = append(problems, "ENVIRONMENT must be development, test, or production")
	}
	if !normalizedOneOf(RequestAuthMode, "hmac_v2", "hmac-v2", "hmac", "api_key", "apikey", "none", "off", "disabled") {
		problems = append(problems, "REQUEST_AUTH_MODE must be hmac_v2, api_key, or none")
	}
	if !normalizedOneOf(ClientCertMode, "off", "optional", "require") {
		problems = append(problems, "CLIENT_CERT_MODE must be off, optional, or require")
	}
	if (HealthcheckTLSClientCertFile == "") != (HealthcheckTLSClientKeyFile == "") {
		problems = append(problems, "HERALD_HEALTHCHECK_TLS_CLIENT_CERT_FILE and HERALD_HEALTHCHECK_TLS_CLIENT_KEY_FILE must be set together")
	}
	if !normalizedOneOf(ProviderFailurePolicy, "strict", "soft") {
		problems = append(problems, "PROVIDER_FAILURE_POLICY must be strict or soft")
	}
	if !normalizedOneOf(ProviderRedirectPolicy, "deny", "same-origin") {
		problems = append(problems, "PROVIDER_REDIRECT_POLICY must be deny or same-origin")
	}
	if (strings.TrimSpace(TrustedProxyHeader) == "") != (len(TrustedProxies) == 0) {
		problems = append(problems, "HERALD_TRUSTED_PROXY_HEADER and HERALD_TRUSTED_PROXIES must be configured together")
	}
	for _, proxy := range TrustedProxies {
		if strings.TrimSpace(proxy) == "" {
			problems = append(problems, "HERALD_TRUSTED_PROXIES must not contain empty entries")
			break
		}
	}

	if MaxBodyBytes <= 0 {
		problems = append(problems, "HERALD_MAX_BODY_BYTES must be positive")
	}
	if ReadTimeout <= 0 || WriteTimeout <= 0 || IdleTimeout <= 0 {
		problems = append(problems, "HERALD_READ_TIMEOUT, HERALD_WRITE_TIMEOUT, and HERALD_IDLE_TIMEOUT must be positive")
	}
	if ChallengeExpiry <= 0 || MaxAttempts <= 0 || LockoutDuration <= 0 {
		problems = append(problems, "CHALLENGE_EXPIRY, MAX_ATTEMPTS, and LOCKOUT_DURATION must be positive")
	}
	if CodeLength < 4 {
		problems = append(problems, "CODE_LENGTH must be at least 4")
	}
	if MaxBodyBytes > 0 && CodeLength > MaxBodyBytes-verificationRequestEnvelopeBytes {
		problems = append(problems, "CODE_LENGTH plus the verification JSON envelope must not exceed HERALD_MAX_BODY_BYTES")
	}
	if ResendCooldown < 0 || IdempotencyKeyTTL < 0 {
		problems = append(problems, "RESEND_COOLDOWN and IDEMPOTENCY_KEY_TTL must not be negative")
	}
	if RateLimitPerUser <= 0 || RateLimitPerIP <= 0 || RateLimitPerDestination <= 0 {
		problems = append(problems, "all rate limits must be positive")
	}
	if ProviderTimeout <= 0 || ProviderMaxResponseBytes <= 0 {
		problems = append(problems, "PROVIDER_TIMEOUT and PROVIDER_MAX_RESPONSE_BYTES must be positive")
	}
	if IdempotencyKeyTTL > 0 && ProviderTimeout > 0 && IdempotencyKeyTTL <= ProviderTimeout {
		problems = append(problems, "IDEMPOTENCY_KEY_TTL must be greater than PROVIDER_TIMEOUT")
	}
	if HMACMaxDrift <= 0 {
		problems = append(problems, "HMAC_MAX_DRIFT must be positive")
	}
	if AuditEnabled && AuditTTL <= 0 {
		problems = append(problems, "AUDIT_TTL must be positive when audit logging is enabled")
	}
	if AuditWriterQueueSize < 0 || AuditWriterWorkers < 0 {
		problems = append(problems, "audit writer queue size and worker count must not be negative")
	}

	if len(problems) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}
