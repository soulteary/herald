package ratelimit

import (
	"context"
	"fmt"
	"time"

	rediskitratelimit "github.com/soulteary/redis-kit/ratelimit"

	"github.com/soulteary/herald/internal/metrics"
)

// Limiters that can deny a challenge creation. The values are also the
// rate-limit metric scopes.
const (
	DeniedByUser        = "user"
	DeniedByIP          = "ip"
	DeniedByDestination = "destination"
	DeniedByCooldown    = "resend_cooldown"
)

// DestinationCooldownPrefix namespaces the destination-only cooldown inside
// the cooldown key space. Other cooldown keys are bare hex digests, so the
// two cannot collide.
const DestinationCooldownPrefix = "dest:"

// creationAdmissionScript checks every creation gate and consumes them only
// when all of them admit the request. Separate per-limiter calls would leave
// an earlier counter incremented when a later gate denies.
//
// The counters use the key names and fixed-window semantics of the redis-kit
// limiter (first hit sets the key with PX window, later hits INCR, the window
// is not extended), so counters and cooldowns written by either implementation
// are valid for the other.
//
// KEYS[1..3] are the user, IP and destination counters; KEYS[4..] are cooldown
// keys. ARGV[1..6] are limit and window (ms) per counter; ARGV[7] is the
// cooldown (ms); ARGV[8] is the value stored in the cooldown keys. Only the
// existence and TTL of a cooldown key are ever checked, so any value is valid
// for every reader. The reply is {denying gate, ms until it resets}; gate 0 means
// admitted, 1-3 a counter, 4 a cooldown.
const creationAdmissionScript = `
-- herald:creation-admission
for i = 1, 3 do
	local limit = tonumber(ARGV[2 * i - 1])
	local window = tonumber(ARGV[2 * i])
	if limit <= 0 then
		return {i, window}
	end
	local current = tonumber(redis.call("get", KEYS[i]) or "0")
	if current >= limit then
		local ttl = redis.call("pttl", KEYS[i])
		if ttl < 0 then
			redis.call("pexpire", KEYS[i], window)
			ttl = window
		end
		return {i, ttl}
	end
end
local cooldown = tonumber(ARGV[7])
local held = false
local wait = 0
for i = 4, #KEYS do
	local ttl = redis.call("pttl", KEYS[i])
	if ttl ~= -2 then
		if ttl < 0 then
			redis.call("pexpire", KEYS[i], cooldown)
			ttl = cooldown
		end
		held = true
		if ttl > wait then
			wait = ttl
		end
	end
end
if held then
	return {4, wait}
end
for i = 1, 3 do
	local window = tonumber(ARGV[2 * i])
	if redis.call("exists", KEYS[i]) == 0 then
		redis.call("set", KEYS[i], 1, "px", window)
	else
		redis.call("incr", KEYS[i])
		if redis.call("pttl", KEYS[i]) < 0 then
			redis.call("pexpire", KEYS[i], window)
		end
	end
end
for i = 4, #KEYS do
	redis.call("set", KEYS[i], ARGV[8], "px", cooldown)
end
return {0, 0}
`

// CreationAdmission describes the gates one challenge creation must pass.
// The identity fields are the same values the per-limiter methods take.
type CreationAdmission struct {
	UserID, IP, Destination                 string
	UserLimit, IPLimit, DestinationLimit    int
	UserWindow, IPWindow, DestinationWindow time.Duration
	// CooldownKeys are checked together and all set on admission.
	CooldownKeys []string
	Cooldown     time.Duration
	// CooldownToken is stored as the value of the cooldown keys this
	// admission sets, so that ReleaseCooldowns can later remove exactly
	// these and no cooldown started by another request. Empty stores the
	// plain marker, which cannot be released.
	CooldownToken string
}

// plainCooldownMarker is the cooldown value written when no token is given,
// and by releases before cooldowns could be released.
const plainCooldownMarker = "1"

// releaseCooldownsScript deletes each cooldown key that still holds the
// caller's token and returns how many it deleted. A key that expired, or that
// a later admission has set again, holds another value and is left alone.
const releaseCooldownsScript = `
-- herald:cooldown-release
local released = 0
for i = 1, #KEYS do
	if redis.call("get", KEYS[i]) == ARGV[1] then
		redis.call("del", KEYS[i])
		released = released + 1
	end
end
return released
`

// ReleaseCooldowns removes the cooldown keys that an admission set with the
// given token, and returns how many were removed. It never touches the user,
// IP or destination counters, and it removes nothing when the token is empty
// or the plain marker.
func (m *Manager) ReleaseCooldowns(ctx context.Context, cooldownKeys []string, token string) (int, error) {
	if token == "" || token == plainCooldownMarker || len(cooldownKeys) == 0 {
		return 0, nil
	}
	if m.client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	keys := make([]string, 0, len(cooldownKeys))
	for _, key := range cooldownKeys {
		keys = append(keys, rediskitratelimit.DefaultCooldownPrefix+key)
	}
	start := time.Now()
	released, err := m.client.Eval(ctx, releaseCooldownsScript, keys, token).Int()
	if err != nil {
		metrics.RecordRedisFailure("cooldown_release", time.Since(start))
		return 0, fmt.Errorf("failed to release cooldowns: %w", err)
	}
	metrics.RecordRedisSuccess("cooldown_release", time.Since(start))
	return released, nil
}

// AdmitCreation applies the user, IP and destination limits and the resend
// cooldowns as one atomic step: either every counter is incremented and every
// cooldown key is set, or nothing is consumed. It returns the limiter that
// denied the request ("" when admitted) and when that limiter resets.
func (m *Manager) AdmitCreation(ctx context.Context, a CreationAdmission) (string, time.Time, error) {
	start := time.Now()
	denied, resetTime, err := m.admitCreation(ctx, a)
	if err != nil {
		metrics.RecordRedisFailure("ratelimit_create", time.Since(start))
	} else {
		metrics.RecordRedisSuccess("ratelimit_create", time.Since(start))
	}
	return denied, resetTime, err
}

func (m *Manager) admitCreation(ctx context.Context, a CreationAdmission) (string, time.Time, error) {
	if m.client == nil {
		return "", time.Time{}, fmt.Errorf("redis client is nil")
	}
	windows := []time.Duration{a.UserWindow, a.IPWindow, a.DestinationWindow}
	limits := []int{a.UserLimit, a.IPLimit, a.DestinationLimit}
	args := make([]interface{}, 0, 8)
	for i, window := range windows {
		if window.Milliseconds() <= 0 {
			return "", time.Time{}, fmt.Errorf("window must be positive")
		}
		args = append(args, limits[i], window.Milliseconds())
	}
	if a.Cooldown.Milliseconds() <= 0 {
		return "", time.Time{}, fmt.Errorf("cooldown must be positive")
	}
	token := a.CooldownToken
	if token == "" {
		token = plainCooldownMarker
	}
	args = append(args, a.Cooldown.Milliseconds(), token)

	keys := []string{
		rediskitratelimit.DefaultKeyPrefix + "user:" + a.UserID,
		rediskitratelimit.DefaultKeyPrefix + "ip:" + a.IP,
		rediskitratelimit.DefaultKeyPrefix + "dest:" + a.Destination,
	}
	for _, key := range a.CooldownKeys {
		keys = append(keys, rediskitratelimit.DefaultCooldownPrefix+key)
	}

	reply, err := m.client.Eval(ctx, creationAdmissionScript, keys, args...).Int64Slice()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to apply creation limits: %w", err)
	}
	if len(reply) != 2 {
		return "", time.Time{}, fmt.Errorf("unexpected creation limit response")
	}
	gates := []string{"", DeniedByUser, DeniedByIP, DeniedByDestination, DeniedByCooldown}
	if reply[0] < 0 || reply[0] >= int64(len(gates)) {
		return "", time.Time{}, fmt.Errorf("unexpected creation limit gate")
	}
	if reply[0] == 0 {
		return "", time.Time{}, nil
	}
	ttlMs := reply[1]
	if ttlMs < 0 {
		ttlMs = 0
	}
	return gates[reply[0]], time.Now().Add(time.Duration(ttlMs) * time.Millisecond), nil
}
