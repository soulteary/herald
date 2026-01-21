package session

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
	defer func() {
		_ = redisClient.Close()
	}()

	keyPrefix := "test_session:"
	defaultTTL := 1 * time.Hour

	manager := NewManager(redisClient, keyPrefix, defaultTTL)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.cache == nil {
		t.Error("NewManager() cache is nil")
	}
	if manager.defaultTTL != defaultTTL {
		t.Errorf("NewManager() defaultTTL = %v, want %v", manager.defaultTTL, defaultTTL)
	}
}

func TestManager_Create(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
		"role":    "admin",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if sessionID == "" {
		t.Error("Create() returned empty session ID")
	}

	// Verify session exists in Redis
	key := "test_session:" + sessionID
	exists, err := redisClient.Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("Failed to check session existence: %v", err)
	}
	if exists == 0 {
		t.Error("Session not found in Redis")
	}
}

func TestManager_Create_WithCustomTTL(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	customTTL := 30 * time.Minute
	sessionID, err := manager.Create(ctx, data, customTTL)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify TTL is approximately correct (allow 1 second tolerance)
	key := "test_session:" + sessionID
	ttl := redisClient.TTL(ctx, key).Val()
	if ttl < customTTL-time.Second || ttl > customTTL+time.Second {
		t.Errorf("Create() TTL = %v, want approximately %v", ttl, customTTL)
	}
}

func TestManager_Get(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
		"role":    "admin",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Get session
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if session == nil {
		t.Fatal("Get() returned nil session")
	}

	if session.ID != sessionID {
		t.Errorf("Get() session.ID = %s, want %s", session.ID, sessionID)
	}

	if session.Data["user_id"] != "user123" {
		t.Errorf("Get() session.Data[user_id] = %v, want user123", session.Data["user_id"])
	}

	if session.Data["role"] != "admin" {
		t.Errorf("Get() session.Data[role] = %v, want admin", session.Data["role"])
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	_, err := manager.Get(ctx, "nonexistent_session_id")
	if err == nil {
		t.Error("Get() expected error for nonexistent session")
	}
}

func TestManager_Set(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	initialData := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, initialData, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update session data
	updatedData := map[string]interface{}{
		"user_id": "user456",
		"role":    "user",
	}

	err = manager.Set(ctx, sessionID, updatedData, 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify updated data
	session, err := manager.Get(ctx, sessionID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if session.Data["user_id"] != "user456" {
		t.Errorf("Set() session.Data[user_id] = %v, want user456", session.Data["user_id"])
	}

	if session.Data["role"] != "user" {
		t.Errorf("Set() session.Data[role] = %v, want user", session.Data["role"])
	}
}

func TestManager_Set_WithCustomTTL(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update with custom TTL
	newTTL := 2 * time.Hour
	err = manager.Set(ctx, sessionID, data, newTTL)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Verify TTL is updated
	key := "test_session:" + sessionID
	ttl := redisClient.TTL(ctx, key).Val()
	if ttl < newTTL-time.Second || ttl > newTTL+time.Second {
		t.Errorf("Set() TTL = %v, want approximately %v", ttl, newTTL)
	}
}

func TestManager_Delete(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify session exists
	exists, err := manager.Exists(ctx, sessionID)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Session should exist before deletion")
	}

	// Delete session
	err = manager.Delete(ctx, sessionID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify session is deleted
	exists, err = manager.Exists(ctx, sessionID)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Session should not exist after deletion")
	}
}

func TestManager_Exists(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Test existing session
	exists, err := manager.Exists(ctx, sessionID)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() returned false for existing session")
	}

	// Test nonexistent session
	exists, err = manager.Exists(ctx, "nonexistent_session_id")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() returned true for nonexistent session")
	}
}

func TestManager_Refresh(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, data, 30*time.Minute)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Get initial TTL
	key := "test_session:" + sessionID
	initialTTL := redisClient.TTL(ctx, key).Val()

	// Wait a bit to ensure TTL decreased
	time.Sleep(2 * time.Second)

	// Refresh with new TTL
	newTTL := 2 * time.Hour
	err = manager.Refresh(ctx, sessionID, newTTL)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	// Verify TTL is refreshed
	refreshedTTL := redisClient.TTL(ctx, key).Val()
	if refreshedTTL <= initialTTL {
		t.Errorf("Refresh() TTL = %v, should be greater than initial TTL %v", refreshedTTL, initialTTL)
	}

	if refreshedTTL < newTTL-time.Second || refreshedTTL > newTTL+time.Second {
		t.Errorf("Refresh() TTL = %v, want approximately %v", refreshedTTL, newTTL)
	}
}

func TestManager_Refresh_NotFound(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	err := manager.Refresh(ctx, "nonexistent_session_id", 1*time.Hour)
	if err == nil {
		t.Error("Refresh() expected error for nonexistent session")
	}
}

func TestManager_Get_Expired(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	// Create session with very short TTL
	sessionID, err := manager.Create(ctx, data, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Wait for expiration
	time.Sleep(200 * time.Millisecond)

	// Try to get expired session
	_, err = manager.Get(ctx, sessionID)
	if err == nil {
		t.Error("Get() expected error for expired session")
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	manager := NewManager(redisClient, "test_session:", 1*time.Hour)

	ctx := context.Background()
	data := map[string]interface{}{
		"user_id": "user123",
	}

	sessionID, err := manager.Create(ctx, data, 0)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			session, err := manager.Get(ctx, sessionID)
			if err != nil {
				t.Errorf("Concurrent Get() error = %v", err)
			}
			if session == nil {
				t.Error("Concurrent Get() returned nil session")
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
