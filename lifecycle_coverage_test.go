package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/soulteary/herald/internal/config"
)

func availableTestPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	return port
}

func TestApplicationLifecycleAndAdminListener(t *testing.T) {
	for _, admin := range []bool{false, true} {
		t.Run(fmt.Sprint("admin=", admin), func(t *testing.T) {
			mr := miniredis.RunT(t)
			oldPort, oldRedis, oldPassword := config.Port, config.RedisAddr, config.RedisPassword
			oldEnv, oldAPI, oldMode := config.Env, config.APIKey, config.RequestAuthMode
			oldAudit, oldOTLP, oldTest := config.AuditEnabled, config.OTLPEnabled, config.TestMode
			oldTestKey, oldTestAddr := config.TestAPIKey, config.TestListenerAddr
			oldCert, oldKey, oldCA, oldClientMode := config.TLSCertFile, config.TLSKeyFile, config.TLSCACertFile, config.ClientCertMode
			oldIdem, oldPepper := config.IdempotencySecret, config.PIIPepper
			oldLog := log
			t.Cleanup(func() {
				config.Port, config.RedisAddr, config.RedisPassword = oldPort, oldRedis, oldPassword
				config.Env, config.APIKey, config.RequestAuthMode = oldEnv, oldAPI, oldMode
				config.AuditEnabled, config.OTLPEnabled, config.TestMode = oldAudit, oldOTLP, oldTest
				config.TestAPIKey, config.TestListenerAddr = oldTestKey, oldTestAddr
				config.TLSCertFile, config.TLSKeyFile, config.TLSCACertFile, config.ClientCertMode = oldCert, oldKey, oldCA, oldClientMode
				config.IdempotencySecret, config.PIIPepper = oldIdem, oldPepper
				log = oldLog
			})
			t.Setenv("LOG_LEVEL", "error")
			config.Port, config.RedisAddr, config.RedisPassword = availableTestPort(t), mr.Addr(), ""
			config.Env, config.APIKey, config.RequestAuthMode = config.EnvDevelopment, strings.Repeat("a", 32), "api_key"
			config.AuditEnabled, config.OTLPEnabled, config.TestMode = false, false, admin
			config.TLSCertFile, config.TLSKeyFile, config.TLSCACertFile, config.ClientCertMode = "", "", "", "off"
			config.IdempotencySecret, config.PIIPepper = strings.Repeat("i", 32), strings.Repeat("p", 32)
			config.TestAPIKey, config.TestListenerAddr = strings.Repeat("t", 32), "127.0.0.1:"+availableTestPort(t)
			if admin {
				config.Env = config.EnvTest
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- runWithContext(ctx) }()
			client := &http.Client{Timeout: 100 * time.Millisecond}
			urls := []string{"http://127.0.0.1:" + config.Port + "/livez"}
			if admin {
				urls = append(urls, "http://"+config.TestListenerAddr+"/livez")
			}
			for _, endpoint := range urls {
				deadline := time.Now().Add(5 * time.Second)
				for {
					select {
					case err := <-done:
						t.Fatalf("startup failed: %v", err)
					default:
					}
					resp, err := client.Get(endpoint)
					if err == nil {
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
						if resp.StatusCode == 200 {
							break
						}
					}
					if time.Now().After(deadline) {
						t.Fatal("listener never became live")
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("application failed to shut down")
			}
		})
	}
}
