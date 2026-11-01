package config

import (
	"strings"
	"testing"
	"time"
)

func TestStrictNumericBoundsReportSpecificSetting(t *testing.T) {
	// Each setting is restored before the next case, so an unrelated validation
	// error cannot make a broken boundary check look successful.
	for _, tc := range []struct {
		name  string
		value *int
		want  string
	}{
		{"body", &MaxBodyBytes, "HERALD_MAX_BODY_BYTES"},
		{"attempts", &MaxAttempts, "MAX_ATTEMPTS"},
		{"audit queue", &AuditWriterQueueSize, "audit writer queue"},
		{"audit workers", &AuditWriterWorkers, "audit writer queue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := *tc.value
			defer func() { *tc.value = old }()
			*tc.value = -1
			if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s error, got %v", tc.want, err)
			}
		})
	}
}

func TestStrictDurationBounds(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   *time.Duration
		invalid time.Duration
		want    string
	}{
		{"read timeout", &ReadTimeout, 0, "HERALD_READ_TIMEOUT"},
		{"challenge expiry", &ChallengeExpiry, 0, "CHALLENGE_EXPIRY"},
		{"resend cooldown", &ResendCooldown, -time.Second, "RESEND_COOLDOWN"},
		{"HMAC window", &HMACMaxDrift, 0, "HMAC_MAX_DRIFT"},
		{"audit TTL", &AuditTTL, 0, "AUDIT_TTL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, oldAudit := *tc.value, AuditEnabled
			defer func() { *tc.value, AuditEnabled = old, oldAudit }()
			*tc.value = tc.invalid
			AuditEnabled = true
			if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %s error, got %v", tc.want, err)
			}
		})
	}
}
