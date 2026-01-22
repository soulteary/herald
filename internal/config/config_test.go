package config

import (
	"os"
	"testing"
	"time"

	"github.com/soulteary/cli-kit/env"
)

func TestGetPort(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		expected string
	}{
		{
			name:     "port with colon prefix",
			port:     ":8082",
			expected: ":8082",
		},
		{
			name:     "port without colon prefix",
			port:     "8082",
			expected: ":8082",
		},
		{
			name:     "empty port",
			port:     "",
			expected: ":",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original value
			originalPort := Port
			defer func() {
				Port = originalPort
			}()

			Port = tt.port
			if got := GetPort(); got != tt.expected {
				t.Errorf("GetPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue string
		expected     string
	}{
		{
			name:         "env var set",
			envKey:       "TEST_ENV_VAR",
			envValue:     "test_value",
			defaultValue: "default",
			expected:     "test_value",
		},
		{
			name:         "env var not set",
			envKey:       "TEST_ENV_VAR_NOT_SET",
			envValue:     "",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				if err := os.Setenv(tt.envKey, tt.envValue); err != nil {
					t.Fatalf("failed to set env var: %v", err)
				}
				defer func() {
					if err := os.Unsetenv(tt.envKey); err != nil {
						t.Errorf("failed to unset env var: %v", err)
					}
				}()
			} else {
				if err := os.Unsetenv(tt.envKey); err != nil {
					t.Fatalf("failed to unset env var: %v", err)
				}
			}

			if got := env.Get(tt.envKey, tt.defaultValue); got != tt.expected {
				t.Errorf("env.Get() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
	}{
		{
			name:         "valid integer",
			envKey:       "TEST_INT_VAR",
			envValue:     "42",
			defaultValue: 0,
			expected:     42,
		},
		{
			name:         "invalid integer",
			envKey:       "TEST_INT_VAR_INVALID",
			envValue:     "not_a_number",
			defaultValue: 10,
			expected:     10,
		},
		{
			name:         "env var not set",
			envKey:       "TEST_INT_VAR_NOT_SET",
			envValue:     "",
			defaultValue: 5,
			expected:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				if err := os.Setenv(tt.envKey, tt.envValue); err != nil {
					t.Fatalf("failed to set env var: %v", err)
				}
				defer func() {
					if err := os.Unsetenv(tt.envKey); err != nil {
						t.Errorf("failed to unset env var: %v", err)
					}
				}()
			} else {
				if err := os.Unsetenv(tt.envKey); err != nil {
					t.Fatalf("failed to unset env var: %v", err)
				}
			}

			if got := env.GetInt(tt.envKey, tt.defaultValue); got != tt.expected {
				t.Errorf("env.GetInt() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue time.Duration
		expected     time.Duration
	}{
		{
			name:         "valid duration",
			envKey:       "TEST_DURATION_VAR",
			envValue:     "5m",
			defaultValue: time.Minute,
			expected:     5 * time.Minute,
		},
		{
			name:         "invalid duration",
			envKey:       "TEST_DURATION_VAR_INVALID",
			envValue:     "not_a_duration",
			defaultValue: time.Hour,
			expected:     time.Hour,
		},
		{
			name:         "env var not set",
			envKey:       "TEST_DURATION_VAR_NOT_SET",
			envValue:     "",
			defaultValue: 10 * time.Second,
			expected:     10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				if err := os.Setenv(tt.envKey, tt.envValue); err != nil {
					t.Fatalf("failed to set env var: %v", err)
				}
				defer func() {
					if err := os.Unsetenv(tt.envKey); err != nil {
						t.Errorf("failed to unset env var: %v", err)
					}
				}()
			} else {
				if err := os.Unsetenv(tt.envKey); err != nil {
					t.Fatalf("failed to unset env var: %v", err)
				}
			}

			if got := env.GetDuration(tt.envKey, tt.defaultValue); got != tt.expected {
				t.Errorf("env.GetDuration() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long string",
			input:    "this_is_a_long_string",
			expected: "this***ring",
		},
		{
			name:     "short string",
			input:    "short",
			expected: "***",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "8 characters",
			input:    "12345678",
			expected: "***",
		},
		{
			name:     "9 characters",
			input:    "123456789",
			expected: "1234***6789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskSensitive(tt.input); got != tt.expected {
				t.Errorf("maskSensitive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	// Save original values
	originalRedisAddr := RedisAddr
	originalAPIKey := APIKey
	originalHMACSecret := HMACSecret
	originalTLSCACertFile := TLSCACertFile
	originalTLSClientCAFile := TLSClientCAFile
	originalIdempotencyKeyTTL := IdempotencyKeyTTL
	originalChallengeExpiry := ChallengeExpiry
	originalSessionStorageEnabled := SessionStorageEnabled

	defer func() {
		RedisAddr = originalRedisAddr
		APIKey = originalAPIKey
		HMACSecret = originalHMACSecret
		TLSCACertFile = originalTLSCACertFile
		TLSClientCAFile = originalTLSClientCAFile
		IdempotencyKeyTTL = originalIdempotencyKeyTTL
		ChallengeExpiry = originalChallengeExpiry
		SessionStorageEnabled = originalSessionStorageEnabled
	}()

	// Test with default values
	RedisAddr = ""
	APIKey = ""
	HMACSecret = ""
	TLSCACertFile = ""
	TLSClientCAFile = ""
	IdempotencyKeyTTL = 0
	ChallengeExpiry = 5 * time.Minute
	SessionStorageEnabled = false

	err := Initialize()
	if err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}

	// Test with TLS_CA_CERT_FILE alias
	TLSClientCAFile = "test-ca.pem"
	TLSCACertFile = ""
	err = Initialize()
	if err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
	if TLSCACertFile != "test-ca.pem" {
		t.Errorf("Initialize() should set TLSCACertFile from TLSClientCAFile, got %v", TLSCACertFile)
	}

	// Test with IdempotencyKeyTTL = 0 (should use ChallengeExpiry)
	IdempotencyKeyTTL = 0
	ChallengeExpiry = 10 * time.Minute
	err = Initialize()
	if err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
	if IdempotencyKeyTTL != ChallengeExpiry {
		t.Errorf("Initialize() should set IdempotencyKeyTTL to ChallengeExpiry when 0, got %v, want %v", IdempotencyKeyTTL, ChallengeExpiry)
	}

	// Test with SessionStorageEnabled = true
	SessionStorageEnabled = true
	err = Initialize()
	if err != nil {
		t.Errorf("Initialize() error = %v, want nil", err)
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue bool
		expected     bool
	}{
		{
			name:         "true value",
			envKey:       "TEST_BOOL_VAR",
			envValue:     "true",
			defaultValue: false,
			expected:     true,
		},
		{
			name:         "false value",
			envKey:       "TEST_BOOL_VAR_FALSE",
			envValue:     "false",
			defaultValue: true,
			expected:     false,
		},
		{
			name:         "invalid boolean",
			envKey:       "TEST_BOOL_VAR_INVALID",
			envValue:     "not_a_boolean",
			defaultValue: true,
			expected:     true, // Should return default on parse error
		},
		{
			name:         "env var not set",
			envKey:       "TEST_BOOL_VAR_NOT_SET",
			envValue:     "",
			defaultValue: false,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				if err := os.Setenv(tt.envKey, tt.envValue); err != nil {
					t.Fatalf("failed to set env var: %v", err)
				}
				defer func() {
					if err := os.Unsetenv(tt.envKey); err != nil {
						t.Errorf("failed to unset env var: %v", err)
					}
				}()
			} else {
				if err := os.Unsetenv(tt.envKey); err != nil {
					t.Fatalf("failed to unset env var: %v", err)
				}
			}

			if got := env.GetBool(tt.envKey, tt.defaultValue); got != tt.expected {
				t.Errorf("env.GetBool() = %v, want %v", got, tt.expected)
			}
		})
	}
}
