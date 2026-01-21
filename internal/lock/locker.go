// Package lock provides distributed lock functionality for Herald.
// Supports both Redis distributed lock and local lock modes with automatic fallback.
package lock

import (
	// Third-party libraries
	"github.com/redis/go-redis/v9"
	rediskitlock "github.com/soulteary/redis-kit/lock"
)

// Locker provides distributed lock functionality, compatible with gocron.Locker interface
// Supports both Redis distributed lock and local lock modes
//
//nolint:govet // fieldalignment: field order has been optimized, but not further adjusted to maintain API compatibility
type Locker struct {
	Cache      *redis.Client       // Redis client, if nil then use local lock
	hybridLock rediskitlock.Locker // Hybrid locker from redis-kit (auto-fallback to local lock)
}

// NewLocker creates a new distributed locker instance
// If redisClient is nil, it will only use local locking
func NewLocker(redisClient *redis.Client) *Locker {
	return &Locker{
		Cache: redisClient,
	}
}

// Lock acquires distributed lock, compatible with gocron.Locker interface
//
// This function uses redis-kit's HybridLocker which automatically falls back to local lock
// if Redis is unavailable. Lock default expiration time is DefaultLockTime seconds.
//
// Parameters:
//   - key: lock key name, used to identify different locks
//
// Returns:
//   - success: true means successfully acquired lock, false means lock is held by another process
//   - err: error occurred when acquiring lock (e.g., Redis connection error)
//
// Notes:
//   - Lock has automatic expiration time, even if process crashes won't cause permanent deadlock (Redis mode)
//   - Local lock only suitable for single-machine deployment, cannot prevent duplicate execution in multi-instance scenarios
func (s *Locker) Lock(key string) (success bool, err error) {
	// Initialize hybrid lock if not already initialized
	if s.hybridLock == nil {
		s.hybridLock = rediskitlock.NewHybridLocker(s.Cache)
	}
	return s.hybridLock.Lock(key)
}

// Unlock releases distributed lock
//
// This function uses redis-kit's HybridLocker which automatically falls back to local lock
// if Redis is unavailable.
//
// Parameters:
//   - key: lock key name, must match the key name used in Lock
//
// Returns:
//   - error: error occurred when releasing lock
//
// Notes:
//   - Must use the same key as in Lock
//   - If lock has expired or is held by another process, will return error (Redis mode)
func (s *Locker) Unlock(key string) error {
	// Initialize hybrid lock if not already initialized
	if s.hybridLock == nil {
		s.hybridLock = rediskitlock.NewHybridLocker(s.Cache)
	}
	return s.hybridLock.Unlock(key)
}
