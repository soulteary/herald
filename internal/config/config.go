package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/soulteary/cli-kit/env"
	"github.com/soulteary/cli-kit/validator"
	"github.com/soulteary/herald/internal/serviceauth"
	logger "github.com/soulteary/logger-kit/v3"
	secure "github.com/soulteary/secure-kit/v2"
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

// parseEnvironment normalizes the ENVIRONMENT value. Unknown values return
// an invalid value; strict validation rejects it before startup.
func parseEnvironment(raw string) Environment {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "production", "prod":
		return EnvProduction
	case "test", "testing":
		return EnvTest
	case "development", "dev", "":
		return EnvDevelopment
	default:
		return ""
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

	// Proxy-derived client IPs are disabled unless both a header and an
	// explicit proxy IP/CIDR allowlist are configured.
	TrustedProxyHeader = env.Get("HERALD_TRUSTED_PROXY_HEADER", "")
	TrustedProxies     = env.GetStringSlice("HERALD_TRUSTED_PROXIES", []string{}, ",")

	// Metrics have a separate read-only credential. An empty key disables
	// metrics access in production; development retains anonymous scraping.
	MetricsAPIKey = env.Get("HERALD_METRICS_API_KEY", "")

	// ServicePoliciesJSON optionally maps HMAC key IDs to fixed services/actions.
	ServicePoliciesJSON = env.Get("HERALD_SERVICE_POLICIES", "")

	// Dedicated auth + listener for the test-code endpoint. The endpoint is only
	// mounted when TestCodeExposureEnabled() is true, is guarded by this key, and
	// should be bound to a loopback/admin listener.
	TestAPIKey       = env.Get("HERALD_TEST_API_KEY", "")
	TestListenerAddr = env.Get("HERALD_TEST_LISTENER_ADDR", "127.0.0.1:18082")

	// RiskAckPasswordlessRedis explicitly acknowledges running against a
	// passwordless Redis in production (discouraged). Without it, Validate()
	// refuses to start in production when REDIS_PASSWORD is empty.
	RiskAckPasswordlessRedis = env.GetBool("HERALD_RISK_ACK_PASSWORDLESS_REDIS", false)

	// Redis config
	RedisAddr     = env.Get("REDIS_ADDR", "localhost:6379")
	RedisPassword = env.Get("REDIS_PASSWORD", "")
	RedisDB       = env.GetInt("REDIS_DB", 0)
	// RedisUsername selects a Redis ACL user; empty uses the default user.
	RedisUsername = env.Get("REDIS_USERNAME", "")
	// Redis TLS is off by default. When enabled the server certificate is
	// verified against REDIS_TLS_CA_FILE (or the system roots when empty) and
	// REDIS_TLS_SERVER_NAME (or the REDIS_ADDR host when empty).
	RedisTLSEnabled    = env.GetBool("REDIS_TLS_ENABLED", false)
	RedisTLSCAFile     = env.Get("REDIS_TLS_CA_FILE", "")
	RedisTLSServerName = env.Get("REDIS_TLS_SERVER_NAME", "")

	// Logging
	LogLevel = env.Get("LOG_LEVEL", "info")

	// API Key for service-to-service authentication
	APIKey = env.Get("API_KEY", "")

	// Hash work budget shared by creation and both verification routes.
	MaxConcurrentHashOperations = env.GetInt("HERALD_MAX_CONCURRENT_HASH_OPERATIONS", 4)

	// Challenge config
	ChallengeExpiry   = env.GetDuration("CHALLENGE_EXPIRY", 5*time.Minute)
	MaxAttempts       = env.GetInt("MAX_ATTEMPTS", 5)
	ResendCooldown    = env.GetDuration("RESEND_COOLDOWN", 60*time.Second)
	CodeLength        = env.GetInt("CODE_LENGTH", 6)
	LockoutDuration   = env.GetDuration("LOCKOUT_DURATION", 10*time.Minute)
	IdempotencyKeyTTL = env.GetDuration("IDEMPOTENCY_KEY_TTL", 0) // 0 means use ChallengeExpiry
	// LockoutChallengeThreshold is the number of challenges a user must
	// exhaust (MAX_ATTEMPTS wrong codes each) within LOCKOUT_DURATION before
	// the user is locked. 1 locks on the first exhausted challenge.
	LockoutChallengeThreshold = env.GetInt("LOCKOUT_CHALLENGE_THRESHOLD", 3)
	// IdempotencyUnresolvedTTL bounds idempotency records whose delivery
	// outcome is unknown (a request that stopped between the send boundary and
	// its result). Until it elapses the key answers 409 delivery_unknown.
	IdempotencyUnresolvedTTL = env.GetDuration("IDEMPOTENCY_UNRESOLVED_TTL", 24*time.Hour)
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

	// VerifyV1Enabled keeps POST /v1/otp/verifications registered. The v1
	// route verifies by challenge ID and code only; it has no expected
	// user/purpose/channel binding. Set HERALD_VERIFY_V1_ENABLED=false once
	// all callers use the context-bound /v2/otp/verifications route.
	VerifyV1Enabled = env.GetBool("HERALD_VERIFY_V1_ENABLED", true)

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
	TOTPEnabled     = env.GetBool("HERALD_TOTP_ENABLED", false)
	TOTPBaseURL     = env.Get("HERALD_TOTP_BASE_URL", "") // Base URL of herald-totp service
	TOTPAPIKey      = env.Get("HERALD_TOTP_API_KEY", "")  // Optional API key for Herald to call herald-totp
	TOTPHMACSecret  = env.Get("HERALD_TOTP_HMAC_SECRET", "")
	TOTPHMACKeyID   = env.Get("HERALD_TOTP_HMAC_KEY_ID", "")
	TOTPHMACService = env.Get("HERALD_TOTP_HMAC_SERVICE", "")

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
	//   HERALD_HMAC_DEFAULT_KEY_ID selects a configuration lookup default. It
	//     never permits omitted wire key IDs in key-map HMAC v2/v3 requests.
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
	// TLS settings used only by the in-container loopback health probe.
	HealthcheckTLSCAFile         = env.Get("HERALD_HEALTHCHECK_TLS_CA_FILE", "")
	HealthcheckTLSServerName     = env.Get("HERALD_HEALTHCHECK_TLS_SERVER_NAME", "localhost")
	HealthcheckTLSClientCertFile = env.Get("HERALD_HEALTHCHECK_TLS_CLIENT_CERT_FILE", "")
	HealthcheckTLSClientKeyFile  = env.Get("HERALD_HEALTHCHECK_TLS_CLIENT_KEY_FILE", "")
	TestMode                     = env.GetBool("HERALD_TEST_MODE", false)

	// Audit logging config
	AuditSpoolDir        = env.Get("AUDIT_SPOOL_DIR", "./audit-spool")
	AuditSpoolMaxBytes   = env.GetInt("AUDIT_SPOOL_MAX_BYTES", 64*1024*1024)
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

	authMode, err := serviceauth.ParseMode(RequestAuthMode)
	if err != nil {
		return err
	}
	if parseEnvironment(EnvironmentValue) == "" {
		return fmt.Errorf("ENVIRONMENT must be development, test, or production")
	}
	if environmentUnset(os.LookupEnv) {
		log.Warn().Str("check", "config.environment").Msg(environmentUnsetWarning)
	}

	// Validate required configs
	if RedisAddr == "" {
		log.Warn().Msg("REDIS_ADDR is not set, using default: localhost:6379")
	} else if _, _, err := validator.ValidateHostPort(RedisAddr); err != nil && Env != EnvProduction {
		// Outside production an invalid address falls back to the local
		// default. Production keeps the configured value so Validate() rejects
		// it instead of connecting to an unintended Redis.
		log.Warn().Str("addr", RedisAddr).Err(err).Msg("Invalid REDIS_ADDR format, using default: localhost:6379")
		RedisAddr = "localhost:6379"
	}

	// Parse HMAC keys if provided
	if HMACKeysJSON != "" {
		if err := parseHMACKeys(); err != nil {
			if strings.TrimSpace(ServicePoliciesJSON) != "" {
				return err
			}
			// Outside production a malformed key map falls back to
			// HMAC_SECRET. Production is rejected by Validate().
			if Env != EnvProduction {
				log.Warn().Err(err).Msg("Failed to parse HERALD_HMAC_KEYS, falling back to HMAC_SECRET")
			}
		} else {
			log.Info().Int("count", len(hmacKeysMap)).Msg("HMAC keys loaded")
		}
	}

	if msg := missingCredentialWarning(authMode); msg != "" {
		log.Warn().Str("request_auth_mode", string(authMode)).Msg(msg)
	}

	for _, advisory := range startupAdvisories() {
		log.Warn().Str("check", advisory.check).Msg(advisory.message)
	}

	// Handle TLS_CA_CERT_FILE alias
	if TLSCACertFile == "" && TLSClientCAFile != "" {
		TLSCACertFile = TLSClientCAFile
	}

	// Set default IdempotencyKeyTTL if not set
	if IdempotencyKeyTTL == 0 {
		IdempotencyKeyTTL = ChallengeExpiry
	}

	if err := initializeSecretsAndPrivacy(); err != nil {
		return err
	}

	// Log configuration (excluding sensitive data)
	log.Info().
		Str("environment", string(Env)).
		Str("port", Port).
		Str("redis", maskSensitive(RedisAddr)).
		Int("redis_db", RedisDB).
		Bool("redis_tls", RedisTLSEnabled).
		Str("log_level", LogLevel).
		Dur("challenge_expiry", ChallengeExpiry).
		Int("max_attempts", MaxAttempts).
		Int("code_length", CodeLength).
		Msg("Configuration initialized")

	// Fail-closed validation. In production, misconfiguration must prevent
	// startup instead of degrading silently.
	if err := Validate(); err != nil {
		return err
	}

	return nil
}

// environmentUnsetWarning is logged when ENVIRONMENT is absent or empty. The
// default is development, where Validate() reports most problems as warnings,
// so a deployment that omits the variable runs without production checks.
const environmentUnsetWarning = "ENVIRONMENT is not set: defaulting to development, production validation is NOT enforced; set ENVIRONMENT explicitly to development, test, or production (a later release will refuse to start without it)"

// environmentUnset reports whether ENVIRONMENT is absent or blank. Both select
// the development default.
func environmentUnset(lookup func(string) (string, bool)) bool {
	value, set := lookup("ENVIRONMENT")
	return !set || strings.TrimSpace(value) == ""
}

// generatedSecretWarning is logged when no secret is configured and a random
// one is generated for this process.
const generatedSecretWarning = "no secret configured (HERALD_IDEMPOTENCY_SECRET, HMAC_SECRET, API_KEY, HERALD_HMAC_KEYS): generated a random secret for this process; it is not shared between instances and does not survive a restart, so idempotency keys, HMAC nonce records, rate-limit and cooldown digests are not consistent across instances or restarts; a multi-instance or restart-sensitive deployment must set HERALD_IDEMPOTENCY_SECRET and HERALD_PII_PEPPER"

// secretFromKeyMap derives a stable secret from the configured HMAC key map.
// Entries are hashed in key-ID order so the result does not depend on map
// iteration.
func secretFromKeyMap(keys map[string]string) string {
	ids := make([]string, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	h := sha256.New()
	h.Write([]byte("herald-derived-secret\x00"))
	for _, id := range ids {
		h.Write([]byte(id))
		h.Write([]byte{0})
		h.Write([]byte(keys[id]))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// generatedSecretBytes is the size of the per-process secret.
const generatedSecretBytes = 32

func generateProcessSecret() (string, error) {
	buf := make([]byte, generatedSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate process secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// missingCredentialWarning describes what the selected REQUEST_AUTH_MODE does
// when its credential is not configured. A missing credential never disables
// authentication: hmac_v2 and api_key reject every request until one is set.
// Only REQUEST_AUTH_MODE=none accepts unauthenticated requests.
func missingCredentialWarning(mode serviceauth.Mode) string {
	switch mode {
	case serviceauth.ModeHMACV2:
		if HMACSecret == "" && len(hmacKeysMap) == 0 {
			return "REQUEST_AUTH_MODE=hmac_v2 but neither HMAC_SECRET nor HERALD_HMAC_KEYS is set: all authenticated endpoints will reject requests with 401 until HMAC credentials are configured"
		}
	case serviceauth.ModeAPIKey:
		if APIKey == "" {
			return "REQUEST_AUTH_MODE=api_key but API_KEY is not set: all authenticated endpoints will reject requests with 401 until API_KEY is configured"
		}
	case serviceauth.ModeNone:
		return "REQUEST_AUTH_MODE=none: service-to-service authentication is disabled and requests are accepted without credentials; do not use outside development"
	}
	return ""
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

	problems = append(problems, listenerAndProviderProblems()...)
	problems = append(problems, explicitValueProblems()...)
	problems = append(problems, redisTransportProblems()...)

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

	problems = append(problems, productionPolicyProblems()...)

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
		for keyID, secret := range hmacKeysMap {
			if strings.TrimSpace(keyID) == "" {
				hmacKeysMap = nil
				parseErr = fmt.Errorf("HERALD_HMAC_KEYS contains an empty key ID")
				return
			}
			if strings.TrimSpace(secret) == "" {
				hmacKeysMap = nil
				parseErr = fmt.Errorf("HERALD_HMAC_KEYS key %q has an empty secret", keyID)
				return
			}
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
// If keyID is empty, returns the configured default. Single-secret mode only
// accepts the implicit ID "default"; unknown IDs must not create aliases.
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

	// A single secret has one identity. Accepting arbitrary IDs here would
	// let a caller mint new nonce and idempotency namespaces for that secret.
	if keyID != "" && keyID != "default" {
		return ""
	}
	return HMACSecret
}

// HasHMACKeys returns true if a nonempty HMAC key map is configured
func HasHMACKeys() bool {
	return len(hmacKeysMap) > 0
}

// GetHMACDefaultKeyID returns the effective default selected while parsing
// HERALD_HMAC_KEYS. It is empty when a multi-key configuration requires an
// explicit X-Key-Id header.
func GetHMACDefaultKeyID() string {
	return hmacDefaultKeyID
}

func initializeSecretsAndPrivacy() error {
	// Derive an idempotency secret if not explicitly configured. Prefer a
	// dedicated secret; otherwise reuse an existing service secret so keys are
	// still opaque and principal-namespaced. With no secret at all (possible
	// only outside production) a random per-process value is used: a fixed
	// value in the source would be known to anyone.
	if IdempotencySecret == "" {
		switch {
		case HMACSecret != "":
			IdempotencySecret = HMACSecret
		case APIKey != "":
			IdempotencySecret = APIKey
		case len(hmacKeysMap) > 0 && Env == EnvProduction:
			return fmt.Errorf("HERALD_IDEMPOTENCY_SECRET must be set in production when only HERALD_HMAC_KEYS is configured")
		case len(hmacKeysMap) > 0:
			// Outside production a key map is the only secret material. Derive
			// from it so every instance with the same map agrees on nonce and
			// idempotency keys, which a per-process value would not give.
			IdempotencySecret = secretFromKeyMap(hmacKeysMap)
		default:
			secret, err := generateProcessSecret()
			if err != nil {
				return err
			}
			IdempotencySecret = secret
			log.Warn().Str("check", "config.secrets").Msg(generatedSecretWarning)
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

	return nil
}

func listenerAndProviderProblems() []string {
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
	switch ClientCertMode {
	case "off":
		if TLSCACertFile != "" {
			problems = append(problems, "TLS client CA configured while CLIENT_CERT_MODE=off")
		}
	case "optional", "require":
		if TLSCACertFile == "" {
			problems = append(problems, "CLIENT_CERT_MODE="+ClientCertMode+" requires TLS_CA_CERT_FILE")
		}
	default:
		problems = append(problems, "CLIENT_CERT_MODE must be one of: off, optional, require")
	}

	// AUDIT_STORAGE_TYPE=loki has no backend implementation in audit-kit, so a
	// request for it would silently degrade to no-op storage. Flag it explicitly
	// (fatal in production, warning otherwise) rather than dropping audit records
	// without notice.
	if strings.Contains(strings.ToLower(AuditStorageType), "loki") {
		problems = append(problems, "AUDIT_STORAGE_TYPE=loki is not supported (no Loki audit backend); use database, file, or redis")
	}

	problems = append(problems, providerTransportProblems()...)
	problems = append(problems, totpSigningIdentityProblems()...)

	return problems
}

// explicitValueProblems reports values that were set explicitly but cannot be
// used. Initialize() substitutes a fallback for them outside production; in
// production a substitute would hide the mistake (a different Redis, or a
// single shared secret instead of the intended key map), so startup fails.
// Unset variables are not reported here.
func explicitValueProblems() []string {
	var problems []string
	if RedisAddr != "" {
		if _, _, err := validator.ValidateHostPort(RedisAddr); err != nil {
			problems = append(problems, "REDIS_ADDR is not a valid host:port address")
		}
	}
	problems = append(problems, booleanValueProblems(os.Getenv)...)
	problems = append(problems, numericValueProblems(os.Getenv)...)
	if HMACKeysJSON != "" && len(hmacKeysMap) == 0 {
		problems = append(problems, "HERALD_HMAC_KEYS is set but is not a usable key map (expected a JSON object of non-empty key IDs and secrets, containing HERALD_HMAC_DEFAULT_KEY_ID when that is set)")
	}
	return problems
}

func productionPolicyProblems() []string {
	var problems []string
	// Production-only fail-closed checks.
	problems = append(problems, productionAuthProblems()...)
	if TestMode {
		problems = append(problems, "HERALD_TEST_MODE=true is forbidden in production")
	}
	if NormalizedProviderFailurePolicy() == "soft" {
		problems = append(problems, "PROVIDER_FAILURE_POLICY=soft is forbidden in production; use strict")
	}
	if RedisPassword == "" && !RiskAckPasswordlessRedis {
		problems = append(problems, "passwordless Redis in production (set REDIS_PASSWORD or HERALD_RISK_ACK_PASSWORDLESS_REDIS=true to override)")
	}
	if _, err := CORSOrigins(); err != nil {
		problems = append(problems, err.Error())
	}
	switch strings.ToLower(strings.TrimSpace(RequestAuthMode)) {
	case "none", "off", "disabled":
		problems = append(problems, "REQUEST_AUTH_MODE="+RequestAuthMode+" is forbidden in production")
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

	return problems
}

func providerTransportProblems() []string {
	var problems []string
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

	return problems
}

func productionAuthProblems() []string {
	var problems []string
	problems = append(problems, validateProductionSecretMaterial()...)
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

	return problems
}

// booleanEnvNames lists every variable read with env.GetBool. That helper
// returns the default for any value strconv.ParseBool does not accept, so
// "yes", "on" or a value with stray whitespace silently selects the default.
var booleanEnvNames = []string{
	"AUDIT_ENABLED",
	"AUDIT_MASK_DESTINATION",
	"HERALD_RISK_ACK_PASSWORDLESS_REDIS",
	"HERALD_TEST_MODE",
	"HERALD_TOTP_ENABLED",
	"HERALD_VERIFY_V1_ENABLED",
	"HMAC_V1_ENABLED",
	"OTLP_ENABLED",
	"REDIS_TLS_ENABLED",
}

// integerEnvNames and durationEnvNames list every variable read with
// env.GetInt and env.GetDuration. Those helpers also return the default for a
// value they cannot parse, so HMAC_MAX_DRIFT=60 (no unit) or
// RATE_LIMIT_PER_IP=5/m would silently select the default.
var integerEnvNames = []string{
	"AUDIT_SPOOL_MAX_BYTES",
	"AUDIT_WRITER_QUEUE_SIZE",
	"AUDIT_WRITER_WORKERS",
	"CODE_LENGTH",
	"HERALD_MAX_BODY_BYTES",
	"HERALD_MAX_CONCURRENT_HASH_OPERATIONS",
	"LOCKOUT_CHALLENGE_THRESHOLD",
	"MAX_ATTEMPTS",
	"PROVIDER_MAX_RESPONSE_BYTES",
	"RATE_LIMIT_PER_DESTINATION",
	"RATE_LIMIT_PER_IP",
	"RATE_LIMIT_PER_USER",
	"REDIS_DB",
	"SMTP_PORT",
}

var durationEnvNames = []string{
	"AUDIT_TTL",
	"CHALLENGE_EXPIRY",
	"HERALD_IDLE_TIMEOUT",
	"HERALD_READ_TIMEOUT",
	"HERALD_WRITE_TIMEOUT",
	"HMAC_MAX_DRIFT",
	"IDEMPOTENCY_KEY_TTL",
	"IDEMPOTENCY_UNRESOLVED_TTL",
	"LOCKOUT_DURATION",
	"PROVIDER_TIMEOUT",
	"RESEND_COOLDOWN",
}

// numericValueProblems reports integer and duration variables whose value
// would be ignored.
func numericValueProblems(getenv func(string) string) []string {
	var problems []string
	for _, name := range integerEnvNames {
		raw := getenv(name)
		if raw == "" {
			continue
		}
		if _, err := strconv.Atoi(raw); err != nil {
			problems = append(problems, name+" is not a valid integer; the default would be used instead")
		}
	}
	for _, name := range durationEnvNames {
		raw := getenv(name)
		if raw == "" {
			continue
		}
		if _, err := time.ParseDuration(raw); err != nil {
			problems = append(problems, name+" is not a valid duration (use a unit, for example 60s or 5m); the default would be used instead")
		}
	}
	return problems
}

// booleanValueProblems reports boolean variables whose value would be ignored.
func booleanValueProblems(getenv func(string) string) []string {
	var problems []string
	for _, name := range booleanEnvNames {
		raw := getenv(name)
		if raw == "" {
			continue
		}
		if _, err := strconv.ParseBool(raw); err != nil {
			problems = append(problems, name+" is not a valid boolean (use true or false); the default would be used instead")
		}
	}
	return problems
}
