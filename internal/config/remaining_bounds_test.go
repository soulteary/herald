package config

import (
	"strings"
	"testing"
)

func TestStrictRejectsIncompleteHealthcheckIdentityAndEmptyProxy(t *testing.T) {
	cert, key, proxies, header := HealthcheckTLSClientCertFile, HealthcheckTLSClientKeyFile, TrustedProxies, TrustedProxyHeader
	defer func() {
		HealthcheckTLSClientCertFile, HealthcheckTLSClientKeyFile, TrustedProxies, TrustedProxyHeader = cert, key, proxies, header
	}()
	HealthcheckTLSClientCertFile, HealthcheckTLSClientKeyFile = "cert.pem", ""
	if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "must be set together") {
		t.Fatalf("half configured identity: %v", err)
	}
	HealthcheckTLSClientCertFile, HealthcheckTLSClientKeyFile = "", ""
	TrustedProxyHeader, TrustedProxies = "X-Forwarded-For", []string{" "}
	if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "empty entries") {
		t.Fatalf("empty proxy allowlist: %v", err)
	}
}

func TestStrictRequiresDurableSpoolConfiguration(t *testing.T) {
	enabled, dir, limit := AuditEnabled, AuditSpoolDir, AuditSpoolMaxBytes
	defer func() { AuditEnabled, AuditSpoolDir, AuditSpoolMaxBytes = enabled, dir, limit }()
	AuditEnabled, AuditSpoolDir = true, " "
	if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "AUDIT_SPOOL_DIR") {
		t.Fatalf("blank spool: %v", err)
	}
	AuditSpoolDir, AuditSpoolMaxBytes = t.TempDir(), 0
	if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "AUDIT_SPOOL_MAX_BYTES") {
		t.Fatalf("zero capacity spool: %v", err)
	}
}
