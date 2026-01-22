package testutil

import (
	"github.com/redis/go-redis/v9"
	rediskittestutil "github.com/soulteary/redis-kit/testutil"
)

// NewTestRedisClient creates a mock Redis client for testing
// This uses the redis-kit testutil package to provide an in-memory Redis mock
func NewTestRedisClient() (*redis.Client, *rediskittestutil.MockRedis) {
	return rediskittestutil.NewMockRedisClient()
}
