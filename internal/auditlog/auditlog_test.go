package auditlog

import (
	"context"
	"sync"
	"testing"

	audit "github.com/soulteary/audit-kit"
	"github.com/stretchr/testify/assert"
)

func TestAuditLogFunctions(t *testing.T) {
	// Reset logger for testing
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	// Initialize with no-op storage for testing
	storage := audit.NewNoopStorage()
	cfg := audit.DefaultConfig()
	cfg.Enabled = true
	auditLogger = audit.NewLoggerWithWriter(storage, cfg)

	l := GetLogger()
	assert.NotNil(t, l)

	ctx := context.Background()

	// Test all logging functions (should not panic)
	t.Run("LogChallengeCreated", func(t *testing.T) {
		LogChallengeCreated(ctx, "ch_123", "user1", "email", "test@example.com", "login", "127.0.0.1")
	})

	t.Run("LogSendSuccess", func(t *testing.T) {
		LogSendSuccess(ctx, "ch_123", "user1", "email", "test@example.com", "login", "smtp", "msg_123", "127.0.0.1")
	})

	t.Run("LogSendFailed", func(t *testing.T) {
		LogSendFailed(ctx, "ch_123", "user1", "email", "test@example.com", "login", "smtp", "connection_failed", "127.0.0.1")
	})

	t.Run("LogVerificationSuccess", func(t *testing.T) {
		LogVerificationSuccess(ctx, "ch_123", "user1", "email", "test@example.com", "login", "127.0.0.1")
	})

	t.Run("LogVerificationFailed", func(t *testing.T) {
		LogVerificationFailed(ctx, "ch_123", "invalid", "127.0.0.1")
	})

	t.Run("LogChallengeRevoked", func(t *testing.T) {
		LogChallengeRevoked(ctx, "ch_123", "127.0.0.1")
	})

	// Test Stop
	err := Stop()
	assert.NoError(t, err)
}

func TestGetLoggerWithoutInit(t *testing.T) {
	// Reset logger
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	// GetLogger should auto-initialize with no-op storage
	l := GetLogger()
	assert.NotNil(t, l)
}

func TestQuery(t *testing.T) {
	// Reset logger for testing
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	// Initialize with no-op storage for testing
	storage := audit.NewNoopStorage()
	cfg := audit.DefaultConfig()
	cfg.Enabled = true
	auditLogger = audit.NewLoggerWithWriter(storage, cfg)

	ctx := context.Background()
	filter := audit.DefaultQueryFilter()

	records, err := Query(ctx, filter)
	assert.NoError(t, err)
	assert.Empty(t, records) // NoopStorage returns empty
}
