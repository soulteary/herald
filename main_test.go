package main

import (
	"net/http"
	"testing"

	"github.com/soulteary/herald/internal/config"

	logger "github.com/soulteary/logger-kit/v2"
)

func TestLoggerKitParseLevelFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected logger.Level
	}{
		{
			name:     "trace level",
			level:    "trace",
			expected: logger.TraceLevel,
		},
		{
			name:     "debug level",
			level:    "debug",
			expected: logger.DebugLevel,
		},
		{
			name:     "info level",
			level:    "info",
			expected: logger.InfoLevel,
		},
		{
			name:     "warn level",
			level:    "warn",
			expected: logger.WarnLevel,
		},
		{
			name:     "error level",
			level:    "error",
			expected: logger.ErrorLevel,
		},
		{
			name:     "fatal level",
			level:    "fatal",
			expected: logger.FatalLevel,
		},
		{
			name:     "panic level",
			level:    "panic",
			expected: logger.PanicLevel,
		},
		{
			name:     "invalid level defaults to info",
			level:    "invalid",
			expected: logger.InfoLevel,
		},
		{
			name:     "empty level defaults to info",
			level:    "",
			expected: logger.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test ParseLevel from logger-kit
			parsed, err := logger.ParseLevel(tt.level)

			// For invalid/empty levels, ParseLevel returns an error or NoLevel
			// Use default level (InfoLevel) when there's an error or NoLevel
			if err != nil || parsed == logger.NoLevel {
				parsed = logger.InfoLevel
			}

			// Verify level was parsed correctly
			if parsed != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.level, parsed, tt.expected)
			}
		})
	}
}

func TestNewHealthcheckClientMatchesListenerTLSMode(t *testing.T) {
	origCert, origKey := config.TLSCertFile, config.TLSKeyFile
	origCA, origName := config.HealthcheckTLSCAFile, config.HealthcheckTLSServerName
	origClientCert, origClientKey := config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile
	defer func() {
		config.TLSCertFile, config.TLSKeyFile = origCert, origKey
		config.HealthcheckTLSCAFile, config.HealthcheckTLSServerName = origCA, origName
		config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile = origClientCert, origClientKey
	}()

	config.TLSCertFile, config.TLSKeyFile = "", ""
	scheme, client, err := newHealthcheckClient()
	if err != nil || scheme != "http" {
		t.Fatalf("plaintext healthcheck = %q, %v", scheme, err)
	}
	if client.Transport != nil {
		t.Fatal("plaintext healthcheck should use the default transport")
	}

	config.TLSCertFile, config.TLSKeyFile = "server.crt", "server.key"
	config.HealthcheckTLSServerName = "herald.internal"
	scheme, client, err = newHealthcheckClient()
	if err != nil || scheme != "https" {
		t.Fatalf("TLS healthcheck = %q, %v", scheme, err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig.ServerName != "herald.internal" {
		t.Fatal("TLS healthcheck did not configure verified server name")
	}
}
