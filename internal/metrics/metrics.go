package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// herald_otp_challenges_total{channel,purpose,result}
	OTPChallengesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "herald_otp_challenges_total",
			Help: "Total number of OTP challenges created",
		},
		[]string{"channel", "purpose", "result"},
	)

	// herald_otp_sends_total{channel,provider,result}
	OTPSendsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "herald_otp_sends_total",
			Help: "Total number of OTP sends via providers",
		},
		[]string{"channel", "provider", "result"},
	)

	// herald_otp_verifications_total{result,reason}
	OTPVerificationsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "herald_otp_verifications_total",
			Help: "Total number of OTP verifications",
		},
		[]string{"result", "reason"},
	)

	// herald_otp_send_duration_seconds{provider}
	OTPSendDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "herald_otp_send_duration_seconds",
			Help:    "Duration of OTP send operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"provider"},
	)

	// herald_rate_limit_hits_total{scope}
	RateLimitHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "herald_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"scope"}, // scope: user, ip, destination, resend_cooldown
	)

	// herald_redis_latency_seconds
	RedisLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "herald_redis_latency_seconds",
			Help:    "Redis operation latency in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"operation"}, // operation: get, set, del, exists
	)
)

// RecordChallengeCreated records a challenge creation event
func RecordChallengeCreated(channel, purpose, result string) {
	OTPChallengesTotal.WithLabelValues(channel, purpose, result).Inc()
}

// RecordOTPSend records an OTP send event
func RecordOTPSend(channel, provider, result string, duration time.Duration) {
	OTPSendsTotal.WithLabelValues(channel, provider, result).Inc()
	OTPSendDurationSeconds.WithLabelValues(provider).Observe(duration.Seconds())
}

// RecordVerification records a verification event
func RecordVerification(result, reason string) {
	OTPVerificationsTotal.WithLabelValues(result, reason).Inc()
}

// RecordRateLimitHit records a rate limit hit
func RecordRateLimitHit(scope string) {
	RateLimitHitsTotal.WithLabelValues(scope).Inc()
}

// RecordRedisLatency records Redis operation latency
func RecordRedisLatency(operation string, duration time.Duration) {
	RedisLatencySeconds.WithLabelValues(operation).Observe(duration.Seconds())
}
