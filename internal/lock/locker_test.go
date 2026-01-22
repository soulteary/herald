package lock

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/testutil"
)

// testRedisClient returns a mock Redis client for testing
func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	client, _ := testutil.NewTestRedisClient()
	return client
}

func TestNewLocker(t *testing.T) {
	locker := NewLocker(nil)
	// NewLocker always returns a non-nil pointer
	if locker.Cache != nil {
		t.Error("Expected Cache to be nil when nil client is passed")
	}
}

func TestLocker_Lock_WithRedis(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("failed to close redis client: %v", err)
		}
	}()

	locker := NewLocker(redisClient)

	// Test acquiring lock
	key := "test:lock:1"
	success, err := locker.Lock(key)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	if !success {
		t.Error("Expected lock to be acquired successfully")
	}

	// Test that same key cannot be locked twice
	success2, err2 := locker.Lock(key)
	if err2 != nil {
		t.Fatalf("Second Lock call failed: %v", err2)
	}
	if success2 {
		t.Error("Expected second lock attempt to fail (lock already held)")
	}

	// Unlock
	if err := locker.Unlock(key); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}

	// After unlock, should be able to lock again
	success3, err3 := locker.Lock(key)
	if err3 != nil {
		t.Fatalf("Third Lock call failed: %v", err3)
	}
	if !success3 {
		t.Error("Expected lock to be acquired after unlock")
	}

	// Cleanup
	if err := locker.Unlock(key); err != nil {
		t.Errorf("Final unlock failed: %v", err)
	}
}

func TestLocker_Lock_WithoutRedis(t *testing.T) {
	locker := NewLocker(nil)

	// Test acquiring lock without Redis (should use local lock)
	key := "test:lock:local"
	success, err := locker.Lock(key)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	if !success {
		t.Error("Expected lock to be acquired successfully")
	}

	// Test that same key cannot be locked twice (local lock)
	success2, err2 := locker.Lock(key)
	if err2 != nil {
		t.Fatalf("Second Lock call failed: %v", err2)
	}
	if success2 {
		t.Error("Expected second lock attempt to fail (lock already held)")
	}

	// Unlock
	if err := locker.Unlock(key); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}

	// After unlock, should be able to lock again
	success3, err3 := locker.Lock(key)
	if err3 != nil {
		t.Fatalf("Third Lock call failed: %v", err3)
	}
	if !success3 {
		t.Error("Expected lock to be acquired after unlock")
	}

	// Cleanup
	if err := locker.Unlock(key); err != nil {
		t.Errorf("Final unlock failed: %v", err)
	}
}

func TestLocker_Unlock_WithoutLock(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("failed to close redis client: %v", err)
		}
	}()

	locker := NewLocker(redisClient)

	// Try to unlock a key that was never locked
	key := "test:lock:never_locked"
	err := locker.Unlock(key)
	// Unlock should not fail even if key doesn't exist (graceful handling)
	// The actual behavior depends on redis-kit implementation
	if err != nil {
		t.Logf("Unlock of non-existent key returned error (expected in some cases): %v", err)
	}
}

func TestLocker_ConcurrentLock(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		if err := redisClient.Close(); err != nil {
			t.Errorf("failed to close redis client: %v", err)
		}
	}()

	locker := NewLocker(redisClient)
	key := "test:lock:concurrent"

	// Acquire lock
	success, err := locker.Lock(key)
	if err != nil {
		t.Fatalf("Lock failed: %v", err)
	}
	if !success {
		t.Error("Expected lock to be acquired successfully")
	}

	// Try to acquire same lock from another "process" (simulated by another locker instance)
	locker2 := NewLocker(redisClient)
	success2, err2 := locker2.Lock(key)
	if err2 != nil {
		t.Fatalf("Second locker Lock failed: %v", err2)
	}
	if success2 {
		t.Error("Expected second locker to fail acquiring the same lock")
	}

	// Unlock from first locker
	if err := locker.Unlock(key); err != nil {
		t.Errorf("Unlock failed: %v", err)
	}

	// Now second locker should be able to acquire
	success3, err3 := locker2.Lock(key)
	if err3 != nil {
		t.Fatalf("Second locker Lock after unlock failed: %v", err3)
	}
	if !success3 {
		t.Error("Expected second locker to acquire lock after first unlock")
	}

	// Cleanup
	if err := locker2.Unlock(key); err != nil {
		t.Errorf("Final unlock failed: %v", err)
	}
}
