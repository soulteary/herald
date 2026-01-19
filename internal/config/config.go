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
	ChallengeExpiry = getEnvDuration("CHALLENGE_EXPIRY", 5*time.Minute)
	MaxAttempts     = getEnvInt("MAX_ATTEMPTS", 5)
	ResendCooldown  = getEnvDuration("RESEND_COOLDOWN", 60*time.Second)
	CodeLength      = getEnvInt("CODE_LENGTH", 6)
	LockoutDuration = getEnvDuration("LOCKOUT_DURATION", 10*time.Minute)

	// Rate limiting config
	RateLimitPerUser        = getEnvInt("RATE_LIMIT_PER_USER", 10)        // per hour
	RateLimitPerIP          = getEnvInt("RATE_LIMIT_PER_IP", 5)           // per minute
	RateLimitPerDestination = getEnvInt("RATE_LIMIT_PER_DESTINATION", 10) // per hour

	// Provider config
	SMTPHost     = getEnv("SMTP_HOST", "")
	SMTPPort     = getEnvInt("SMTP_PORT", 587)
	SMTPUser     = getEnv("SMTP_USER", "")
	SMTPPassword = getEnv("SMTP_PASSWORD", "")
	SMTPFrom     = getEnv("SMTP_FROM", "")

	// SMS Provider config (example: Aliyun)
	SMSProvider        = getEnv("SMS_PROVIDER", "") // "aliyun", "tencent", etc.
	AliyunAccessKey    = getEnv("ALIYUN_ACCESS_KEY", "")
	AliyunSecretKey    = getEnv("ALIYUN_SECRET_KEY", "")
	AliyunSignName     = getEnv("ALIYUN_SIGN_NAME", "")
	AliyunTemplateCode = getEnv("ALIYUN_TEMPLATE_CODE", "")

	// Service authentication (HMAC)
	HMACSecret  = getEnv("HMAC_SECRET", "")
	ServiceName = getEnv("SERVICE_NAME", "herald")
)

// Initialize validates and initializes configuration
func Initialize() error {
	// Validate required configs
	if RedisAddr == "" {
		logrus.Warn("REDIS_ADDR is not set, using default: localhost:6379")
	}

	if APIKey == "" {
		logrus.Warn("API_KEY is not set, service-to-service authentication will be disabled")
	}

	// Log configuration (excluding sensitive data)
	logrus.Infof("Configuration initialized:")
	logrus.Infof("  Port: %s", Port)
	logrus.Infof("  Redis: %s (DB: %d)", maskSensitive(RedisAddr), RedisDB)
	logrus.Infof("  Log Level: %s", LogLevel)
	logrus.Infof("  Challenge Expiry: %v", ChallengeExpiry)
	logrus.Infof("  Max Attempts: %d", MaxAttempts)
	logrus.Infof("  Code Length: %d", CodeLength)

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

func maskSensitive(s string) string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 8 {
		return "***"
	}
	return s[:4] + "***" + s[len(s)-4:]
}
