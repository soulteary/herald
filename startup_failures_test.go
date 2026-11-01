package main

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/soulteary/herald/internal/config"
)

func TestStartupReportsDependencyAndListenerFailures(t *testing.T) {
	for _, kind := range []string{"configuration", "redis", "provider", "listener"} {
		t.Run(kind, func(t *testing.T) {
			mr := miniredis.RunT(t)
			oldRedis, oldPass, oldEnv := config.RedisAddr, config.RedisPassword, config.Env
			oldMode, oldAPI, oldIdem, oldPepper := config.RequestAuthMode, config.APIKey, config.IdempotencySecret, config.PIIPepper
			oldAudit, oldOTLP, oldTest := config.AuditEnabled, config.OTLPEnabled, config.TestMode
			oldBody, oldURL, oldCA, oldCertMode, oldLog := config.MaxBodyBytes, config.HeraldSMTPAPIURL, config.TLSCACertFile, config.ClientCertMode, log
			defer func() {
				config.RedisAddr, config.RedisPassword, config.Env = oldRedis, oldPass, oldEnv
				config.RequestAuthMode, config.APIKey, config.IdempotencySecret, config.PIIPepper = oldMode, oldAPI, oldIdem, oldPepper
				config.AuditEnabled, config.OTLPEnabled, config.TestMode = oldAudit, oldOTLP, oldTest
				config.MaxBodyBytes, config.HeraldSMTPAPIURL, config.TLSCACertFile, config.ClientCertMode, log = oldBody, oldURL, oldCA, oldCertMode, oldLog
			}()
			t.Setenv("LOG_LEVEL", "error")
			config.RedisAddr, config.RedisPassword, config.Env = mr.Addr(), "", config.EnvDevelopment
			config.RequestAuthMode, config.APIKey = "api_key", strings.Repeat("a", 32)
			config.IdempotencySecret, config.PIIPepper = strings.Repeat("i", 32), strings.Repeat("p", 32)
			config.AuditEnabled, config.OTLPEnabled, config.TestMode = false, false, false
			want := ""
			switch kind {
			case "configuration":
				config.MaxBodyBytes = 0
				want = "HERALD_MAX_BODY_BYTES"
			case "redis":
				mr.Close()
				want = "connect"
			case "provider":
				config.HeraldSMTPAPIURL = "://bad"
				want = "initialize handlers"
			case "listener":
				config.TLSCACertFile = "ca.pem"
				config.ClientCertMode = "off"
				want = "CA"
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := runWithContext(ctx)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s: error=%v want %s", kind, err, want)
			}
		})
	}
}
