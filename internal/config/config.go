package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	// Version is set at build time
	Version = "dev"

	// Server config
	Port = getEnv("PORT", ":8082")

	// Redis config
	RedisAddr     = getEnv("REDIS_ADDR", "localhost:6379")
	RedisPassword = getEnv("REDIS_PASSWORD", "")
	RedisDB       = getEnvInt("REDIS_DB", 0)

	// Logging
	LogLevel = getEnv("LOG_LEVEL", "info")

	// API Key for service-to-service authentication
	APIKey = getEnv("API_KEY", "")

	// Challenge config
	ChallengeExpiry   = getEnvDuration("CHALLENGE_EXPIRY", 5*time.Minute)
	MaxAttempts       = getEnvInt("MAX_ATTEMPTS", 5)
	ResendCooldown    = getEnvDuration("RESEND_COOLDOWN", 60*time.Second)
	CodeLength        = getEnvInt("CODE_LENGTH", 6)
	LockoutDuration   = getEnvDuration("LOCKOUT_DURATION", 10*time.Minute)
	IdempotencyKeyTTL = getEnvDuration("IDEMPOTENCY_KEY_TTL", 0) // 0 means use ChallengeExpiry
	AllowedPurposes   = getEnv("ALLOWED_PURPOSES", "login")      // Comma-separated list: "login,reset,bind,stepup"

	// Rate limiting config
	RateLimitPerUser        = getEnvInt("RATE_LIMIT_PER_USER", 10)        // per hour
	RateLimitPerIP          = getEnvInt("RATE_LIMIT_PER_IP", 5)           // per minute
	RateLimitPerDestination = getEnvInt("RATE_LIMIT_PER_DESTINATION", 10) // per hour

	// Provider config
	SMTPHost              = getEnv("SMTP_HOST", "")
	SMTPPort              = getEnvInt("SMTP_PORT", 587)
	SMTPUser              = getEnv("SMTP_USER", "")
	SMTPPassword          = getEnv("SMTP_PASSWORD", "")
	SMTPFrom              = getEnv("SMTP_FROM", "")
	ProviderFailurePolicy = getEnv("PROVIDER_FAILURE_POLICY", "soft") // "strict" | "soft"

	// SMS Provider config (example: Aliyun)
	SMSProvider        = getEnv("SMS_PROVIDER", "") // "aliyun", "tencent", etc.
	AliyunAccessKey    = getEnv("ALIYUN_ACCESS_KEY", "")
	AliyunSecretKey    = getEnv("ALIYUN_SECRET_KEY", "")
	AliyunSignName     = getEnv("ALIYUN_SIGN_NAME", "")
	AliyunTemplateCode = getEnv("ALIYUN_TEMPLATE_CODE", "")

	// Service authentication (HMAC)
	HMACSecret  = getEnv("HMAC_SECRET", "")
	ServiceName = getEnv("SERVICE_NAME", "herald")

	// TLS/mTLS config
	TLSCertFile     = getEnv("TLS_CERT_FILE", "")
	TLSKeyFile      = getEnv("TLS_KEY_FILE", "")
	TLSCACertFile   = getEnv("TLS_CA_CERT_FILE", "")   // For mTLS (client certificate verification)
	TLSClientCAFile = getEnv("TLS_CLIENT_CA_FILE", "") // Alias for TLS_CA_CERT_FILE
	TestMode        = getEnvBool("HERALD_TEST_MODE", false)

	// Session storage config
	SessionStorageEnabled = getEnvBool("HERALD_SESSION_STORAGE_ENABLED", false)
	SessionDefaultTTL     = getEnvDuration("HERALD_SESSION_DEFAULT_TTL", 1*time.Hour)
	SessionKeyPrefix      = getEnv("HERALD_SESSION_KEY_PREFIX", "session:")

	// Audit logging config
	AuditEnabled         = getEnvBool("AUDIT_ENABLED", true)
	AuditMaskDestination = getEnvBool("AUDIT_MASK_DESTINATION", false)
	AuditTTL             = getEnvDuration("AUDIT_TTL", 7*24*time.Hour) // 7 days default

	// Template config
	TemplateDir = getEnv("TEMPLATE_DIR", "") // Optional: path to template directory
)

// Initialize validates and initializes configuration
func Initialize() error {
	// Validate required configs
	if RedisAddr == "" {
		logrus.Warn("REDIS_ADDR is not set, using default: localhost:6379")
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

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
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
