package main

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected logrus.Level
	}{
		{
			name:     "trace level",
			level:    "trace",
			expected: logrus.TraceLevel,
		},
		{
			name:     "debug level",
			level:    "debug",
			expected: logrus.DebugLevel,
		},
		{
			name:     "info level",
			level:    "info",
			expected: logrus.InfoLevel,
		},
		{
			name:     "warn level",
			level:    "warn",
			expected: logrus.WarnLevel,
		},
		{
			name:     "warning level",
			level:    "warning",
			expected: logrus.WarnLevel,
		},
		{
			name:     "error level",
			level:    "error",
			expected: logrus.ErrorLevel,
		},
		{
			name:     "fatal level",
			level:    "fatal",
			expected: logrus.FatalLevel,
		},
		{
			name:     "panic level",
			level:    "panic",
			expected: logrus.PanicLevel,
		},
		{
			name:     "uppercase level",
			level:    "INFO",
			expected: logrus.InfoLevel,
		},
		{
			name:     "mixed case level",
			level:    "DeBuG",
			expected: logrus.DebugLevel,
		},
		{
			name:     "invalid level defaults to info",
			level:    "invalid",
			expected: logrus.InfoLevel,
		},
		{
			name:     "empty level defaults to info",
			level:    "",
			expected: logrus.InfoLevel,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original level
			originalLevel := logrus.GetLevel()

			// Test setLogLevel
			setLogLevel(tt.level)

			// Verify level was set correctly
			if logrus.GetLevel() != tt.expected {
				t.Errorf("setLogLevel(%q) = %v, want %v", tt.level, logrus.GetLevel(), tt.expected)
			}

			// Restore original level
			logrus.SetLevel(originalLevel)
		})
	}
}
