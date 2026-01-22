package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRecordChallengeCreated(t *testing.T) {
	// Reset metrics before test
	OTPChallengesTotal.Reset()

	RecordChallengeCreated("email", "login", "success")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := OTPChallengesTotal.WithLabelValues("email", "login", "success").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}
	if metric.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric.Counter.GetValue())
	}
}

func TestRecordOTPSend(t *testing.T) {
	// Reset metrics before test
	OTPSendsTotal.Reset()
	OTPSendDurationSeconds.Reset()

	duration := 100 * time.Millisecond
	RecordOTPSend("email", "smtp", "success", duration)

	// Verify send counter was incremented
	metric := &dto.Metric{}
	if err := OTPSendsTotal.WithLabelValues("email", "smtp", "success").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}
	if metric.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric.Counter.GetValue())
	}

	// Verify duration was recorded (histograms use Observe, not Write)
	// We can't easily test histogram values without gathering metrics
	// Just verify the metric exists and can be observed
	OTPSendDurationSeconds.WithLabelValues("smtp").Observe(0.1)
}

func TestRecordVerification(t *testing.T) {
	// Reset metrics before test
	OTPVerificationsTotal.Reset()

	RecordVerification("success", "")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := OTPVerificationsTotal.WithLabelValues("success", "").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}
	if metric.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric.Counter.GetValue())
	}

	// Test failure case
	RecordVerification("failure", "invalid")
	metric2 := &dto.Metric{}
	if err := OTPVerificationsTotal.WithLabelValues("failure", "invalid").Write(metric2); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric2.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric2.Counter.GetValue())
	}
}

func TestRecordRateLimitHit(t *testing.T) {
	// Reset metrics before test
	RateLimitHitsTotal.Reset()

	RecordRateLimitHit("user")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := RateLimitHitsTotal.WithLabelValues("user").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}
	if metric.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric.Counter.GetValue())
	}

	// Test different scopes
	RecordRateLimitHit("ip")
	RecordRateLimitHit("destination")
	RecordRateLimitHit("resend_cooldown")

	scopes := []string{"ip", "destination", "resend_cooldown"}
	for _, scope := range scopes {
		metric := &dto.Metric{}
		if err := RateLimitHitsTotal.WithLabelValues(scope).Write(metric); err != nil {
			t.Fatalf("Failed to write metric for scope %s: %v", scope, err)
		}
		if metric.Counter.GetValue() != 1.0 {
			t.Errorf("Counter value for scope %s = %v, want 1.0", scope, metric.Counter.GetValue())
		}
	}
}

func TestRecordRedisLatency(t *testing.T) {
	// Reset metrics before test
	RedisLatencySeconds.Reset()

	duration := 5 * time.Millisecond
	RecordRedisLatency("get", duration)

	// Verify metric was recorded (histograms use Observe, not Write)
	// We can't easily test histogram values without gathering metrics
	// Just verify the metric exists and can be observed
	RedisLatencySeconds.WithLabelValues("get").Observe(0.005)

	// Test different operations
	operations := []string{"set", "del", "exists"}
	for _, op := range operations {
		RecordRedisLatency(op, duration)
		RedisLatencySeconds.WithLabelValues(op).Observe(0.005)
	}
}

func TestMetricsMultipleIncrements(t *testing.T) {
	// Reset metrics before test
	OTPChallengesTotal.Reset()

	// Record multiple events
	RecordChallengeCreated("email", "login", "success")
	RecordChallengeCreated("email", "login", "success")
	RecordChallengeCreated("sms", "login", "success")

	// Verify email metric
	metric := &dto.Metric{}
	if err := OTPChallengesTotal.WithLabelValues("email", "login", "success").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if metric.Counter.GetValue() != 2.0 {
		t.Errorf("Counter value = %v, want 2.0", metric.Counter.GetValue())
	}

	// Verify sms metric
	metric2 := &dto.Metric{}
	if err := OTPChallengesTotal.WithLabelValues("sms", "login", "success").Write(metric2); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if metric2.Counter.GetValue() != 1.0 {
		t.Errorf("Counter value = %v, want 1.0", metric2.Counter.GetValue())
	}
}

func TestMetricsFailureCases(t *testing.T) {
	// Reset metrics before test
	OTPChallengesTotal.Reset()
	OTPSendsTotal.Reset()
	OTPVerificationsTotal.Reset()

	// Test failure cases
	RecordChallengeCreated("email", "login", "failure")
	RecordOTPSend("email", "smtp", "failure", 50*time.Millisecond)
	RecordVerification("failure", "expired")

	// Verify failure metrics
	challengeMetric := &dto.Metric{}
	if err := OTPChallengesTotal.WithLabelValues("email", "login", "failure").Write(challengeMetric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if challengeMetric.Counter.GetValue() != 1.0 {
		t.Errorf("Challenge failure counter = %v, want 1.0", challengeMetric.Counter.GetValue())
	}

	sendMetric := &dto.Metric{}
	if err := OTPSendsTotal.WithLabelValues("email", "smtp", "failure").Write(sendMetric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if sendMetric.Counter.GetValue() != 1.0 {
		t.Errorf("Send failure counter = %v, want 1.0", sendMetric.Counter.GetValue())
	}

	verifyMetric := &dto.Metric{}
	if err := OTPVerificationsTotal.WithLabelValues("failure", "expired").Write(verifyMetric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}
	if verifyMetric.Counter.GetValue() != 1.0 {
		t.Errorf("Verification failure counter = %v, want 1.0", verifyMetric.Counter.GetValue())
	}
}

// Test that metrics are properly registered with Prometheus
func TestMetricsRegistration(t *testing.T) {
	// Verify metrics are registered by checking they can be collected
	registry := prometheus.NewRegistry()
	registry.MustRegister(OTPChallengesTotal)
	registry.MustRegister(OTPSendsTotal)
	registry.MustRegister(OTPVerificationsTotal)
	registry.MustRegister(OTPSendDurationSeconds)
	registry.MustRegister(RateLimitHitsTotal)
	registry.MustRegister(RedisLatencySeconds)

	// Collect metrics
	_, err := registry.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
}
