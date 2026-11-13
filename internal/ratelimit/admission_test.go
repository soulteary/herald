package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func admissionFixture(t *testing.T) (*Manager, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return NewManager(rc), mr
}

func admission(user, ip, dest string) CreationAdmission {
	return CreationAdmission{
		UserID: user, UserLimit: 10, UserWindow: time.Hour,
		IP: ip, IPLimit: 10, IPWindow: time.Minute,
		Destination: dest, DestinationLimit: 10, DestinationWindow: time.Hour,
		CooldownKeys: []string{user + "+" + dest, DestinationCooldownPrefix + dest},
		Cooldown:     time.Minute,
	}
}

func counterValue(t *testing.T, mr *miniredis.Miniredis, key string) string {
	t.Helper()
	if !mr.Exists(key) {
		return ""
	}
	value, err := mr.Get(key)
	require.NoError(t, err)
	return value
}

func TestAdmitCreation_AdmitsAndConsumesEveryGateOnce(t *testing.T) {
	m, mr := admissionFixture(t)
	denied, _, err := m.AdmitCreation(context.Background(), admission("u", "i", "d"))
	require.NoError(t, err)
	require.Empty(t, denied)

	require.Equal(t, "1", counterValue(t, mr, "ratelimit:user:u"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:ip:i"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:dest:d"))
	require.Equal(t, time.Hour, mr.TTL("ratelimit:user:u"))
	require.Equal(t, time.Minute, mr.TTL("ratelimit:ip:i"))
	require.Equal(t, time.Hour, mr.TTL("ratelimit:dest:d"))
	require.Equal(t, time.Minute, mr.TTL("ratelimit:cooldown:u+d"))
	require.Equal(t, time.Minute, mr.TTL("ratelimit:cooldown:dest:d"))
}

// The combined step must read and write the counters the redis-kit limiter
// uses, so counters recorded before an upgrade still apply after it and both
// implementations can run side by side during a rolling upgrade.
func TestAdmitCreation_SharesCountersWithPerLimiterCalls(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		allowed, _, _, err := m.CheckUserRateLimit(ctx, "u", 3, time.Hour)
		require.NoError(t, err)
		require.True(t, allowed)
	}
	mr.FastForward(10 * time.Minute)

	a := admission("u", "i", "d")
	a.UserLimit = 3
	denied, _, err := m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	require.Equal(t, "3", counterValue(t, mr, "ratelimit:user:u"))
	// A later hit does not extend the fixed window.
	require.Equal(t, 50*time.Minute, mr.TTL("ratelimit:user:u"))

	// The per-limiter call now sees the counter written by the combined step,
	// and the cooldown it set.
	allowed, remaining, _, err := m.CheckUserRateLimit(ctx, "u", 3, time.Hour)
	require.NoError(t, err)
	require.False(t, allowed)
	require.Zero(t, remaining)
	allowed, _, err = m.CheckResendCooldown(ctx, "u+d", time.Minute)
	require.NoError(t, err)
	require.False(t, allowed)

	// A cooldown set by the per-limiter call denies the combined step.
	allowed, _, err = m.CheckResendCooldown(ctx, "u2+d2", time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)
	b := admission("u2", "i", "d2")
	denied, _, err = m.AdmitCreation(ctx, b)
	require.NoError(t, err)
	require.Equal(t, DeniedByCooldown, denied)
}

func TestAdmitCreation_DenialConsumesNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deny   func(*CreationAdmission)
		seed   func(*miniredis.Miniredis)
		want   string
		maxTTL time.Duration
	}{
		{"user limit", func(a *CreationAdmission) { a.UserLimit = 1 }, func(mr *miniredis.Miniredis) {
			require.NoError(t, mr.Set("ratelimit:user:u", "1"))
			mr.SetTTL("ratelimit:user:u", 20*time.Minute)
		}, DeniedByUser, 20 * time.Minute},
		{"ip limit", func(a *CreationAdmission) { a.IPLimit = 1 }, func(mr *miniredis.Miniredis) {
			require.NoError(t, mr.Set("ratelimit:ip:i", "1"))
			mr.SetTTL("ratelimit:ip:i", 30*time.Second)
		}, DeniedByIP, 30 * time.Second},
		{"destination limit", func(a *CreationAdmission) { a.DestinationLimit = 1 }, func(mr *miniredis.Miniredis) {
			require.NoError(t, mr.Set("ratelimit:dest:d", "1"))
			mr.SetTTL("ratelimit:dest:d", 40*time.Minute)
		}, DeniedByDestination, 40 * time.Minute},
		{"user and destination cooldown", func(*CreationAdmission) {}, func(mr *miniredis.Miniredis) {
			require.NoError(t, mr.Set("ratelimit:cooldown:u+d", "1"))
			mr.SetTTL("ratelimit:cooldown:u+d", 45*time.Second)
		}, DeniedByCooldown, 45 * time.Second},
		{"destination cooldown", func(*CreationAdmission) {}, func(mr *miniredis.Miniredis) {
			require.NoError(t, mr.Set("ratelimit:cooldown:dest:d", "1"))
			mr.SetTTL("ratelimit:cooldown:dest:d", 15*time.Second)
		}, DeniedByCooldown, 15 * time.Second},
		{"non-positive limit", func(a *CreationAdmission) { a.IPLimit = 0 }, func(*miniredis.Miniredis) {}, DeniedByIP, time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, mr := admissionFixture(t)
			tc.seed(mr)
			before := mr.Dump()
			a := admission("u", "i", "d")
			tc.deny(&a)

			start := time.Now()
			denied, reset, err := m.AdmitCreation(context.Background(), a)
			require.NoError(t, err)
			require.Equal(t, tc.want, denied)
			require.Equal(t, before, mr.Dump(), "a denied request must not change any key")
			// The reset time is now plus the denying key's remaining TTL.
			require.WithinDuration(t, start.Add(tc.maxTTL), reset, 5*time.Second)
		})
	}
}

// When both cooldown keys are held, the caller must wait for the longer one.
func TestAdmitCreation_ReportsLongestCooldown(t *testing.T) {
	m, mr := admissionFixture(t)
	require.NoError(t, mr.Set("ratelimit:cooldown:u+d", "1"))
	mr.SetTTL("ratelimit:cooldown:u+d", 10*time.Second)
	require.NoError(t, mr.Set("ratelimit:cooldown:dest:d", "1"))
	mr.SetTTL("ratelimit:cooldown:dest:d", 50*time.Second)

	start := time.Now()
	denied, reset, err := m.AdmitCreation(context.Background(), admission("u", "i", "d"))
	require.NoError(t, err)
	require.Equal(t, DeniedByCooldown, denied)
	require.WithinDuration(t, start.Add(50*time.Second), reset, 5*time.Second)
}

func TestAdmitCreation_WindowAndCooldownExpire(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()
	a := admission("u", "i", "d")
	a.IPLimit = 1

	denied, _, err := m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	denied, _, err = m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Equal(t, DeniedByIP, denied)

	mr.FastForward(time.Minute + time.Second)
	denied, _, err = m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	require.Equal(t, "2", counterValue(t, mr, "ratelimit:user:u"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:ip:i"))
}

// A key that lost its TTL would otherwise deny forever.
func TestAdmitCreation_RestoresMissingTTL(t *testing.T) {
	m, mr := admissionFixture(t)
	require.NoError(t, mr.Set("ratelimit:user:u", "5"))
	a := admission("u", "i", "d")
	a.UserLimit = 5
	denied, _, err := m.AdmitCreation(context.Background(), a)
	require.NoError(t, err)
	require.Equal(t, DeniedByUser, denied)
	require.Equal(t, time.Hour, mr.TTL("ratelimit:user:u"))
	require.Equal(t, "5", counterValue(t, mr, "ratelimit:user:u"))
}

func TestAdmitCreation_ConcurrentRequestsAdmitExactlyTheLimit(t *testing.T) {
	m, mr := admissionFixture(t)
	const requests, limit = 64, 7
	var admitted, deniedByIP atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < requests; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Distinct users and destinations share one IP limit.
			id := string(rune('A'+i/26)) + string(rune('a'+i%26))
			a := admission("u"+id, "shared", "d"+id)
			a.IPLimit = limit
			denied, _, err := m.AdmitCreation(context.Background(), a)
			if err != nil {
				t.Errorf("AdmitCreation: %v", err)
				return
			}
			switch denied {
			case "":
				admitted.Add(1)
			case DeniedByIP:
				deniedByIP.Add(1)
			default:
				t.Errorf("denied by %q", denied)
			}
		}(i)
	}
	wg.Wait()
	require.EqualValues(t, limit, admitted.Load())
	require.EqualValues(t, requests-limit, deniedByIP.Load())
	require.Equal(t, "7", counterValue(t, mr, "ratelimit:ip:shared"))
	// Denied requests left no user counter behind.
	users := 0
	for _, key := range mr.Keys() {
		if len(key) > len("ratelimit:user:") && key[:len("ratelimit:user:")] == "ratelimit:user:" {
			users++
		}
	}
	require.Equal(t, limit, users)
}

func TestAdmitCreation_FailsClosed(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()

	bad := admission("u", "i", "d")
	bad.Cooldown = 0
	_, _, err := m.AdmitCreation(ctx, bad)
	require.Error(t, err)
	bad = admission("u", "i", "d")
	bad.IPWindow = 0
	_, _, err = m.AdmitCreation(ctx, bad)
	require.Error(t, err)
	require.Empty(t, mr.Keys())

	// A counter that is not a number is a backend fault, not an admission.
	require.NoError(t, mr.Set("ratelimit:dest:d", "corrupt"))
	denied, _, err := m.AdmitCreation(ctx, admission("u", "i", "d"))
	require.Error(t, err)
	require.Empty(t, denied)
	require.False(t, mr.Exists("ratelimit:user:u"))

	mr.Close()
	denied, _, err = m.AdmitCreation(ctx, admission("u", "i", "d"))
	require.Error(t, err)
	require.Empty(t, denied)

	_, _, err = (&Manager{}).AdmitCreation(ctx, admission("u", "i", "d"))
	require.Error(t, err)
}
