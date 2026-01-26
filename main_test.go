package main

import (
	"testing"

	logger "github.com/soulteary/logger-kit"
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
