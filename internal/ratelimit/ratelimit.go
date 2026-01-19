package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
)

const (
	rateLimitKeyPrefix = "ratelimit:"
)

// Manager handles rate limiting operations
type Manager struct {
	redis *redis.Client
}

// NewManager creates a new rate limit manager
func NewManager(redisClient *redis.Client) *Manager {
	return &Manager{
		redis: redisClient,
	}
}

// CheckRateLimit checks if a request should be rate limited
// Returns (allowed, remaining, resetTime, error)
func (m *Manager) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	redisKey := rateLimitKeyPrefix + key

	// Get current count
	count, err := m.redis.Get(ctx, redisKey).Int()
	if err == redis.Nil {
		// First request, set count to 1
		if err := m.redis.Set(ctx, redisKey, 1, window).Err(); err != nil {
			return false, 0, time.Time{}, fmt.Errorf("failed to set rate limit: %w", err)
		}
		remaining := limit - 1
		resetTime := time.Now().Add(window)
		return true, remaining, resetTime, nil
	}
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to get rate limit: %w", err)
	}

	// Check if limit exceeded
	if count >= limit {
		ttl := m.redis.TTL(ctx, redisKey).Val()
		resetTime := time.Now().Add(ttl)
		logrus.Debugf("Rate limit exceeded for key: %s (count: %d, limit: %d)", key, count, limit)
		return false, 0, resetTime, nil
	}

	// Increment count
	newCount, err := m.redis.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	// Set expiration if this is the first increment (key was just created)
	// When key already exists, expiration is already set, so we only need to set it for new keys
	if newCount == 1 {
		if err := m.redis.Expire(ctx, redisKey, window).Err(); err != nil {
			logrus.Warnf("Failed to set expiration on rate limit key: %v", err)
		}
	} else {
		// Ensure expiration is set even if key existed (defensive programming)
		ttl := m.redis.TTL(ctx, redisKey).Val()
		if ttl <= 0 {
			// Key exists but has no expiration, set it
			if err := m.redis.Expire(ctx, redisKey, window).Err(); err != nil {
				logrus.Warnf("Failed to set expiration on existing rate limit key: %v", err)
			}
		}
	}

	remaining := limit - int(newCount)
	if remaining < 0 {
		remaining = 0
	}

	ttl := m.redis.TTL(ctx, redisKey).Val()
	resetTime := time.Now().Add(ttl)

	return true, remaining, resetTime, nil
}

// CheckUserRateLimit checks rate limit for a user
func (m *Manager) CheckUserRateLimit(ctx context.Context, userID string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("user:%s", userID)
	return m.CheckRateLimit(ctx, key, limit, window)
}

// CheckIPRateLimit checks rate limit for an IP address
func (m *Manager) CheckIPRateLimit(ctx context.Context, ip string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("ip:%s", ip)
	return m.CheckRateLimit(ctx, key, limit, window)
}

// CheckDestinationRateLimit checks rate limit for a destination (phone/email)
func (m *Manager) CheckDestinationRateLimit(ctx context.Context, destination string, limit int, window time.Duration) (bool, int, time.Time, error) {
	key := fmt.Sprintf("dest:%s", destination)
	return m.CheckRateLimit(ctx, key, limit, window)
}

// CheckResendCooldown checks if resend is allowed (cooldown period)
func (m *Manager) CheckResendCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, time.Time, error) {
	redisKey := rateLimitKeyPrefix + "cooldown:" + key

	exists, err := m.redis.Exists(ctx, redisKey).Result()
	if err != nil {
		return false, time.Time{}, fmt.Errorf("failed to check cooldown: %w", err)
	}

	if exists > 0 {
		ttl := m.redis.TTL(ctx, redisKey).Val()
		resetTime := time.Now().Add(ttl)
		return false, resetTime, nil
	}

	// Set cooldown
	if err := m.redis.Set(ctx, redisKey, "1", cooldown).Err(); err != nil {
		return false, time.Time{}, fmt.Errorf("failed to set cooldown: %w", err)
	}

	resetTime := time.Now().Add(cooldown)
	return true, resetTime, nil
}
