package challenge

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit"

	"github.com/soulteary/herald/internal/metrics"
)

const (
	challengeKeyPrefix = "otp:ch:"
	lockKeyPrefix      = "otp:lock:"
)

// Challenge represents a verification challenge
type Challenge struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Channel     string    `json:"channel"` // "sms" | "email"
	Destination string    `json:"destination"`
	CodeHash    string    `json:"code_hash"`
	Purpose     string    `json:"purpose"`
	ExpiresAt   time.Time `json:"expires_at"`
	Attempts    int       `json:"attempts"`
	CreatedIP   string    `json:"created_ip"`
	CreatedAt   time.Time `json:"created_at"`
}

// ChallengeResponse represents the response when creating a challenge
type ChallengeResponse struct {
	ChallengeID  string `json:"challenge_id"`
	ExpiresIn    int    `json:"expires_in"`     // seconds
	NextResendIn int    `json:"next_resend_in"` // seconds
}

// Manager handles challenge operations
type Manager struct {
	cache           rediskitcache.Cache
	lockCache       rediskitcache.Cache
	expiry          time.Duration
	maxAttempts     int
	lockoutDuration time.Duration
	codeLength      int
}

// NewManager creates a new challenge manager
func NewManager(redisClient *redis.Client, expiry time.Duration, maxAttempts int, lockoutDuration time.Duration, codeLength int) *Manager {
	// Create cache instances with appropriate prefixes
	challengeCache := rediskitcache.NewCache(redisClient, challengeKeyPrefix)
	lockCache := rediskitcache.NewCache(redisClient, lockKeyPrefix)

	return &Manager{
		cache:           challengeCache,
		lockCache:       lockCache,
		expiry:          expiry,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
		codeLength:      codeLength,
	}
}

// CreateChallenge creates a new challenge and stores it in Redis
func (m *Manager) CreateChallenge(ctx context.Context, userID, channel, destination, purpose, clientIP string) (*Challenge, string, error) {
	// Generate challenge ID
	challengeID := generateChallengeID()

	// Generate verification code
	code := generateCode(m.codeLength)

	// Hash the code using Argon2
	codeHash := hashCode(code)

	// Create challenge
	challenge := &Challenge{
		ID:          challengeID,
		UserID:      userID,
		Channel:     channel,
		Destination: destination,
		CodeHash:    codeHash,
		Purpose:     purpose,
		ExpiresAt:   time.Now().Add(m.expiry),
		Attempts:    0,
		CreatedIP:   clientIP,
		CreatedAt:   time.Now(),
	}

	// Store in Redis using cache interface
	start := time.Now()
	if err := m.cache.Set(ctx, challengeID, challenge, m.expiry); err != nil {
		metrics.RecordRedisFailure("set", time.Since(start))
		return nil, "", fmt.Errorf("failed to store challenge: %w", err)
	}
	metrics.RecordRedisSuccess("set", time.Since(start))

	logrus.Debugf("Challenge created: %s for user %s", challengeID, userID)
	return challenge, code, nil
}

// GetCodeForTesting retrieves the verification code for a challenge in test mode
// This should only be used in test environments
func (m *Manager) GetCodeForTesting(ctx context.Context, challengeID string) (string, error) {
	var challenge Challenge
	start := time.Now()
	if err := m.cache.Get(ctx, challengeID, &challenge); err != nil {
		metrics.RecordRedisFailure("get", time.Since(start))
		return "", fmt.Errorf("challenge not found: %w", err)
	}
	metrics.RecordRedisSuccess("get", time.Since(start))

	// In test mode, we need to store the code separately
	// For now, return empty - test mode code storage will be handled in handlers
	return "", fmt.Errorf("test mode code retrieval not implemented - use HERALD_TEST_MODE with code storage")
}

// VerifyChallenge verifies a code against a challenge
func (m *Manager) VerifyChallenge(ctx context.Context, challengeID, code, clientIP string) (bool, *Challenge, error) {
	// Get challenge from Redis using cache interface
	var challenge Challenge
	start := time.Now()
	if err := m.cache.Get(ctx, challengeID, &challenge); err != nil {
		metrics.RecordRedisFailure("get", time.Since(start))
		return false, nil, fmt.Errorf("challenge not found or expired: %w", err)
	}
	metrics.RecordRedisSuccess("get", time.Since(start))

	// Check if expired
	if time.Now().After(challenge.ExpiresAt) {
		// Delete expired challenge
		start = time.Now()
		_ = m.cache.Del(ctx, challengeID)
		metrics.RecordRedisSuccess("del", time.Since(start))
		return false, nil, fmt.Errorf("challenge expired")
	}

	// Check if locked
	if challenge.Attempts >= m.maxAttempts {
		// Lock the user
		start = time.Now()
		_ = m.lockCache.Set(ctx, challenge.UserID, "1", m.lockoutDuration)
		metrics.RecordRedisSuccess("set", time.Since(start))
		return false, nil, fmt.Errorf("challenge locked due to too many attempts")
	}

	// Check if user is locked
	start = time.Now()
	exists, err := m.lockCache.Exists(ctx, challenge.UserID)
	metrics.RecordRedisSuccess("exists", time.Since(start))
	if err == nil && exists {
		return false, nil, fmt.Errorf("user is temporarily locked")
	}

	// Verify code
	if !verifyCode(code, challenge.CodeHash) {
		// Increment attempts
		challenge.Attempts++
		start = time.Now()
		ttl, err := m.cache.TTL(ctx, challengeID)
		metrics.RecordRedisSuccess("ttl", time.Since(start))
		if err == nil && ttl > 0 {
			start = time.Now()
			_ = m.cache.Set(ctx, challengeID, challenge, ttl)
			metrics.RecordRedisSuccess("set", time.Since(start))
		}
		// Check if should lock after incrementing attempts
		if challenge.Attempts >= m.maxAttempts {
			// Lock the user
			start = time.Now()
			_ = m.lockCache.Set(ctx, challenge.UserID, "1", m.lockoutDuration)
			metrics.RecordRedisSuccess("set", time.Since(start))
			return false, nil, fmt.Errorf("challenge locked due to too many attempts")
		}
		return false, nil, fmt.Errorf("invalid code")
	}

	// Success - delete challenge (one-time use)
	start = time.Now()
	_ = m.cache.Del(ctx, challengeID)
	metrics.RecordRedisSuccess("del", time.Since(start))

	logrus.Debugf("Challenge verified successfully: %s", challengeID)
	return true, &challenge, nil
}

// GetChallenge retrieves a challenge by ID
func (m *Manager) GetChallenge(ctx context.Context, challengeID string) (*Challenge, error) {
	var challenge Challenge
	start := time.Now()
	if err := m.cache.Get(ctx, challengeID, &challenge); err != nil {
		metrics.RecordRedisFailure("get", time.Since(start))
		return nil, fmt.Errorf("challenge not found: %w", err)
	}
	metrics.RecordRedisSuccess("get", time.Since(start))

	return &challenge, nil
}

// RevokeChallenge revokes a challenge
func (m *Manager) RevokeChallenge(ctx context.Context, challengeID string) error {
	start := time.Now()
	err := m.cache.Del(ctx, challengeID)
	if err != nil {
		metrics.RecordRedisFailure("del", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("del", time.Since(start))
	}
	return err
}

// IsUserLocked checks if a user is locked
func (m *Manager) IsUserLocked(ctx context.Context, userID string) bool {
	start := time.Now()
	exists, err := m.lockCache.Exists(ctx, userID)
	metrics.RecordRedisSuccess("exists", time.Since(start))
	return err == nil && exists
}

// argon2Hasher is a singleton instance for code hashing
var argon2Hasher = secure.NewArgon2Hasher()

// Helper functions

func generateChallengeID() string {
	token, err := secure.RandomToken(16)
	if err != nil {
		// This should never happen with crypto/rand, but handle gracefully
		logrus.Errorf("Failed to generate challenge ID: %v", err)
		token, _ = secure.RandomHex(16)
	}
	return "ch_" + token[:22]
}

func generateCode(length int) string {
	code, err := secure.RandomDigits(length)
	if err != nil {
		// This should never happen with crypto/rand, but handle gracefully
		logrus.Errorf("Failed to generate code: %v", err)
		// Return a fallback - but this is logged for debugging
		code, _ = secure.RandomDigits(length)
	}
	return code
}

func hashCode(code string) string {
	hash, err := argon2Hasher.Hash(code)
	if err != nil {
		// This should never happen, but handle gracefully
		logrus.Errorf("Failed to hash code: %v", err)
		return ""
	}
	return hash
}

func verifyCode(code, hash string) bool {
	// secure.Argon2Hasher.Verify uses constant-time comparison internally
	return argon2Hasher.Verify(hash, code)
}
