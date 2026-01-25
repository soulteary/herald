package metrics

import (
	"time"

	metrics "github.com/soulteary/metrics-kit"
)

var (
	// Registry is the Prometheus registry for Herald metrics
	Registry *metrics.Registry

	// OTP holds OTP-related metrics
	OTP *metrics.OTPMetrics

	// RateLimit holds rate limiting metrics
	RateLimit *metrics.RateLimitMetrics

	// Redis holds Redis operation metrics
	Redis *metrics.RedisMetrics
)

func init() {
	Init()
}

// Init initializes all Herald metrics using metrics-kit
func Init() {
	Registry = metrics.NewRegistry("herald")
	cm := metrics.NewCommonMetrics(Registry)

	OTP = cm.NewOTPMetrics()
	RateLimit = cm.NewRateLimitMetrics()
	Redis = cm.NewRedisMetrics()
}

// RecordChallengeCreated records a challenge creation event
func RecordChallengeCreated(channel, purpose, result string) {
	OTP.RecordChallengeCreated(channel, purpose, result)
}

// RecordOTPSend records an OTP send event
func RecordOTPSend(channel, provider, result string, duration time.Duration) {
	OTP.RecordSend(channel, provider, result, duration)
}

// RecordVerification records a verification event
func RecordVerification(result, reason string) {
	OTP.RecordVerification(result, reason)
}

// RecordRateLimitHit records a rate limit hit
func RecordRateLimitHit(scope string) {
	RateLimit.RecordHit(scope)
}

// RecordRedisLatency records Redis operation latency
func RecordRedisLatency(operation string, duration time.Duration) {
	Redis.RecordSuccess(operation, duration)
}

// RecordRedisSuccess records a successful Redis operation
func RecordRedisSuccess(operation string, duration time.Duration) {
	Redis.RecordSuccess(operation, duration)
}

// RecordRedisFailure records a failed Redis operation
func RecordRedisFailure(operation string, duration time.Duration) {
	Redis.RecordFailure(operation, duration)
}
