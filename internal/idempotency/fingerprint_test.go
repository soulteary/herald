package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFingerprintPreservesFieldBoundaries(t *testing.T) {
	cases := []struct {
		name        string
		left, right []string
	}{
		{"shift NUL", []string{"a\x00", "b"}, []string{"a", "\x00b"}},
		{"shift complete fields", []string{"user\x00email\x00a@example.com\x00login\x00first", "email", "a@example.com", "login", "second"}, []string{"user", "email", "a@example.com", "login", "first\x00email\x00a@example.com\x00login\x00second"}},
		{"field count", []string{"a\x00b"}, []string{"a", "b"}},
		{"empty boundaries", []string{"\x00", ""}, []string{"", "\x00"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if Fingerprint(tc.left...) == Fingerprint(tc.right...) {
				t.Fatal("different field tuples share a fingerprint")
			}
		})
	}
}

func TestFingerprintFormatsRemainDisjoint(t *testing.T) {
	for _, parts := range [][]string{{"a\x00b"}, {"\x00", ""}, {"用户", "\x00", "语言"}} {
		if got := Fingerprint(parts...); !strings.HasPrefix(got, "v2:") || len(got) != 67 {
			t.Fatalf("NUL tuple lacks versioned framing: %q", got)
		}
	}
	if got := Fingerprint("v2:", "用户", ""); strings.Contains(got, ":") || len(got) != 64 {
		t.Fatalf("ordinary digest changed format: %q", got)
	}
}

func TestFingerprintLegacyNULRecordsFailClosedWithoutChangingState(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	const key = "legacy-nul-request"
	for _, state := range []State{StatePending, StateSending, StateSucceeded} {
		t.Run(string(state), func(t *testing.T) {
			legacy := sha256.Sum256([]byte("user\x00name\x00email\x00"))
			record := Record{State: state, Fingerprint: hex.EncodeToString(legacy[:]), Owner: "original-owner", ChallengeID: "original-challenge", Response: json.RawMessage(`{"challenge_id":"original-challenge"}`)}
			payload, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			ttl := time.Minute
			if state == StateSending {
				ttl = 0
			}
			redisKey := s.redisKey("principal", key)
			if err := s.client.Set(ctx, redisKey, payload, ttl).Err(); err != nil {
				t.Fatal(err)
			}
			beforeTTL, err := s.client.PTTL(ctx, redisKey).Result()
			if err != nil {
				t.Fatal(err)
			}
			replay, owner, err := s.Begin(ctx, "principal", key, Fingerprint("user\x00name", "email"))
			if err != ErrConflict || owner != "" || replay != nil {
				t.Fatalf("legacy NUL record reopened or replayed: replay=%s owner=%q err=%v", replay, owner, err)
			}
			after, err := s.client.Get(ctx, redisKey).Result()
			if err != nil || after != string(payload) {
				t.Fatalf("legacy record changed: %s %v", after, err)
			}
			afterTTL, err := s.client.PTTL(ctx, redisKey).Result()
			if err != nil || afterTTL != beforeTTL {
				t.Fatalf("legacy retention changed: before=%s after=%s err=%v", beforeTTL, afterTTL, err)
			}
		})
	}
}

func TestFingerprintKeepsExistingOrdinaryRequestsCompatible(t *testing.T) {
	// This is the exact pre-fix wire representation for a normal request.
	legacy := sha256.Sum256([]byte("user\x00email\x00a@example.com\x00login\x00zh-CN\x00"))
	if got := Fingerprint("user", "email", "a@example.com", "login", "zh-CN"); got != hex.EncodeToString(legacy[:]) {
		t.Fatalf("ordinary request changed fingerprint: %s", got)
	}
}
