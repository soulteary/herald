package audit

import (
	"context"
	"testing"
	"time"

	"github.com/soulteary/herald/internal/audit/types"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

func TestNewManager(t *testing.T) {
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient)
	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.cache == nil {
		t.Error("NewManager() cache is nil")
	}
}

func TestManager_Log(t *testing.T) {
	// Save original config
	originalAuditEnabled := config.AuditEnabled
	defer func() {
		config.AuditEnabled = originalAuditEnabled
	}()

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient)

	tests := []struct {
		name           string
		auditEnabled   bool
		record         *types.AuditRecord
		shouldStore    bool
		checkTimestamp bool
	}{
		{
			name:         "challenge created",
			auditEnabled: true,
			record: &types.AuditRecord{
				EventType:   types.EventChallengeCreated,
				ChallengeID: "ch_123",
				UserID:      "user123",
				Channel:     "email",
				Destination: "test@example.com",
				Purpose:     "login",
				Result:      "success",
				IP:          "127.0.0.1",
			},
			shouldStore:    true,
			checkTimestamp: true,
		},
		{
			name:         "challenge verified",
			auditEnabled: true,
			record: &types.AuditRecord{
				EventType:   types.EventChallengeVerified,
				ChallengeID: "ch_456",
				UserID:      "user456",
				Result:      "success",
				IP:          "192.168.1.1",
			},
			shouldStore:    true,
			checkTimestamp: true,
		},
		{
			name:         "send failed",
			auditEnabled: true,
			record: &types.AuditRecord{
				EventType:   types.EventSendFailed,
				ChallengeID: "ch_789",
				UserID:      "user789",
				Channel:     "sms",
				Result:      "failure",
				Reason:      "provider_error",
				Provider:    "aliyun",
				IP:          "10.0.0.1",
			},
			shouldStore:    true,
			checkTimestamp: true,
		},
		{
			name:         "audit disabled",
			auditEnabled: false,
			record: &types.AuditRecord{
				EventType: types.EventChallengeCreated,
				Result:    "success",
			},
			shouldStore: false,
		},
		{
			name:         "record with timestamp",
			auditEnabled: true,
			record: &types.AuditRecord{
				EventType:   types.EventChallengeCreated,
				ChallengeID: "ch_timestamp",
				Result:      "success",
				Timestamp:   time.Now().Unix(),
			},
			shouldStore:    true,
			checkTimestamp: false, // Already set
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config.AuditEnabled = tt.auditEnabled

			// Clear Redis before test
			ctx := context.Background()
			redisClient.FlushDB(ctx)

			manager.Log(ctx, tt.record)

			if tt.shouldStore {
				// Verify record was stored (check that a key exists)
				// The exact key format depends on challenge_id or user_id
				// Check if any key with the prefix exists
				keys, err := redisClient.Keys(ctx, auditKeyPrefix+"*").Result()
				if err != nil {
					t.Logf("Failed to check keys (may be expected with mock): %v", err)
					return
				}
				if len(keys) == 0 && tt.auditEnabled {
					t.Error("Expected audit record to be stored, but no keys found")
				}
			}

			// Verify timestamp was set if needed
			if tt.checkTimestamp && tt.record.Timestamp == 0 {
				t.Error("Expected timestamp to be set, but it was 0")
			}
		})
	}
}

func TestMaskDestination(t *testing.T) {
	tests := []struct {
		name       string
		dest       string
		channel    string
		expected   string
		shouldMask bool
	}{
		{
			name:       "email masking",
			dest:       "user@example.com",
			channel:    "email",
			expected:   "u***@example.com",
			shouldMask: true,
		},
		{
			name:       "email single character",
			dest:       "a@example.com",
			channel:    "email",
			expected:   "a***@example.com",
			shouldMask: true,
		},
		{
			name:       "email empty local part",
			dest:       "@example.com",
			channel:    "email",
			expected:   "****@example.com",
			shouldMask: true,
		},
		{
			name:       "phone number long",
			dest:       "+8613800138000",
			channel:    "sms",
			expected:   "+86****000",
			shouldMask: true,
		},
		{
			name:       "phone number short",
			dest:       "13800138000",
			channel:    "sms",
			expected:   "138****000",
			shouldMask: true,
		},
		{
			name:       "phone number very short",
			dest:       "123456",
			channel:    "sms",
			expected:   "****",
			shouldMask: true,
		},
		{
			name:       "phone number medium",
			dest:       "1380013800",
			channel:    "sms",
			expected:   "****",
			shouldMask: true,
		},
		{
			name:       "unknown channel",
			dest:       "test@example.com",
			channel:    "unknown",
			expected:   "****",
			shouldMask: true,
		},
		{
			name:       "empty destination",
			dest:       "",
			channel:    "email",
			expected:   "",
			shouldMask: false,
		},
		{
			name:       "invalid email format",
			dest:       "notanemail",
			channel:    "email",
			expected:   "****",
			shouldMask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskDestination(tt.dest, tt.channel)
			if result != tt.expected {
				t.Errorf("MaskDestination(%q, %q) = %q, want %q", tt.dest, tt.channel, result, tt.expected)
			}
		})
	}
}

func TestManager_Log_WithMasking(t *testing.T) {
	// Save original config
	originalAuditEnabled := config.AuditEnabled
	originalAuditMaskDestination := config.AuditMaskDestination
	defer func() {
		config.AuditEnabled = originalAuditEnabled
		config.AuditMaskDestination = originalAuditMaskDestination
	}()

	config.AuditEnabled = true
	config.AuditMaskDestination = true

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient)
	ctx := context.Background()

	record := &types.AuditRecord{
		EventType:   types.EventChallengeCreated,
		ChallengeID: "ch_mask",
		UserID:      "user123",
		Channel:     "email",
		Destination: "user@example.com",
		Result:      "success",
	}

	manager.Log(ctx, record)

	// Verify destination was masked
	if record.Destination != "u***@example.com" {
		t.Errorf("Expected destination to be masked, got %q", record.Destination)
	}
}

func TestEventTypes(t *testing.T) {
	// Verify all event types are defined
	eventTypes := []types.EventType{
		types.EventChallengeCreated,
		types.EventChallengeVerified,
		types.EventChallengeRevoked,
		types.EventSendSuccess,
		types.EventSendFailed,
		types.EventVerificationFailed,
	}

	for _, et := range eventTypes {
		if et == "" {
			t.Error("Event type is empty")
		}
	}
}

func TestAuditRecord_KeyGeneration(t *testing.T) {
	// Save original config
	originalAuditEnabled := config.AuditEnabled
	defer func() {
		config.AuditEnabled = originalAuditEnabled
	}()

	config.AuditEnabled = true

	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient)
	ctx := context.Background()

	tests := []struct {
		name           string
		record         *types.AuditRecord
		expectedPrefix string
	}{
		{
			name: "with challenge_id",
			record: &types.AuditRecord{
				EventType:   types.EventChallengeCreated,
				ChallengeID: "ch_123",
				Result:      "success",
			},
			expectedPrefix: auditKeyPrefix,
		},
		{
			name: "with user_id only",
			record: &types.AuditRecord{
				EventType: types.EventChallengeCreated,
				UserID:    "user123",
				Result:    "success",
			},
			expectedPrefix: auditKeyPrefix,
		},
		{
			name: "with neither challenge_id nor user_id",
			record: &types.AuditRecord{
				EventType: types.EventChallengeCreated,
				Result:    "success",
			},
			expectedPrefix: auditKeyPrefix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redisClient.FlushDB(ctx)
			manager.Log(ctx, tt.record)

			// Verify a key was created with the prefix
			keys, err := redisClient.Keys(ctx, auditKeyPrefix+"*").Result()
			if err != nil {
				t.Logf("Failed to check keys (may be expected with mock): %v", err)
				return
			}
			if len(keys) == 0 {
				t.Error("Expected at least one audit key to be created")
			} else {
				// Verify key starts with prefix
				for _, key := range keys {
					if len(key) < len(tt.expectedPrefix) || key[:len(tt.expectedPrefix)] != tt.expectedPrefix {
						t.Errorf("Key %q does not start with prefix %q", key, tt.expectedPrefix)
					}
				}
			}
		})
	}
}
