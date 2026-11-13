package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestSecurityAndAuditCountersRecordOneEvent(t *testing.T) {
	cases := []struct {
		name   string
		record func()
		value  func() float64
	}{
		{"idempotency", func() { RecordIdempotency("delivery_unknown") }, func() float64 { return counterValue(t, idempotencyTotal.WithLabelValues("delivery_unknown")) }},
		{"timeout", func() { RecordProviderTimeout("email") }, func() float64 { return counterValue(t, providerTimeoutTotal.WithLabelValues("email")) }},
		{"contention", func() { RecordVerificationContention("contended") }, func() float64 { return counterValue(t, verificationContention.WithLabelValues("contended")) }},
		{"nonce", RecordNonceReplay, func() float64 { return counterValue(t, nonceReplayTotal) }},
		{"failed attempt lock", RecordFailedAttemptLock, func() float64 { return counterValue(t, failedAttemptLockTotal) }},
		{"state", func() { RecordChallengeState("revoked") }, func() float64 { return counterValue(t, challengeStateTotal.WithLabelValues("revoked")) }},
		{"audit rejected", RecordAuditDropped, func() float64 { return counterValue(t, auditDroppedTotal) }},
		{"audit retry", RecordAuditWriteFailure, func() float64 { return counterValue(t, auditWriteFailureTotal) }},
		{"audit post-action", func() { RecordAuditPostActionFailure("verify") }, func() float64 {
			return counterValue(t, auditPostActionFailure.WithLabelValues("verify"))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.value()
			tc.record()
			if after := tc.value(); after != before+1 {
				t.Fatalf("counter delta=%v", after-before)
			}
		})
	}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := c.Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}
