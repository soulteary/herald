package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/soulteary/cli-kit/env"
	"github.com/soulteary/cli-kit/validator"
	logger "github.com/soulteary/logger-kit/v2"
	secure "github.com/soulteary/secure-kit"
)

// log is the package-level logger, initialized in Initialize
var log *logger.Logger

// Environment describes the deployment environment. Production applies the
// strictest Validate() checks; development/test relax some of them.
type Environment string

const (
	// EnvDevelopment is the default relaxed environment.
	EnvDevelopment Environment = "development"
	// EnvTest enables test-only affordances (debug_code, test-code endpoint)
	// but only in combination with HERALD_TEST_MODE=true.
	EnvTest Environment = "test"
	// EnvProduction applies fail-closed validation of security-sensitive config.
	EnvProduction Environment = "production"
)

// parseEnvironment normalizes the ENVIRONMENT value. Unknown values default to
// development so a typo never silently grants production trust while still not
// accidentally enabling test affordances.
func parseEnvironment(raw string) Environment {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "production", "prod":
		return EnvProduction
	case "test", "testing":
		return EnvTest
	case "development", "dev", "":
		return EnvDevelopment
	default:
		return EnvDevelopment
	}
}

// IsProduction reports whether Herald is running in the production environment.
func IsProduction() bool { return Env == EnvProduction }

// IsTestEnv reports whether Herald is running in the test environment.
func IsTestEnv() bool { return Env == EnvTest }

// TestCodeExposureEnabled reports whether plaintext test codes may be exposed
// (debug_code field and the /test/code endpoint). This requires BOTH the test
// environment AND HERALD_TEST_MODE=true, and is never true in production.
func TestCodeExposureEnabled() bool {
	return Env == EnvTest && TestMode
}

var (
	// Environment: development | test | production. Controls Validate() strictness.
	EnvironmentValue = env.Get("ENVIRONMENT", "development")
	Env              = parseEnvironment(EnvironmentValue)

	// Server config
	Port = env.Get("PORT", ":8082")

	// Fiber/server hardening limits.
	MaxBodyBytes = env.GetInt("HERALD_MAX_BODY_BYTES", 64*1024) // request body cap
	ReadTimeout  = env.GetDuration("HERALD_READ_TIMEOUT", 15*time.Second)
	WriteTimeout = env.GetDuration("HERALD_WRITE_TIMEOUT", 15*time.Second)
	IdleTimeout  = env.GetDuration("HERALD_IDLE_TIMEOUT", 60*time.Second)

	// CORS is disabled by default. Set an explicit comma-separated allowlist to
	// enable it; "*" is rejected in production by Validate().
	CORSAllowOrigins = env.Get("HERALD_CORS_ALLOW_ORIGINS", "")

	// Dedicated auth + listener for the test-code endpoint. The endpoint is only
	// mounted when TestCodeExposureEnabled() is true, is guarded by this key, and
	// should be bound to a loopback/admin listener.
	TestAPIKey       = env.Get("HERALD_TEST_API_KEY", "")
	TestListenerAddr = env.Get("HERALD_TEST_LISTENER_ADDR", "127.0.0.1:0")

	// RiskAckPasswordlessRedis explicitly acknowledges running against a
	// passwordless Redis in production (discouraged). Without it, Validate()
	// refuses to start in production when REDIS_PASSWORD is empty.
	RiskAckPasswordlessRedis = env.GetBool("HERALD_RISK_ACK_PASSWORDLESS_REDIS", false)

	// Redis config
	RedisAddr     = env.Get("REDIS_ADDR", "localhost:6379")
	RedisPassword = env.Get("REDIS_PASSWORD", "")
	RedisDB       = env.GetInt("REDIS_DB", 0)

	// Logging
	LogLevel = env.Get("LOG_LEVEL", "info")

	// API Key for service-to-service authentication
	APIKey = env.Get("API_KEY", "")

	// Challenge config
	ChallengeExpiry   = env.GetDuration("CHALLENGE_EXPIRY", 5*time.Minute)
	MaxAttempts       = env.GetInt("MAX_ATTEMPTS", 5)
	ResendCooldown    = env.GetDuration("RESEND_COOLDOWN", 60*time.Second)
	CodeLength        = env.GetInt("CODE_LENGTH", 6)
	LockoutDuration   = env.GetDuration("LOCKOUT_DURATION", 10*time.Minute)
	IdempotencyKeyTTL = env.GetDuration("IDEMPOTENCY_KEY_TTL", 0) // 0 means use ChallengeExpiry
	// IdempotencySecret derives opaque Redis keys for idempotency records so a
	// raw client-supplied key is never used directly and principals cannot
	// collide. Falls back to HMAC_SECRET / API_KEY when unset (see Initialize).
	IdempotencySecret = env.Get("HERALD_IDEMPOTENCY_SECRET", "")
	// PIIPepper keys the peppered digests used for privacy-preserving
	// rate-limit keys and audit fingerprints so raw email/phone/user values
	// never land in Redis keys or metrics. Falls back to IdempotencySecret
	// when unset (see Initialize).
	PIIPepper       = env.Get("HERALD_PII_PEPPER", "")
	AllowedPurposes = env.GetStringSlice("ALLOWED_PURPOSES", []string{"login"}, ",") // Comma-separated list: "login,reset,bind,stepup"

	// Rate limiting config
	RateLimitPerUser        = env.GetInt("RATE_LIMIT_PER_USER", 10)        // per hour
	RateLimitPerIP          = env.GetInt("RATE_LIMIT_PER_IP", 5)           // per minute
	RateLimitPerDestination = env.GetInt("RATE_LIMIT_PER_DESTINATION", 10) // per hour

	// Provider config
	SMTPHost              = env.Get("SMTP_HOST", "")
	SMTPPort              = env.GetInt("SMTP_PORT", 587)
	SMTPUser              = env.Get("SMTP_USER", "")
	SMTPPassword          = env.Get("SMTP_PASSWORD", "")
	SMTPFrom              = env.Get("SMTP_FROM", "")
	ProviderFailurePolicy = env.Get("PROVIDER_FAILURE_POLICY", "soft") // "strict" | "soft"

	// Provider transport hardening (Phase 3).
	ProviderTimeout          = env.GetDuration("PROVIDER_TIMEOUT", 10*time.Second)
	ProviderMaxResponseBytes = env.GetInt("PROVIDER_MAX_RESPONSE_BYTES", 1<<20) // 1 MiB
	ProviderRedirectPolicy   = env.Get("PROVIDER_REDIRECT_POLICY", "deny")      // "deny" | "same-origin"

	// SMS Provider config (HTTP API mode - recommended)
	SMSProvider   = env.Get("SMS_PROVIDER", "")     // Provider name (e.g., "aliyun", "tencent", "http")
	SMSAPIBaseURL = env.Get("SMS_API_BASE_URL", "") // HTTP API base URL for SMS provider
	SMSAPIKey     = env.Get("SMS_API_KEY", "")      // HTTP API key for SMS provider

	// TOTP (herald-totp): Herald proxies TOTP to herald-totp service when enabled
	TOTPEnabled    = env.GetBool("HERALD_TOTP_ENABLED", false)
	TOTPBaseURL    = env.Get("HERALD_TOTP_BASE_URL", "") // Base URL of herald-totp service
	TOTPAPIKey     = env.Get("HERALD_TOTP_API_KEY", "")  // Optional API key for Herald to call herald-totp
	TOTPHMACSecret = env.Get("HERALD_TOTP_HMAC_SECRET", "")

	// DingTalk channel: Herald calls herald-dingtalk via HTTP (no DingTalk credentials in Herald)
	HeraldDingtalkAPIURL = env.Get("HERALD_DINGTALK_API_URL", "") // Base URL of herald-dingtalk service
	HeraldDingtalkAPIKey = env.Get("HERALD_DINGTALK_API_KEY", "") // Optional API key for herald-dingtalk

	// Email channel via herald-smtp: Herald calls herald-smtp via HTTP (no SMTP credentials in Herald when set)
	HeraldSMTPAPIURL = env.Get("HERALD_SMTP_API_URL", "") // Base URL of herald-smtp service; when set, built-in SMTP is not used
	HeraldSMTPAPIKey = env.Get("HERALD_SMTP_API_KEY", "") // Optional API key for herald-smtp

	// Service authentication (HMAC)
	HMACSecret   = env.Get("HMAC_SECRET", "")
	HMACKeysJSON = env.Get("HERALD_HMAC_KEYS", "") // JSON format: {"key-id-1":"secret-1","key-id-2":"secret-2"}
	ServiceName  = env.Get("SERVICE_NAME", "herald")

	// Phase 4: HMAC v2 replay-resistant auth policy.
	//   REQUEST_AUTH_MODE selects the request-body auth scheme:
	//     "hmac_v2" (default, replay-resistant), "api_key", or "none" (dev only).
	//   CLIENT_CERT_MODE controls mTLS handling ("off" | "optional" | "require"),
	//     kept independent from REQUEST_AUTH_MODE so a client cert is not
	//     conflated with request-body integrity.
	//   HMAC_V1_ENABLED explicitly re-enables the legacy v1 scheme for one
	//     migration cycle; it is off by default.
	//   HERALD_HMAC_DEFAULT_KEY_ID must be set explicitly when using a multi-key
	//     map with unsigned X-Key-Id (no random/arbitrary default).
	//   HMAC_MAX_DRIFT bounds the accepted timestamp skew (default 60s).
	RequestAuthMode  = env.Get("REQUEST_AUTH_MODE", "hmac_v2")
	ClientCertMode   = env.Get("CLIENT_CERT_MODE", "off")
	HMACV1Enabled    = env.GetBool("HMAC_V1_ENABLED", false)
	HMACDefaultKeyID = env.Get("HERALD_HMAC_DEFAULT_KEY_ID", "")
	HMACMaxDrift     = env.GetDuration("HMAC_MAX_DRIFT", 60*time.Second)
	NonceStorePrefix = env.Get("HERALD_NONCE_PREFIX", "otp:nonce:")

	// HMAC keys map (parsed from HERALD_HMAC_KEYS)
	hmacKeysMap      map[string]string
	hmacKeysMapOnce  sync.Once
	hmacDefaultKeyID string // Default key ID if X-Key-Id not provided

	// TLS/mTLS config
	TLSCertFile     = env.Get("TLS_CERT_FILE", "")
	TLSKeyFile      = env.Get("TLS_KEY_FILE", "")
	TLSCACertFile   = env.Get("TLS_CA_CERT_FILE", "")   // For mTLS (client certificate verification)
	TLSClientCAFile = env.Get("TLS_CLIENT_CA_FILE", "") // Alias for TLS_CA_CERT_FILE
	TestMode        = env.GetBool("HERALD_TEST_MODE", false)

	// Session storage config
	SessionStorageEnabled = env.GetBool("HERALD_SESSION_STORAGE_ENABLED", false)
	SessionDefaultTTL     = env.GetDuration("HERALD_SESSION_DEFAULT_TTL", 1*time.Hour)
	SessionKeyPrefix      = env.Get("HERALD_SESSION_KEY_PREFIX", "session:")

	// Audit logging config
	AuditEnabled         = env.GetBool("AUDIT_ENABLED", true)
	AuditMaskDestination = env.GetBool("AUDIT_MASK_DESTINATION", false)
	AuditTTL             = env.GetDuration("AUDIT_TTL", 7*24*time.Hour) // 7 days default

	// Audit persistent storage config
	AuditStorageType     = env.Get("AUDIT_STORAGE_TYPE", "") // "database", "file", "redis", or comma-separated list
	AuditDatabaseURL     = env.Get("AUDIT_DATABASE_URL", "")
	AuditTableName       = env.Get("AUDIT_TABLE_NAME", "audit_logs")
	AuditFilePath        = env.Get("AUDIT_FILE_PATH", "")
	AuditWriterQueueSize = env.GetInt("AUDIT_WRITER_QUEUE_SIZE", 1000)
	AuditWriterWorkers   = env.GetInt("AUDIT_WRITER_WORKERS", 2)

	// Template config
	TemplateDir = env.Get("TEMPLATE_DIR", "") // Optional: path to template directory

	// OpenTelemetry config
	OTLPEnabled  = env.GetBool("OTLP_ENABLED", false)
	OTLPEndpoint = env.Get("OTLP_ENDPOINT", "") // e.g., "http://localhost:4318" for OTLP HTTP
)

// Initialize validates and initializes configuration
func Initialize(l *logger.Logger) error {
	log = l

	// Validate required configs
	if RedisAddr == "" {
		log.Warn().Msg("REDIS_ADDR is not set, using default: localhost:6379")
	} else {
		// Validate Redis address format using cli-kit validator
		if _, _, err := validator.ValidateHostPort(RedisAddr); err != nil {
			log.Warn().Str("addr", RedisAddr).Err(err).Msg("Invalid REDIS_ADDR format, using default: localhost:6379")
			RedisAddr = "localhost:6379"
		}
	}

	// Parse HMAC keys if provided
	if HMACKeysJSON != "" {
		if err := parseHMACKeys(); err != nil {
			log.Warn().Err(err).Msg("Failed to parse HERALD_HMAC_KEYS, falling back to HMAC_SECRET")
		} else {
			log.Info().Int("count", len(hmacKeysMap)).Msg("HMAC keys loaded")
			// Set default key ID to first key if available
			for keyID := range hmacKeysMap {
				hmacDefaultKeyID = keyID
				break
			}
		}
	}

	if APIKey == "" && HMACSecret == "" && len(hmacKeysMap) == 0 {
		log.Warn().Msg("Neither API_KEY nor HMAC_SECRET/HERALD_HMAC_KEYS is set, service-to-service authentication will be disabled")
	}

	// Handle TLS_CA_CERT_FILE alias
	if TLSCACertFile == "" && TLSClientCAFile != "" {
		TLSCACertFile = TLSClientCAFile
	}

	// Set default IdempotencyKeyTTL if not set
	if IdempotencyKeyTTL == 0 {
		IdempotencyKeyTTL = ChallengeExpiry
	}

	// Derive an idempotency secret if not explicitly configured. Prefer a
	// dedicated secret; otherwise reuse an existing service secret so keys are
	// still opaque and principal-namespaced.
	if IdempotencySecret == "" {
		switch {
		case HMACSecret != "":
			IdempotencySecret = HMACSecret
		case APIKey != "":
			IdempotencySecret = APIKey
		default:
			IdempotencySecret = "herald-idempotency-fallback"
		}
	}

	// Derive a PII pepper if not explicitly configured so rate-limit keys and
	// audit fingerprints are still non-reversible. Production SHOULD set
	// HERALD_PII_PEPPER explicitly (recommended, warned in Validate).
	if PIIPepper == "" {
		PIIPepper = IdempotencySecret
	}

	// Privacy default: in production, mask destinations in audit records unless
	// the operator has explicitly opted out. This prevents raw phone/email from
	// landing in the audit store by default.
	if Env == EnvProduction && AuditEnabled {
		if _, set := os.LookupEnv("AUDIT_MASK_DESTINATION"); !set {
			AuditMaskDestination = true
			log.Info().Msg("Production default: AUDIT_MASK_DESTINATION=true (set it explicitly to override)")
		}
	}

	// Log configuration (excluding sensitive data)
	log.Info().
		Str("environment", string(Env)).
		Str("port", Port).
		Str("redis", maskSensitive(RedisAddr)).
		Int("redis_db", RedisDB).
		Str("log_level", LogLevel).
		Dur("challenge_expiry", ChallengeExpiry).
		Int("max_attempts", MaxAttempts).
		Int("code_length", CodeLength).
		Bool("session_storage", SessionStorageEnabled).
		Msg("Configuration initialized")

	// Fail-closed validation. In production, misconfiguration must prevent
	// startup instead of degrading silently.
	if err := Validate(); err != nil {
		return err
	}

	return nil
}

// authConfigured reports whether at least one service-to-service auth mechanism
// is configured (API key, single HMAC secret, or a multi-key HMAC map).
func authConfigured() bool {
	return APIKey != "" || HMACSecret != "" || len(hmacKeysMap) > 0
}

// Validate enforces security-sensitive invariants. Most checks are only fatal
// in production; development/test environments log warnings instead so local
// workflows are not blocked. Returns a non-nil error when startup must abort.
func Validate() error {
	if err := validateStrictSettings(); err != nil {
		return err
	}

	var problems []string

	// TLS must be configured coherently regardless of environment: a cert
	// without a key (or vice versa) is a misconfiguration that would otherwise
	// silently fall back to plaintext.
	if (TLSCertFile == "") != (TLSKeyFile == "") {
		problems = append(problems, "half-configured TLS: both TLS_CERT_FILE and TLS_KEY_FILE must be set together")
	}
	// A client CA (mTLS) requires a server cert/key to terminate TLS.
	if TLSCACertFile != "" && (TLSCertFile == "" || TLSKeyFile == "") {
		problems = append(problems, "TLS client CA configured without server certificate/key")
	}

	// AUDIT_STORAGE_TYPE=loki has no backend implementation in audit-kit, so a
	// request for it would silently degrade to no-op storage. Flag it explicitly
	// (fatal in production, warning otherwise) rather than dropping audit records
	// without notice.
	if strings.Contains(strings.ToLower(AuditStorageType), "loki") {
		problems = append(problems, "AUDIT_STORAGE_TYPE=loki is not supported (no Loki audit backend); use database, file, or redis")
	}

	// Provider transport hardening: reject plaintext provider URLs in production.
	for _, u := range []struct{ name, url string }{
		{"HERALD_SMTP_API_URL", HeraldSMTPAPIURL},
		{"SMS_API_BASE_URL", SMSAPIBaseURL},
		{"HERALD_DINGTALK_API_URL", HeraldDingtalkAPIURL},
		{"HERALD_TOTP_BASE_URL", TOTPBaseURL},
	} {
		if u.url != "" && !strings.HasPrefix(strings.ToLower(u.url), "https://") {
			problems = append(problems, u.name+" must use https:// in production")
		}
	}

	if Env != EnvProduction {
		// Non-production: surface issues as warnings and continue, but never
		// enable production trust.
		if len(problems) > 0 && log != nil {
			for _, p := range problems {
				log.Warn().Str("check", "config.validate").Msg(p)
			}
		}
		return nil
	}

	// Production-only fail-closed checks.
	if !authConfigured() {
		problems = append(problems, "no service-to-service authentication configured (set API_KEY, HMAC_SECRET, or HERALD_HMAC_KEYS)")
	}
	// Validate the credentials required by the selected request-auth mode. Merely
	// having a credential for a different mode must not let production start in a
	// permanently unhealthy state where every business request is rejected.
	switch strings.ToLower(strings.TrimSpace(RequestAuthMode)) {
	case "api_key", "apikey":
		if APIKey == "" {
			problems = append(problems, "REQUEST_AUTH_MODE=api_key requires API_KEY")
		}
	case "hmac_v2", "hmac-v2", "hmac":
		if HMACSecret == "" && len(hmacKeysMap) == 0 {
			problems = append(problems, "REQUEST_AUTH_MODE=hmac_v2 requires HMAC_SECRET or HERALD_HMAC_KEYS")
		}
	}
	if TestMode {
		problems = append(problems, "HERALD_TEST_MODE=true is forbidden in production")
	}
	if ProviderFailurePolicy == "soft" {
		problems = append(problems, "PROVIDER_FAILURE_POLICY=soft is forbidden in production; use strict")
	}
	if RedisPassword == "" && !RiskAckPasswordlessRedis {
		problems = append(problems, "passwordless Redis in production (set REDIS_PASSWORD or HERALD_RISK_ACK_PASSWORDLESS_REDIS=true to override)")
	}
	if strings.TrimSpace(CORSAllowOrigins) == "*" {
		problems = append(problems, "CORS wildcard (HERALD_CORS_ALLOW_ORIGINS=*) is forbidden in production")
	}
	if strings.EqualFold(RequestAuthMode, "none") {
		problems = append(problems, "REQUEST_AUTH_MODE=none is forbidden in production")
	}
	if HMACV1Enabled {
		// Not fatal, but the legacy scheme is not replay-resistant; warn loudly.
		if log != nil {
			log.Warn().Str("check", "config.validate").Msg("HMAC_V1_ENABLED=true in production: the legacy v1 scheme is not replay-resistant; disable after migration")
		}
	}

	// Recommendation (non-fatal): an explicit PII pepper distinct from the auth
	// secret keeps rate-limit keys / audit fingerprints non-reversible even if
	// an auth secret is rotated. Warn rather than refuse start.
	if env.Get("HERALD_PII_PEPPER", "") == "" && log != nil {
		log.Warn().Str("check", "config.validate").Msg("HERALD_PII_PEPPER not set; deriving PII pepper from an existing secret (set an explicit pepper in production)")
	}

	if len(problems) > 0 {
		return fmt.Errorf("production config validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

// GetPort returns the server port
func GetPort() string {
	if !strings.HasPrefix(Port, ":") {
		return ":" + Port
	}
	return Port
}

func maskSensitive(s string) string {
	return secure.MaskAPIKey(s)
}

// parseHMACKeys parses HERALD_HMAC_KEYS JSON string into a map
func parseHMACKeys() error {
	var parseErr error
	hmacKeysMapOnce.Do(func() {
		hmacKeysMap = make(map[string]string)
		if HMACKeysJSON == "" {
			return
		}

		if err := json.Unmarshal([]byte(HMACKeysJSON), &hmacKeysMap); err != nil {
			if log != nil {
				log.Error().Err(err).Msg("Failed to parse HERALD_HMAC_KEYS JSON")
			}
			hmacKeysMap = nil
			parseErr = fmt.Errorf("failed to parse HMAC keys JSON: %w", err)
			return
		}

		if len(hmacKeysMap) == 0 {
			if log != nil {
				log.Warn().Msg("HERALD_HMAC_KEYS is empty or contains no keys")
			}
			hmacKeysMap = nil
			parseErr = fmt.Errorf("HERALD_HMAC_KEYS contains no keys")
			return
		}

		// Choose the default key id. Prefer an explicitly configured
		// HERALD_HMAC_DEFAULT_KEY_ID (deterministic, no reliance on map order).
		// Only when there is exactly one key do we default to it implicitly.
		if HMACDefaultKeyID != "" {
			if _, ok := hmacKeysMap[HMACDefaultKeyID]; ok {
				hmacDefaultKeyID = HMACDefaultKeyID
			} else {
				parseErr = fmt.Errorf("HERALD_HMAC_DEFAULT_KEY_ID %q not present in HERALD_HMAC_KEYS", HMACDefaultKeyID)
				hmacKeysMap = nil
				return
			}
		} else if len(hmacKeysMap) == 1 {
			for keyID := range hmacKeysMap {
				hmacDefaultKeyID = keyID
			}
		}
		// With multiple keys and no explicit default, hmacDefaultKeyID stays
		// empty so a request without X-Key-Id is rejected rather than mapped to
		// an arbitrary key.
	})

	if parseErr != nil {
		return parseErr
	}

	if hmacKeysMap == nil {
		return fmt.Errorf("failed to parse HMAC keys")
	}
	return nil
}

// GetHMACSecret returns the HMAC secret for the given key ID
// If keyID is empty, returns the default key or HMACSecret
func GetHMACSecret(keyID string) string {
	// If HMAC keys map is configured, use it
	if len(hmacKeysMap) > 0 {
		if keyID == "" {
			// Use default key ID if not provided
			keyID = hmacDefaultKeyID
		}
		if secret, ok := hmacKeysMap[keyID]; ok {
			return secret
		}
		// Key ID not found, return empty (will fail authentication)
		if log != nil {
			log.Debug().Str("key_id", keyID).Msg("HMAC key ID not found in configured keys")
		}
		return ""
	}

	// Fallback to single HMACSecret (backward compatibility)
	return HMACSecret
}

// HasHMACKeys returns true if multiple HMAC keys are configured
func HasHMACKeys() bool {
	return len(hmacKeysMap) > 0
}
