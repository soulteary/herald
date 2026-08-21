package idempotency

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewStore(client, "test:idem:", []byte("secret"), time.Minute)
}

// TestStore_ConcurrentBeginSingleOwner proves exactly one concurrent caller
// owns the pending slot; the rest see in-flight/replay, never a second owner.
func TestStore_ConcurrentBeginSingleOwner(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const n = 50
	var owners int64
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, owned, err := s.Begin(ctx, "p1", "k1", "fp1")
			if err == nil && owned {
				atomic.AddInt64(&owners, 1)
			}
		}()
	}
	wg.Wait()
	if owners != 1 {
		t.Fatalf("owners = %d, want exactly 1", owners)
	}
}

func TestStore_ConflictOnDifferentFingerprint(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, owned, err := s.Begin(ctx, "p1", "k1", "fpA"); err != nil || !owned {
		t.Fatalf("first begin owned=%v err=%v", owned, err)
	}
	if _, _, err := s.Begin(ctx, "p1", "k1", "fpB"); err != ErrConflict {
		t.Fatalf("second begin err = %v, want ErrConflict", err)
	}
}

func TestStore_ReplayAfterSucceed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, owned, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || !owned {
		t.Fatalf("begin owned=%v err=%v", owned, err)
	}
	resp := json.RawMessage(`{"challenge_id":"abc"}`)
	if err := s.Succeed(ctx, "p1", "k1", "fp1", resp); err != nil {
		t.Fatalf("succeed: %v", err)
	}
	replay, owned2, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || owned2 {
		t.Fatalf("replay begin owned=%v err=%v", owned2, err)
	}
	if string(replay) != string(resp) {
		t.Fatalf("replay = %s, want %s", string(replay), string(resp))
	}
}

func TestStore_RetryAfterFail(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, owned, err := s.Begin(ctx, "p1", "k1", "fp1"); err != nil || !owned {
		t.Fatalf("begin owned=%v err=%v", owned, err)
	}
	if err := s.Fail(ctx, "p1", "k1"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if _, owned, err := s.Begin(ctx, "p1", "k1", "fp1"); err != nil || !owned {
		t.Fatalf("retry begin owned=%v err=%v, want owned after fail", owned, err)
	}
}

func TestStore_DifferentPrincipalsNoCollision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, owned, err := s.Begin(ctx, "pA", "shared", "fp1"); err != nil || !owned {
		t.Fatalf("pA begin owned=%v err=%v", owned, err)
	}
	if _, owned, err := s.Begin(ctx, "pB", "shared", "fp1"); err != nil || !owned {
		t.Fatalf("pB begin owned=%v err=%v (must not collide with pA)", owned, err)
	}
}
