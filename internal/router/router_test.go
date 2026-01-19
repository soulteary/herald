package router

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
)

// setupTestRedis creates a test Redis client
// In a real scenario, you might want to use a test container or mock
func setupTestRedis(t *testing.T) *redis.Client {
	t.Helper()

	// Use test Redis if available, otherwise skip
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       15, // Use DB 15 for testing
	})

	// Test connection
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping test: Redis not available at %s: %v", addr, err)
	}

	// Clean up test DB
	client.FlushDB(ctx)

	return client
}

func TestNewRouter(t *testing.T) {
	// Note: NewRouter() calls logrus.Fatalf if Redis connection fails,
	// which would exit the program. This test requires Redis to be available.

	// Setup test Redis
	testClient := setupTestRedis(t)
	defer testClient.Close()

	// Save original config values
	originalRedisAddr := os.Getenv("REDIS_ADDR")
	originalRedisPassword := os.Getenv("REDIS_PASSWORD")
	originalRedisDB := os.Getenv("REDIS_DB")

	defer func() {
		// Restore original values
		if originalRedisAddr != "" {
			os.Setenv("REDIS_ADDR", originalRedisAddr)
		} else {
			os.Unsetenv("REDIS_ADDR")
		}
		if originalRedisPassword != "" {
			os.Setenv("REDIS_PASSWORD", originalRedisPassword)
		} else {
			os.Unsetenv("REDIS_PASSWORD")
		}
		if originalRedisDB != "" {
			os.Setenv("REDIS_DB", originalRedisDB)
		} else {
			os.Unsetenv("REDIS_DB")
		}
	}()

	// Set test Redis config
	// Note: config package reads env vars at package init, so this won't work
	// unless we refactor. For now, we test with existing Redis config.
	// In production, you'd want to refactor NewRouter to accept dependencies.

	// Test that NewRouter creates a valid Fiber app
	app := NewRouter()
	if app == nil {
		t.Fatal("NewRouter() returned nil")
	}

	// Verify the app is not nil (it's a *fiber.App, but we can't easily test the type without importing)
	// The fact that NewRouter() didn't panic or return nil means it succeeded
	if app == nil {
		t.Error("App is nil")
	}
}
