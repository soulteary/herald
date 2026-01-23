package config

import (
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/soulteary/cli-kit/env"
	"github.com/soulteary/cli-kit/validator"
)

var (
	// Version is set at build time
	Version = "dev"

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

	// SMS Provider config (example: Aliyun)
	SMSProvider        = env.Get("SMS_PROVIDER", "") // "aliyun", "tencent", etc.
	AliyunAccessKey    = env.Get("ALIYUN_ACCESS_KEY", "")
	AliyunSecretKey    = env.Get("ALIYUN_SECRET_KEY", "")
	AliyunSignName     = env.Get("ALIYUN_SIGN_NAME", "")
	AliyunTemplateCode = env.Get("ALIYUN_TEMPLATE_CODE", "")

	// Service authentication (HMAC)
	HMACSecret  = env.Get("HMAC_SECRET", "")
	ServiceName = env.Get("SERVICE_NAME", "herald")

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

	// Template config
	TemplateDir = env.Get("TEMPLATE_DIR", "") // Optional: path to template directory
)

// Initialize validates and initializes configuration
func Initialize() error {
	// Validate required configs
	if RedisAddr == "" {
		logrus.Warn("REDIS_ADDR is not set, using default: localhost:6379")
	} else {
		// Validate Redis address format using cli-kit validator
		if _, _, err := validator.ValidateHostPort(RedisAddr); err != nil {
			logrus.Warnf("Invalid REDIS_ADDR format: %s (%v), using default: localhost:6379", RedisAddr, err)
			RedisAddr = "localhost:6379"
		}
	}

	if APIKey == "" && HMACSecret == "" {
		logrus.Warn("Neither API_KEY nor HMAC_SECRET is set, service-to-service authentication will be disabled")
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
	logrus.Infof("Configuration initialized:")
	logrus.Infof("  Port: %s", Port)
	logrus.Infof("  Redis: %s (DB: %d)", maskSensitive(RedisAddr), RedisDB)
	logrus.Infof("  Log Level: %s", LogLevel)
	logrus.Infof("  Challenge Expiry: %v", ChallengeExpiry)
	logrus.Infof("  Max Attempts: %d", MaxAttempts)
	logrus.Infof("  Code Length: %d", CodeLength)
	if SessionStorageEnabled {
		logrus.Infof("  Session Storage: enabled (TTL: %v, Prefix: %s)", SessionDefaultTTL, SessionKeyPrefix)
	} else {
		logrus.Infof("  Session Storage: disabled")
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
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}
