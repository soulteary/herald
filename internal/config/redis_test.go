package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withRedisTransport(t *testing.T) {
	t.Helper()
	oldUser, oldEnabled, oldCA, oldName := RedisUsername, RedisTLSEnabled, RedisTLSCAFile, RedisTLSServerName
	t.Cleanup(func() {
		RedisUsername, RedisTLSEnabled, RedisTLSCAFile, RedisTLSServerName = oldUser, oldEnabled, oldCA, oldName
	})
	RedisUsername, RedisTLSEnabled, RedisTLSCAFile, RedisTLSServerName = "", false, "", ""
}

func TestRedisTLSConfig(t *testing.T) {
	withRedisTransport(t)

	if cfg, err := RedisTLSConfig(); err != nil || cfg != nil {
		t.Fatalf("TLS must be off by default: cfg=%v err=%v", cfg, err)
	}

	RedisTLSEnabled = true
	RedisTLSServerName = "redis.internal"
	cfg, err := RedisTLSConfig()
	if err != nil || cfg == nil {
		t.Fatalf("enabled TLS: cfg=%v err=%v", cfg, err)
	}
	if cfg.InsecureSkipVerify || cfg.RootCAs != nil || cfg.ServerName != "redis.internal" || cfg.MinVersion < 0x0303 {
		t.Fatalf("unexpected TLS config: skip=%v roots=%v name=%q min=%#x", cfg.InsecureSkipVerify, cfg.RootCAs, cfg.ServerName, cfg.MinVersion)
	}

	RedisTLSCAFile = filepath.Join(t.TempDir(), "missing.pem")
	if _, err := RedisTLSConfig(); err == nil || !strings.Contains(err.Error(), "REDIS_TLS_CA_FILE") {
		t.Fatalf("missing CA file accepted: %v", err)
	}

	RedisTLSCAFile = filepath.Join(t.TempDir(), "empty.pem")
	if err := os.WriteFile(RedisTLSCAFile, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RedisTLSConfig(); err == nil || !strings.Contains(err.Error(), "REDIS_TLS_CA_FILE") {
		t.Fatalf("CA file without certificates accepted: %v", err)
	}
}

// TestValidate_RedisTransportSettingsThatWouldBeIgnored proves production
// rejects Redis transport settings that would silently not apply, and that the
// defaults and a complete configuration pass.
func TestValidate_RedisTransportSettingsThatWouldBeIgnored(t *testing.T) {
	defer withProdEnv(t)()
	withRedisTransport(t)

	if err := Validate(); err != nil {
		t.Fatalf("default Redis transport rejected: %v", err)
	}

	RedisTLSCAFile = "/etc/herald/redis-ca.pem"
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_TLS_ENABLED") {
		t.Fatalf("CA file without REDIS_TLS_ENABLED accepted: %v", err)
	}
	RedisTLSEnabled = true
	RedisUsername = "herald"
	if err := Validate(); err != nil {
		t.Fatalf("TLS with ACL user rejected: %v", err)
	}

	RedisPassword = ""
	RiskAckPasswordlessRedis = true
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_USERNAME") {
		t.Fatalf("username without password accepted: %v", err)
	}

	Env = EnvDevelopment
	if err := Validate(); err != nil {
		t.Fatalf("development must only warn: %v", err)
	}
}
