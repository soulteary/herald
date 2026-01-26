package router

import (
	"testing"

	logger "github.com/soulteary/logger-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// testLogger returns a logger for testing (disabled output)
func testLogger() *logger.Logger {
	return logger.New(logger.Config{
		Level:  logger.ErrorLevel, // Only log errors during tests
		Format: logger.FormatJSON,
	})
}

func TestNewRouterWithClient(t *testing.T) {
	// Setup mock Redis
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() {
		_ = redisClient.Close()
	}()

	// Test that NewRouterWithClient creates a valid Fiber app
	app := NewRouterWithClient(redisClient, testLogger())
	if app == nil {
		t.Fatal("NewRouterWithClient() returned nil")
	}

	// Test with session manager enabled
	// Save original config
	originalSessionStorageEnabled := config.SessionStorageEnabled
	defer func() {
		config.SessionStorageEnabled = originalSessionStorageEnabled
	}()

	config.SessionStorageEnabled = true
	app2 := NewRouterWithClient(redisClient, testLogger())
	if app2 == nil {
		t.Fatal("NewRouterWithClient() with session storage returned nil")
	}

	// Test with test mode enabled
	originalTestMode := config.TestMode
	defer func() {
		config.TestMode = originalTestMode
	}()

	config.TestMode = true
	app3 := NewRouterWithClient(redisClient, testLogger())
	if app3 == nil {
		t.Fatal("NewRouterWithClient() with test mode returned nil")
	}
}
