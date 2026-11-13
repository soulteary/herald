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
// cooldown (ms). The reply is {denying gate, ms until it resets}; gate 0 means
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
	redis.call("set", KEYS[i], "1", "px", cooldown)
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
	args := make([]interface{}, 0, 7)
	for i, window := range windows {
		if window.Milliseconds() <= 0 {
			return "", time.Time{}, fmt.Errorf("window must be positive")
		}
		args = append(args, limits[i], window.Milliseconds())
	}
	if a.Cooldown.Milliseconds() <= 0 {
		return "", time.Time{}, fmt.Errorf("cooldown must be positive")
	}
	args = append(args, a.Cooldown.Milliseconds())

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
