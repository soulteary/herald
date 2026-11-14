package handlers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/stretchr/testify/require"
)

// Block exactly where the real manager allocates Argon2 memory. This proves
// admission behavior without allocating many 64 MiB buffers or causing OOM.
type blockingHashManager struct {
	challengekit.ManagerInterface
	entered chan struct{}
	release <-chan struct{}
	active  atomic.Int64
	peak    atomic.Int64
}

func (m *blockingHashManager) Create(context.Context, challengekit.CreateRequest) (*challengekit.Challenge, string, error) {
	n := m.active.Add(1)
	defer m.active.Add(-1)
	for old := m.peak.Load(); n > old; old = m.peak.Load() {
		if m.peak.CompareAndSwap(old, n) {
			break
		}
	}
	m.entered <- struct{}{}
	<-m.release
	return nil, "", errors.New("synthetic hash completion")
}

func TestCreateHashWorkHasBoundedAdmission(t *testing.T) {
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	registerEmailTestProvider(t, h)
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	manager := &blockingHashManager{ManagerInterface: h.challengeManager, entered: make(chan struct{}, 8), release: release}
	h.challengeManager = manager
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	responses := make(chan *fiberTestResp, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func(i int) {
			body := fmt.Sprintf(`{"user_id":"budget-%d","channel":"email","destination":"budget-%d@example.com","purpose":"login"}`, i, i)
			resp, err := idemRequest(app, fmt.Sprintf("budget-%d", i), body)
			if err != nil {
				failures <- err
				return
			}
			responses <- resp
		}(i)
	}
	// Every request must either enter the expensive phase or be rejected, without
	// queueing behind it. Hold the first four operations throughout this check.
	rejected := 0
	for i := 0; i < 8; i++ {
		select {
		case <-manager.entered:
		case resp := <-responses:
			require.Equal(t, 503, resp.status)
			require.JSONEq(t, `{"ok":false,"reason":"hash_capacity_exceeded"}`, string(resp.body))
			rejected++
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(4 * time.Second):
			t.Fatal("request queued behind expensive hash work")
		}
	}
	peak := manager.peak.Load()
	once.Do(func() { close(release) })
	for i := rejected; i < 8; i++ {
		select {
		case <-responses:
		case err := <-failures:
			t.Fatal(err)
		case <-time.After(4 * time.Second):
			t.Fatal("request did not finish")
		}
	}
	t.Logf("peak concurrent Create calls=%d; rejected before hash=%d", peak, rejected)
	require.LessOrEqual(t, peak, int64(4))
	require.GreaterOrEqual(t, rejected, 4)
}

func BenchmarkChallengeCreationAllocation(b *testing.B) {
	mr := miniredis.RunT(b)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { _ = rc.Close() })
	m := newAtomicChallengeManager(rc, challengekit.Config{Expiry: time.Minute, CodeLength: 6, MaxAttempts: 5})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := m.Create(context.Background(), challengekit.CreateRequest{UserID: "allocation", Channel: challengekit.ChannelEmail, Destination: "allocation@example.com", Purpose: "login"})
		if err != nil {
			b.Fatal(err)
		}
	}
}
