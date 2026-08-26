package auditlog

import (
	"context"
	"sync"
	"testing"

	audit "github.com/soulteary/audit-kit"
	logger "github.com/soulteary/logger-kit/v2"
	"github.com/stretchr/testify/assert"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

var (
	testLoggerOnce     sync.Once
	testLoggerInstance *logger.Logger
)

// testLogger returns a shared logger for testing. logger.New configures
// zerolog package globals, so constructing a new logger while the asynchronous
// audit writer is emitting an event causes a data race under go test -race.
func testLogger() *logger.Logger {
	testLoggerOnce.Do(func() {
		testLoggerInstance = logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})
	})
	return testLoggerInstance
}

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

func TestSetLogger(t *testing.T) {
	// SetLogger is used by other packages; ensure it doesn't panic
	SetLogger(nil)
}

func TestInit_WithNilRedis(t *testing.T) {
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	Init(nil)
	l := GetLogger()
	assert.NotNil(t, l)
}

func TestStop_WhenNil(t *testing.T) {
	auditLogger = nil
	err := Stop()
	assert.NoError(t, err)
}

func TestInit_WithRedisClient(t *testing.T) {
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	// Use real Redis from testutil so Init picks Redis storage path
	redisClient, err := testutil.NewTestRedisClient()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	defer func() { _ = redisClient.Close() }()

	SetLogger(testLogger())
	Init(redisClient)
	l := GetLogger()
	assert.NotNil(t, l)
}

func TestInit_StorageErrorFallback(t *testing.T) {
	auditLogger = nil
	auditLoggerInit = sync.Once{}

	origStorageType := config.AuditStorageType
	origDBURL := config.AuditDatabaseURL
	defer func() {
		config.AuditStorageType = origStorageType
		config.AuditDatabaseURL = origDBURL
	}()

	// Set storage type to database with invalid URL so NewStorageFromType fails → noop fallback
	config.AuditStorageType = "database"
	config.AuditDatabaseURL = ""

	SetLogger(testLogger())
	Init(nil)
	l := GetLogger()
	assert.NotNil(t, l)
}

func TestSetLoggerConcurrent(t *testing.T) {
	shared := testLogger()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				SetLogger(shared)
				return
			}
			SetLogger(nil)
		}(i)
	}
	wg.Wait()
	SetLogger(nil)
}
