package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	rediskitratelimit "github.com/soulteary/redis-kit/ratelimit"
)

// Manager handles rate limiting operations
type Manager struct {
	limiter *rediskitratelimit.RateLimiter
}

// NewManager creates a new rate limit manager
func NewManager(redisClient *redis.Client) *Manager {
	return &Manager{
		limiter: rediskitratelimit.NewRateLimiter(redisClient),
	}
}

// CheckRateLimit checks if a request should be rate limited
// Returns (allowed, remaining, resetTime, error)
func (m *Manager) CheckRateLimit(ctx context.Context, key string, limit int, window time.Duration) (bool, int, time.Time, error) {
	return m.limiter.CheckLimit(ctx, key, limit, window)
}

// CheckUserRateLimit checks rate limit for a user
func (m *Manager) CheckUserRateLimit(ctx context.Context, userID string, limit int, window time.Duration) (bool, int, time.Time, error) {
	return m.limiter.CheckUserLimit(ctx, userID, limit, window)
}

// CheckIPRateLimit checks rate limit for an IP address
func (m *Manager) CheckIPRateLimit(ctx context.Context, ip string, limit int, window time.Duration) (bool, int, time.Time, error) {
	return m.limiter.CheckIPLimit(ctx, ip, limit, window)
}

// CheckDestinationRateLimit checks rate limit for a destination (phone/email)
func (m *Manager) CheckDestinationRateLimit(ctx context.Context, destination string, limit int, window time.Duration) (bool, int, time.Time, error) {
	return m.limiter.CheckDestinationLimit(ctx, destination, limit, window)
}

// CheckResendCooldown checks if resend is allowed (cooldown period)
func (m *Manager) CheckResendCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, time.Time, error) {
	return m.limiter.CheckCooldown(ctx, key, cooldown)
}
