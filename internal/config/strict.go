package config

import (
	"fmt"
	"strings"

	"github.com/soulteary/herald/internal/serviceauth"
)

// contextBoundVerificationRequestEnvelopeBytes is the size of the smallest
// compact v2 verification request that carries a 36-character legacy
// challenge ID and all three required context bindings. Keep the JSON literal here
// so changes to the request envelope are visible instead of hidden in a magic
// number.
const contextBoundVerificationRequestEnvelopeBytes = len(`{"challenge_id":"00000000-0000-0000-0000-000000000000","code":"","expected_user_id":"x","expected_purpose":"x","expected_channel":"sms"}`)

func normalizedOneOf(value string, allowed ...string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if v == candidate {
			return true
		}
	}
	return false
}

// NormalizedProviderFailurePolicy returns the canonical value used by both
// validation and request handling.
func NormalizedProviderFailurePolicy() string {
	return strings.ToLower(strings.TrimSpace(ProviderFailurePolicy))
}

// validateStrictSettings rejects unknown enum values and unsafe numeric
// boundaries in every environment. These are operator mistakes, not relaxed
// development settings: silently guessing a value can select a weaker policy.
func validateStrictSettings() error {
	var problems []string

	problems = append(problems, strictEnumProblems()...)
	problems = append(problems, strictCredentialProblems()...)
	problems = append(problems, strictProxyProblems()...)
	problems = append(problems, strictHTTPProblems()...)
	problems = append(problems, strictDeliveryProblems()...)
	problems = append(problems, strictAuditProblems()...)

	if len(problems) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func strictEnumProblems() []string {
	var problems []string

	if !normalizedOneOf(EnvironmentValue, "", "development", "dev", "test", "testing", "production", "prod") {
		problems = append(problems, "ENVIRONMENT must be development, test, or production")
	}
	if _, err := serviceauth.ParseMode(RequestAuthMode); err != nil {
		problems = append(problems, err.Error())
	}
	if _, err := ServicePolicies(); err != nil {
		problems = append(problems, err.Error())
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
	return problems
}

func strictHTTPProblems() []string {
	var problems []string
	if MaxConcurrentHashOperations <= 0 {
		problems = append(problems, "HERALD_MAX_CONCURRENT_HASH_OPERATIONS must be positive")
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
	if LockoutChallengeThreshold < 1 {
		problems = append(problems, "LOCKOUT_CHALLENGE_THRESHOLD must be at least 1")
	}
	if CodeLength < 4 {
		problems = append(problems, "CODE_LENGTH must be at least 4")
	}
	if MaxBodyBytes > 0 && CodeLength > MaxBodyBytes-contextBoundVerificationRequestEnvelopeBytes {
		problems = append(problems, "CODE_LENGTH plus a context-bound v2 verification JSON envelope must not exceed HERALD_MAX_BODY_BYTES")
	}
	return problems
}

func strictDeliveryProblems() []string {
	var problems []string

	// A zero cooldown is rejected by the limiter at request time, so every
	// challenge request would fail with 503 after spending its quota.
	if ResendCooldown <= 0 {
		problems = append(problems, "RESEND_COOLDOWN must be positive")
	}
	if IdempotencyKeyTTL < 0 {
		problems = append(problems, "IDEMPOTENCY_KEY_TTL must not be negative")
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
	// An unresolved record that expired before the pending lease would reopen
	// the key for a second send sooner than an untouched claim does.
	pendingTTL := IdempotencyKeyTTL
	if pendingTTL == 0 {
		pendingTTL = ChallengeExpiry
	}
	if IdempotencyUnresolvedTTL <= 0 {
		problems = append(problems, "IDEMPOTENCY_UNRESOLVED_TTL must be positive")
	} else if pendingTTL > 0 && IdempotencyUnresolvedTTL <= pendingTTL {
		problems = append(problems, "IDEMPOTENCY_UNRESOLVED_TTL must be greater than IDEMPOTENCY_KEY_TTL (CHALLENGE_EXPIRY when that is 0)")
	}
	if HMACMaxDrift <= 0 {
		problems = append(problems, "HMAC_MAX_DRIFT must be positive")
	}
	return problems
}

func strictAuditProblems() []string {
	var problems []string

	if AuditEnabled && (strings.TrimSpace(AuditSpoolDir) == "" || AuditSpoolMaxBytes <= 0) {
		problems = append(problems, "AUDIT_SPOOL_DIR and positive AUDIT_SPOOL_MAX_BYTES are required when audit logging is enabled")
	}
	if AuditEnabled && AuditTTL <= 0 {
		problems = append(problems, "AUDIT_TTL must be positive when audit logging is enabled")
	}
	if AuditWriterQueueSize < 0 || AuditWriterWorkers < 0 {
		problems = append(problems, "audit writer queue size and worker count must not be negative")
	}

	return problems
}

func strictProxyProblems() []string {
	var problems []string

	if (strings.TrimSpace(TrustedProxyHeader) == "") != (len(TrustedProxies) == 0) {
		problems = append(problems, "HERALD_TRUSTED_PROXY_HEADER and HERALD_TRUSTED_PROXIES must be configured together")
	}
	for _, proxy := range TrustedProxies {
		if strings.TrimSpace(proxy) == "" {
			problems = append(problems, "HERALD_TRUSTED_PROXIES must not contain empty entries")
			break
		}
	}

	return problems
}
