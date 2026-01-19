package challenge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/argon2"
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
	redis           *redis.Client
	expiry          time.Duration
	maxAttempts     int
	lockoutDuration time.Duration
	codeLength      int
}

// NewManager creates a new challenge manager
func NewManager(redisClient *redis.Client, expiry time.Duration, maxAttempts int, lockoutDuration time.Duration, codeLength int) *Manager {
	return &Manager{
		redis:           redisClient,
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

	// Store in Redis
	key := challengeKeyPrefix + challengeID
	data, err := json.Marshal(challenge)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal challenge: %w", err)
	}

	// Store with expiration
	if err := m.redis.Set(ctx, key, data, m.expiry).Err(); err != nil {
		return nil, "", fmt.Errorf("failed to store challenge: %w", err)
	}

	logrus.Debugf("Challenge created: %s for user %s", challengeID, userID)
	return challenge, code, nil
}

// VerifyChallenge verifies a code against a challenge
func (m *Manager) VerifyChallenge(ctx context.Context, challengeID, code, clientIP string) (bool, *Challenge, error) {
	key := challengeKeyPrefix + challengeID

	// Get challenge from Redis
	data, err := m.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return false, nil, fmt.Errorf("challenge not found or expired")
	}
	if err != nil {
		return false, nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	var challenge Challenge
	if err := json.Unmarshal(data, &challenge); err != nil {
		return false, nil, fmt.Errorf("failed to unmarshal challenge: %w", err)
	}

	// Check if expired
	if time.Now().After(challenge.ExpiresAt) {
		// Delete expired challenge
		_ = m.redis.Del(ctx, key)
		return false, nil, fmt.Errorf("challenge expired")
	}

	// Check if locked
	if challenge.Attempts >= m.maxAttempts {
		// Lock the user
		lockKey := lockKeyPrefix + challenge.UserID
		_ = m.redis.Set(ctx, lockKey, "1", m.lockoutDuration)
		return false, nil, fmt.Errorf("challenge locked due to too many attempts")
	}

	// Check if user is locked
	lockKey := lockKeyPrefix + challenge.UserID
	if m.redis.Exists(ctx, lockKey).Val() > 0 {
		return false, nil, fmt.Errorf("user is temporarily locked")
	}

	// Verify code
	if !verifyCode(code, challenge.CodeHash) {
		// Increment attempts
		challenge.Attempts++
		updatedData, _ := json.Marshal(challenge)
		ttl := m.redis.TTL(ctx, key).Val()
		if ttl > 0 {
			_ = m.redis.Set(ctx, key, updatedData, ttl)
		}
		return false, nil, fmt.Errorf("invalid code")
	}

	// Success - delete challenge (one-time use)
	_ = m.redis.Del(ctx, key)

	logrus.Debugf("Challenge verified successfully: %s", challengeID)
	return true, &challenge, nil
}

// GetChallenge retrieves a challenge by ID
func (m *Manager) GetChallenge(ctx context.Context, challengeID string) (*Challenge, error) {
	key := challengeKeyPrefix + challengeID

	data, err := m.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("challenge not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get challenge: %w", err)
	}

	var challenge Challenge
	if err := json.Unmarshal(data, &challenge); err != nil {
		return nil, fmt.Errorf("failed to unmarshal challenge: %w", err)
	}

	return &challenge, nil
}

// RevokeChallenge revokes a challenge
func (m *Manager) RevokeChallenge(ctx context.Context, challengeID string) error {
	key := challengeKeyPrefix + challengeID
	return m.redis.Del(ctx, key).Err()
}

// IsUserLocked checks if a user is locked
func (m *Manager) IsUserLocked(ctx context.Context, userID string) bool {
	lockKey := lockKeyPrefix + userID
	return m.redis.Exists(ctx, lockKey).Val() > 0
}

// Helper functions

func generateChallengeID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "ch_" + base64.URLEncoding.EncodeToString(b)[:22]
}

func generateCode(length int) string {
	b := make([]byte, length)
	// Generate random bytes
	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback: use current time as seed (not secure, but better than panic)
		for i := range b {
			b[i] = byte('0' + (int(time.Now().UnixNano()+int64(i)) % 10))
		}
		return string(b)
	}
	// Convert to digits 0-9
	for i := range b {
		b[i] = byte('0' + (randomBytes[i] % 10))
	}
	return string(b)
}

func hashCode(code string) string {
	// Use Argon2 for hashing
	salt := make([]byte, 16)
	rand.Read(salt)
	hash := argon2.IDKey([]byte(code), salt, 1, 64*1024, 4, 32)
	return base64.URLEncoding.EncodeToString(salt) + ":" + base64.URLEncoding.EncodeToString(hash)
}

func verifyCode(code, hash string) bool {
	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		return false
	}

	salt, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	expectedHash, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	actualHash := argon2.IDKey([]byte(code), salt, 1, 64*1024, 4, 32)
	return string(actualHash) == string(expectedHash)
}
