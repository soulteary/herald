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
			_, owner, err := s.Begin(ctx, "p1", "k1", "fp1")
			if err == nil && owner != "" {
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
	if _, owner, err := s.Begin(ctx, "p1", "k1", "fpA"); err != nil || owner == "" {
		t.Fatalf("first begin owner=%q err=%v", owner, err)
	}
	if _, _, err := s.Begin(ctx, "p1", "k1", "fpB"); err != ErrConflict {
		t.Fatalf("second begin err = %v, want ErrConflict", err)
	}
}

func TestStore_ReplayAfterSucceed(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, owner, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || owner == "" {
		t.Fatalf("begin owner=%q err=%v", owner, err)
	}
	resp := json.RawMessage(`{"challenge_id":"abc"}`)
	if err := s.Succeed(ctx, "p1", "k1", "fp1", owner, resp); err != nil {
		t.Fatalf("succeed: %v", err)
	}
	replay, owner2, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || owner2 != "" {
		t.Fatalf("replay begin owner=%q err=%v", owner2, err)
	}
	if string(replay) != string(resp) {
		t.Fatalf("replay = %s, want %s", string(replay), string(resp))
	}
}

func TestStore_RetryAfterFail(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, owner, err := s.Begin(ctx, "p1", "k1", "fp1"); err != nil || owner == "" {
		t.Fatalf("begin owner=%q err=%v", owner, err)
	} else if err := s.Fail(ctx, "p1", "k1", "fp1", owner); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if _, owner, err := s.Begin(ctx, "p1", "k1", "fp1"); err != nil || owner == "" {
		t.Fatalf("retry begin owner=%q err=%v, want owner after fail", owner, err)
	}
}

func TestStore_DifferentPrincipalsNoCollision(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if _, owner, err := s.Begin(ctx, "pA", "shared", "fp1"); err != nil || owner == "" {
		t.Fatalf("pA begin owner=%q err=%v", owner, err)
	}
	if _, owner, err := s.Begin(ctx, "pB", "shared", "fp1"); err != nil || owner == "" {
		t.Fatalf("pB begin owner=%q err=%v (must not collide with pA)", owner, err)
	}
}

func TestStore_StaleOwnerCannotMutateReplacement(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, staleOwner, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || staleOwner == "" {
		t.Fatalf("first begin owner=%q err=%v", staleOwner, err)
	}

	key := s.redisKey("p1", "k1")
	if err := s.client.Del(ctx, key).Err(); err != nil {
		t.Fatalf("expire pending record: %v", err)
	}
	_, replacementOwner, err := s.Begin(ctx, "p1", "k1", "fp1")
	if err != nil || replacementOwner == "" || replacementOwner == staleOwner {
		t.Fatalf("replacement owner=%q stale=%q err=%v", replacementOwner, staleOwner, err)
	}

	if err := s.Fail(ctx, "p1", "k1", "fp1", staleOwner); err != ErrOwnershipLost {
		t.Fatalf("stale Fail error = %v, want ErrOwnershipLost", err)
	}
	if err := s.Succeed(ctx, "p1", "k1", "fp1", staleOwner, json.RawMessage(`{"stale":true}`)); err != ErrOwnershipLost {
		t.Fatalf("stale Succeed error = %v, want ErrOwnershipLost", err)
	}
	if err := s.Succeed(ctx, "p1", "k1", "fp1", replacementOwner, json.RawMessage(`{"fresh":true}`)); err != nil {
		t.Fatalf("replacement Succeed: %v", err)
	}
}
