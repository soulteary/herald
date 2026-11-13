package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	metrics "github.com/soulteary/metrics-kit/v3"
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

	// Low-cardinality security/reliability counters (Phase 3/4/6). All labels
	// are drawn from a small fixed allowlist of values to avoid cardinality
	// blowups and to never carry PII.
	idempotencyTotal       *prometheus.CounterVec // label: result
	providerTimeoutTotal   *prometheus.CounterVec // label: channel
	verificationContention *prometheus.CounterVec // label: result
	nonceReplayTotal       prometheus.Counter
	challengeStateTotal    *prometheus.CounterVec // label: state
	auditDroppedTotal      prometheus.Counter
	auditWriteFailureTotal prometheus.Counter
	auditQuarantinedTotal  prometheus.Counter
	auditPostActionFailure *prometheus.CounterVec // label: operation
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

	idempotencyTotal = Registry.Counter("idempotency_total").
		Help("Idempotency outcomes by result").Labels("result").BuildVec()
	providerTimeoutTotal = Registry.Counter("provider_timeout_total").
		Help("Provider send timeouts by channel").Labels("channel").BuildVec()
	verificationContention = Registry.Counter("verification_contention_total").
		Help("Verification lock contention outcomes").Labels("result").BuildVec()
	nonceReplayTotal = Registry.Counter("nonce_replay_total").
		Help("Rejected replayed HMAC nonces").Build()
	challengeStateTotal = Registry.Counter("challenge_state_total").
		Help("Challenge state transitions").Labels("state").BuildVec()
	auditDroppedTotal = Registry.Counter("audit_dropped_total").
		Help("Audit records rejected before durable acceptance").Build()
	auditWriteFailureTotal = Registry.Counter("audit_write_failure_total").Help("Audit delivery or cleanup failures; undelivered records retained for retry").Build()
	auditQuarantinedTotal = Registry.Counter("audit_quarantined_total").
		Help("Audit spool entries set aside because they could not be read or decoded; never delivered").Build()
	auditPostActionFailure = Registry.Counter("audit_post_action_failure_total").
		Help("Audit records lost after the action had completed; the request still succeeded").Labels("operation").BuildVec()
}

// RecordIdempotency records an idempotency outcome. result is one of:
// replay|conflict|in_flight|backend_error.
func RecordIdempotency(result string) {
	if idempotencyTotal != nil {
		idempotencyTotal.WithLabelValues(result).Inc()
	}
}

// RecordProviderTimeout records a provider send timeout for a channel.
func RecordProviderTimeout(channel string) {
	if providerTimeoutTotal != nil {
		providerTimeoutTotal.WithLabelValues(channel).Inc()
	}
}

// RecordVerificationContention records a verification lock-contention outcome
// (result: contended|acquired).
func RecordVerificationContention(result string) {
	if verificationContention != nil {
		verificationContention.WithLabelValues(result).Inc()
	}
}

// RecordNonceReplay records a rejected replayed HMAC nonce.
func RecordNonceReplay() {
	if nonceReplayTotal != nil {
		nonceReplayTotal.Inc()
	}
}

// RecordChallengeState records a challenge state transition
// (state: created|activated|superseded|revoked).
func RecordChallengeState(state string) {
	if challengeStateTotal != nil {
		challengeStateTotal.WithLabelValues(state).Inc()
	}
}

// RecordAuditDropped records rejection before durable local acceptance.
func RecordAuditDropped() {
	if auditDroppedTotal != nil {
		auditDroppedTotal.Inc()
	}
}

// RecordAuditWriteFailure records a failed delivery whose local copy is retained.
func RecordAuditWriteFailure() {
	if auditWriteFailureTotal != nil {
		auditWriteFailureTotal.Inc()
	}
}

// RecordAuditQuarantined records a spool entry that was set aside because it
// could not be read or decoded.
func RecordAuditQuarantined() {
	if auditQuarantinedTotal != nil {
		auditQuarantinedTotal.Inc()
	}
}

// RecordAuditPostActionFailure records an audit record that could not be
// accepted after its action had already completed, so the request was
// answered normally without it (operation: create|verify|revoke).
func RecordAuditPostActionFailure(operation string) {
	if auditPostActionFailure != nil {
		auditPostActionFailure.WithLabelValues(operation).Inc()
	}
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
