package challenge

import (
	"context"
	"encoding/json"
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
	expiry := 5 * time.Minute
	maxAttempts := 5
	lockoutDuration := 10 * time.Minute
	codeLength := 6

	manager := NewManager(redisClient, expiry, maxAttempts, lockoutDuration, codeLength)

	if manager == nil {
		t.Fatal("NewManager() returned nil")
	}
	if manager.expiry != expiry {
		t.Errorf("NewManager() expiry = %v, want %v", manager.expiry, expiry)
	}
	if manager.maxAttempts != maxAttempts {
		t.Errorf("NewManager() maxAttempts = %d, want %d", manager.maxAttempts, maxAttempts)
	}
	if manager.lockoutDuration != lockoutDuration {
		t.Errorf("NewManager() lockoutDuration = %v, want %v", manager.lockoutDuration, lockoutDuration)
	}
	if manager.codeLength != codeLength {
		t.Errorf("NewManager() codeLength = %d, want %d", manager.codeLength, codeLength)
	}
}

func TestManager_CreateChallenge(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	challenge, code, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	if challenge == nil {
		t.Fatal("CreateChallenge() returned nil challenge")
	}

	if challenge.ID == "" {
		t.Error("CreateChallenge() challenge ID is empty")
	}

	if challenge.UserID != userID {
		t.Errorf("CreateChallenge() UserID = %v, want %v", challenge.UserID, userID)
	}

	if challenge.Channel != channel {
		t.Errorf("CreateChallenge() Channel = %v, want %v", challenge.Channel, channel)
	}

	if challenge.Destination != destination {
		t.Errorf("CreateChallenge() Destination = %v, want %v", challenge.Destination, destination)
	}

	if code == "" {
		t.Error("CreateChallenge() code is empty")
	}

	if len(code) != 6 {
		t.Errorf("CreateChallenge() code length = %d, want 6", len(code))
	}

	// Verify challenge is stored in Redis
	key := challengeKeyPrefix + challenge.ID
	val, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("Failed to get challenge from Redis: %v", err)
	}

	var storedChallenge Challenge
	if err := json.Unmarshal([]byte(val), &storedChallenge); err != nil {
		t.Fatalf("Failed to unmarshal challenge: %v", err)
	}

	if storedChallenge.ID != challenge.ID {
		t.Errorf("Stored challenge ID = %v, want %v", storedChallenge.ID, challenge.ID)
	}
}

func TestManager_VerifyChallenge(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, code, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Verify with correct code
	valid, verifiedChallenge, err := manager.VerifyChallenge(ctx, challenge.ID, code, clientIP)
	if err != nil {
		t.Fatalf("VerifyChallenge() error = %v", err)
	}

	if !valid {
		t.Error("VerifyChallenge() should return true for correct code")
	}

	if verifiedChallenge == nil {
		t.Fatal("VerifyChallenge() returned nil challenge")
	}

	if verifiedChallenge.ID != challenge.ID {
		t.Errorf("VerifyChallenge() challenge ID = %v, want %v", verifiedChallenge.ID, challenge.ID)
	}

	// Verify challenge is deleted after successful verification
	key := challengeKeyPrefix + challenge.ID
	_, err = redisClient.Get(ctx, key).Result()
	if err != redis.Nil {
		t.Error("VerifyChallenge() should delete challenge after successful verification")
	}
}

func TestManager_VerifyChallenge_InvalidCode(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, _, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Verify with incorrect code
	valid, _, err := manager.VerifyChallenge(ctx, challenge.ID, "000000", clientIP)
	if err == nil {
		t.Error("VerifyChallenge() should return error for invalid code")
	}

	if valid {
		t.Error("VerifyChallenge() should return false for invalid code")
	}

	// Verify challenge still exists (not deleted on failure)
	key := challengeKeyPrefix + challenge.ID
	_, err = redisClient.Get(ctx, key).Result()
	if err == redis.Nil {
		t.Error("VerifyChallenge() should not delete challenge on invalid code")
	}
}

func TestManager_VerifyChallenge_Expired(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	// Use very short expiry for testing
	manager := NewManager(redisClient, 1*time.Millisecond, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, code, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Try to verify expired challenge
	valid, _, err := manager.VerifyChallenge(ctx, challenge.ID, code, clientIP)
	if err == nil {
		t.Error("VerifyChallenge() should return error for expired challenge")
	}

	if valid {
		t.Error("VerifyChallenge() should return false for expired challenge")
	}
}

func TestManager_VerifyChallenge_MaxAttempts(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 3, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, _, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Try incorrect code multiple times
	for i := 0; i < 3; i++ {
		valid, _, err := manager.VerifyChallenge(ctx, challenge.ID, "000000", clientIP)
		if i < 2 {
			// First two attempts should fail but not lock
			if valid {
				t.Errorf("VerifyChallenge() attempt %d should return false", i+1)
			}
		} else {
			// Third attempt should lock
			if err == nil {
				t.Error("VerifyChallenge() should return error after max attempts")
			}
		}
	}

	// Verify user is locked
	if !manager.IsUserLocked(ctx, userID) {
		t.Error("IsUserLocked() should return true after max attempts")
	}
}

func TestManager_GetChallenge(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, _, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Get challenge
	retrievedChallenge, err := manager.GetChallenge(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("GetChallenge() error = %v", err)
	}

	if retrievedChallenge == nil {
		t.Fatal("GetChallenge() returned nil challenge")
	}

	if retrievedChallenge.ID != challenge.ID {
		t.Errorf("GetChallenge() ID = %v, want %v", retrievedChallenge.ID, challenge.ID)
	}

	if retrievedChallenge.UserID != userID {
		t.Errorf("GetChallenge() UserID = %v, want %v", retrievedChallenge.UserID, userID)
	}
}

func TestManager_GetChallenge_NotFound(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()

	// Try to get non-existent challenge
	_, err := manager.GetChallenge(ctx, "non_existent_id")
	if err == nil {
		t.Error("GetChallenge() should return error for non-existent challenge")
	}
}

func TestManager_RevokeChallenge(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 5, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"
	channel := "email"
	destination := "test@example.com"
	purpose := "login"
	clientIP := "127.0.0.1"

	// Create a challenge
	challenge, _, err := manager.CreateChallenge(ctx, userID, channel, destination, purpose, clientIP)
	if err != nil {
		t.Fatalf("CreateChallenge() error = %v", err)
	}

	// Revoke challenge
	err = manager.RevokeChallenge(ctx, challenge.ID)
	if err != nil {
		t.Fatalf("RevokeChallenge() error = %v", err)
	}

	// Verify challenge is deleted
	key := challengeKeyPrefix + challenge.ID
	_, err = redisClient.Get(ctx, key).Result()
	if err != redis.Nil {
		t.Error("RevokeChallenge() should delete challenge")
	}
}

func TestManager_IsUserLocked(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	manager := NewManager(redisClient, 5*time.Minute, 3, 10*time.Minute, 6)

	ctx := context.Background()
	userID := "user123"

	// User should not be locked initially
	if manager.IsUserLocked(ctx, userID) {
		t.Error("IsUserLocked() should return false for unlocked user")
	}

	// Manually set lock
	lockKey := lockKeyPrefix + userID
	redisClient.Set(ctx, lockKey, "1", 10*time.Minute)

	// User should be locked
	if !manager.IsUserLocked(ctx, userID) {
		t.Error("IsUserLocked() should return true for locked user")
	}
}

func TestGenerateChallengeID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateChallengeID()
		if ids[id] {
			t.Errorf("generateChallengeID() generated duplicate ID: %s", id)
		}
		ids[id] = true

		if len(id) == 0 {
			t.Error("generateChallengeID() returned empty ID")
		}

		if len(id) < 3 {
			t.Errorf("generateChallengeID() ID too short: %s", id)
		}
	}
}

func TestGenerateCode(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "length 4",
			length: 4,
		},
		{
			name:   "length 6",
			length: 6,
		},
		{
			name:   "length 8",
			length: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := generateCode(tt.length)
			if len(code) != tt.length {
				t.Errorf("generateCode() length = %d, want %d", len(code), tt.length)
			}

			// Verify all characters are digits
			for _, c := range code {
				if c < '0' || c > '9' {
					t.Errorf("generateCode() contains non-digit: %c", c)
				}
			}
		})
	}
}

func TestHashCode_VerifyCode(t *testing.T) {
	code := "123456"
	hash := hashCode(code)

	if hash == "" {
		t.Error("hashCode() returned empty hash")
	}

	// Verify code should match
	if !verifyCode(code, hash) {
		t.Error("verifyCode() should return true for correct code")
	}

	// Wrong code should not match
	if verifyCode("000000", hash) {
		t.Error("verifyCode() should return false for incorrect code")
	}

	// Invalid hash format should not match
	if verifyCode(code, "invalid_hash") {
		t.Error("verifyCode() should return false for invalid hash format")
	}
}
