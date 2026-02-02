package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/soulteary/cli-kit/env"
	"github.com/soulteary/cli-kit/validator"
	logger "github.com/soulteary/logger-kit"
	secure "github.com/soulteary/secure-kit"
)

// log is the package-level logger, initialized in Initialize
var log *logger.Logger

var (
	// Server config
	Port = env.Get("PORT", ":8082")

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
	IdempotencyKeyTTL = env.GetDuration("IDEMPOTENCY_KEY_TTL", 0)                      // 0 means use ChallengeExpiry
	AllowedPurposes   = env.GetStringSlice("ALLOWED_PURPOSES", []string{"login"}, ",") // Comma-separated list: "login,reset,bind,stepup"

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

	// SMS Provider config (HTTP API mode - recommended)
	SMSProvider   = env.Get("SMS_PROVIDER", "")     // Provider name (e.g., "aliyun", "tencent", "http")
	SMSAPIBaseURL = env.Get("SMS_API_BASE_URL", "") // HTTP API base URL for SMS provider
	SMSAPIKey     = env.Get("SMS_API_KEY", "")      // HTTP API key for SMS provider

	// DingTalk channel: Herald calls herald-dingtalk via HTTP (no DingTalk credentials in Herald)
	HeraldDingtalkAPIURL = env.Get("HERALD_DINGTALK_API_URL", "") // Base URL of herald-dingtalk service
	HeraldDingtalkAPIKey = env.Get("HERALD_DINGTALK_API_KEY", "") // Optional API key for herald-dingtalk

	// Service authentication (HMAC)
	HMACSecret   = env.Get("HMAC_SECRET", "")
	HMACKeysJSON = env.Get("HERALD_HMAC_KEYS", "") // JSON format: {"key-id-1":"secret-1","key-id-2":"secret-2"}
	ServiceName  = env.Get("SERVICE_NAME", "herald")

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
	AuditStorageType     = env.Get("AUDIT_STORAGE_TYPE", "") // "database", "file", "loki", or comma-separated list
	AuditDatabaseURL     = env.Get("AUDIT_DATABASE_URL", "")
	AuditTableName       = env.Get("AUDIT_TABLE_NAME", "audit_logs")
	AuditFilePath        = env.Get("AUDIT_FILE_PATH", "")
	AuditLokiURL         = env.Get("AUDIT_LOKI_URL", "")
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

	// Log configuration (excluding sensitive data)
	log.Info().
		Str("port", Port).
		Str("redis", maskSensitive(RedisAddr)).
		Int("redis_db", RedisDB).
		Str("log_level", LogLevel).
		Dur("challenge_expiry", ChallengeExpiry).
		Int("max_attempts", MaxAttempts).
		Int("code_length", CodeLength).
		Bool("session_storage", SessionStorageEnabled).
		Msg("Configuration initialized")

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

		// Set default key ID to first key if available
		for keyID := range hmacKeysMap {
			hmacDefaultKeyID = keyID
			break
		}
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
