package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedisClient returns a Redis client for testing
// If Redis is not available, tests will be skipped
func testRedisClient(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for testing
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}

	// Clean up test database
	client.FlushDB(ctx)

	return client
}

func TestNewManager(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.redis == nil {
		t.Error("NewManager() redis client is nil")
	}
}

func TestManager_CheckRateLimit_FirstRequest(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	key := "test_key"
	limit := 10
	window := time.Hour

	allowed, remaining, resetTime, err := manager.CheckRateLimit(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("CheckRateLimit() error = %v", err)
	}

	if !allowed {
		t.Error("CheckRateLimit() should allow first request")
	}

	if remaining != limit-1 {
		t.Errorf("CheckRateLimit() remaining = %d, want %d", remaining, limit-1)
	}

	if resetTime.IsZero() {
		t.Error("CheckRateLimit() resetTime should not be zero")
	}
}

func TestManager_CheckRateLimit_WithinLimit(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	key := "test_key"
	limit := 10
	window := time.Hour

	// Make multiple requests
	for i := 0; i < 5; i++ {
		allowed, remaining, _, err := manager.CheckRateLimit(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("CheckRateLimit() error = %v", err)
		}

		if !allowed {
			t.Errorf("CheckRateLimit() should allow request %d", i+1)
		}

		expectedRemaining := limit - (i + 1)
		if remaining != expectedRemaining {
			t.Errorf("CheckRateLimit() remaining = %d, want %d", remaining, expectedRemaining)
		}
	}
}

func TestManager_CheckRateLimit_Exceeded(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	key := "test_key"
	limit := 3
	window := time.Hour

	// Make requests up to limit
	for i := 0; i < limit; i++ {
		allowed, _, _, err := manager.CheckRateLimit(ctx, key, limit, window)
		if err != nil {
			t.Fatalf("CheckRateLimit() error = %v", err)
		}

		if !allowed {
			t.Errorf("CheckRateLimit() should allow request %d", i+1)
		}
	}

	// Next request should be rate limited
	allowed, remaining, resetTime, err := manager.CheckRateLimit(ctx, key, limit, window)
	if err != nil {
		t.Fatalf("CheckRateLimit() error = %v", err)
	}

	if allowed {
		t.Error("CheckRateLimit() should not allow request exceeding limit")
	}

	if remaining != 0 {
		t.Errorf("CheckRateLimit() remaining = %d, want 0", remaining)
	}

	if resetTime.IsZero() {
		t.Error("CheckRateLimit() resetTime should not be zero")
	}
}

func TestManager_CheckUserRateLimit(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	userID := "user123"
	limit := 10
	window := time.Hour

	allowed, remaining, _, err := manager.CheckUserRateLimit(ctx, userID, limit, window)
	if err != nil {
		t.Fatalf("CheckUserRateLimit() error = %v", err)
	}

	if !allowed {
		t.Error("CheckUserRateLimit() should allow first request")
	}

	if remaining != limit-1 {
		t.Errorf("CheckUserRateLimit() remaining = %d, want %d", remaining, limit-1)
	}
}

func TestManager_CheckIPRateLimit(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	ip := "127.0.0.1"
	limit := 5
	window := time.Minute

	allowed, remaining, _, err := manager.CheckIPRateLimit(ctx, ip, limit, window)
	if err != nil {
		t.Fatalf("CheckIPRateLimit() error = %v", err)
	}

	if !allowed {
		t.Error("CheckIPRateLimit() should allow first request")
	}

	if remaining != limit-1 {
		t.Errorf("CheckIPRateLimit() remaining = %d, want %d", remaining, limit-1)
	}
}

func TestManager_CheckDestinationRateLimit(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	destination := "test@example.com"
	limit := 10
	window := time.Hour

	allowed, remaining, _, err := manager.CheckDestinationRateLimit(ctx, destination, limit, window)
	if err != nil {
		t.Fatalf("CheckDestinationRateLimit() error = %v", err)
	}

	if !allowed {
		t.Error("CheckDestinationRateLimit() should allow first request")
	}

	if remaining != limit-1 {
		t.Errorf("CheckDestinationRateLimit() remaining = %d, want %d", remaining, limit-1)
	}
}

func TestManager_CheckResendCooldown_FirstTime(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	key := "user123:test@example.com"
	cooldown := 60 * time.Second

	allowed, resetTime, err := manager.CheckResendCooldown(ctx, key, cooldown)
	if err != nil {
		t.Fatalf("CheckResendCooldown() error = %v", err)
	}

	if !allowed {
		t.Error("CheckResendCooldown() should allow first request")
	}

	if resetTime.IsZero() {
		t.Error("CheckResendCooldown() resetTime should not be zero")
	}
}

func TestManager_CheckResendCooldown_WithinCooldown(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient)

	ctx := context.Background()
	key := "user123:test@example.com"
	cooldown := 60 * time.Second

	// First request should be allowed
	allowed, _, err := manager.CheckResendCooldown(ctx, key, cooldown)
	if err != nil {
		t.Fatalf("CheckResendCooldown() error = %v", err)
	}

	if !allowed {
		t.Error("CheckResendCooldown() should allow first request")
	}

	// Second request should be blocked
	allowed, resetTime, err := manager.CheckResendCooldown(ctx, key, cooldown)
	if err != nil {
		t.Fatalf("CheckResendCooldown() error = %v", err)
	}

	if allowed {
		t.Error("CheckResendCooldown() should not allow request within cooldown")
	}

	if resetTime.IsZero() {
		t.Error("CheckResendCooldown() resetTime should not be zero")
	}
}
