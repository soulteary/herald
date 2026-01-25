package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	rediskitratelimit "github.com/soulteary/redis-kit/ratelimit"

	"github.com/soulteary/herald/internal/metrics"
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
	start := time.Now()
	allowed, remaining, resetTime, err := m.limiter.CheckLimit(ctx, key, limit, window)
	if err != nil {
		metrics.RecordRedisFailure("ratelimit", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("ratelimit", time.Since(start))
	}
	return allowed, remaining, resetTime, err
}

// CheckUserRateLimit checks rate limit for a user
func (m *Manager) CheckUserRateLimit(ctx context.Context, userID string, limit int, window time.Duration) (bool, int, time.Time, error) {
	start := time.Now()
	allowed, remaining, resetTime, err := m.limiter.CheckUserLimit(ctx, userID, limit, window)
	if err != nil {
		metrics.RecordRedisFailure("ratelimit_user", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("ratelimit_user", time.Since(start))
	}
	return allowed, remaining, resetTime, err
}

// CheckIPRateLimit checks rate limit for an IP address
func (m *Manager) CheckIPRateLimit(ctx context.Context, ip string, limit int, window time.Duration) (bool, int, time.Time, error) {
	start := time.Now()
	allowed, remaining, resetTime, err := m.limiter.CheckIPLimit(ctx, ip, limit, window)
	if err != nil {
		metrics.RecordRedisFailure("ratelimit_ip", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("ratelimit_ip", time.Since(start))
	}
	return allowed, remaining, resetTime, err
}

// CheckDestinationRateLimit checks rate limit for a destination (phone/email)
func (m *Manager) CheckDestinationRateLimit(ctx context.Context, destination string, limit int, window time.Duration) (bool, int, time.Time, error) {
	start := time.Now()
	allowed, remaining, resetTime, err := m.limiter.CheckDestinationLimit(ctx, destination, limit, window)
	if err != nil {
		metrics.RecordRedisFailure("ratelimit_dest", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("ratelimit_dest", time.Since(start))
	}
	return allowed, remaining, resetTime, err
}

// CheckResendCooldown checks if resend is allowed (cooldown period)
func (m *Manager) CheckResendCooldown(ctx context.Context, key string, cooldown time.Duration) (bool, time.Time, error) {
	start := time.Now()
	allowed, resetTime, err := m.limiter.CheckCooldown(ctx, key, cooldown)
	if err != nil {
		metrics.RecordRedisFailure("cooldown", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("cooldown", time.Since(start))
	}
	return allowed, resetTime, err
}
