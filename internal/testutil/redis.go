package testutil

import (
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	rediskittestutil "github.com/soulteary/redis-kit/testutil"
)

// NewTestRedisClient creates an in-memory Redis client for testing.
//
// It uses miniredis, which implements real Redis semantics (including Lua
// EVAL, SET NX, and WATCH/MULTI) so tests exercise the same atomic code paths
// as production. The redis-kit MockRedis only understood a few hardcoded Lua
// scripts and could not validate the distributed-lock / swap-active / atomic
// idempotency invariants introduced in the security hardening work.
//
// The second return value is retained for source compatibility with existing
// callers that expect a *MockRedis; it is nil because miniredis is used
// instead. Callers only ever used the *redis.Client.
func NewTestRedisClient() (*redis.Client, *rediskittestutil.MockRedis) {
	mr, err := miniredis.Run()
	if err != nil {
		panic("testutil: failed to start miniredis: " + err.Error())
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return client, nil
}
