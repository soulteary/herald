package idempotency

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type commandFault struct {
	command string
	missing bool
}

func (*commandFault) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*commandFault) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *commandFault) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == h.command {
			if h.missing {
				return redis.Nil
			}
			return errors.New("connection lost")
		}
		return next(ctx, cmd)
	}
}

func TestStoreBackendFaultsFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, command, raw string
		missing            bool
		want               error
	}{
		{"claim unavailable", "set", "", false, ErrBackendUnavailable},
		{"read unavailable", "get", `{}`, false, ErrBackendUnavailable},
		{"expired between claim and read", "get", `{}`, true, ErrInFlight},
		{"corrupt record", "", `broken`, false, ErrBackendUnavailable},
		{"failed record", "", `{"state":"failed","fingerprint":"fp"}`, false, ErrInFlight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
			defer func() { _ = rc.Close() }()
			s := NewStore(rc, "", []byte("secret"), 0)
			if tc.raw != "" {
				if err := mr.Set(s.redisKey("p", "k"), tc.raw); err != nil {
					t.Fatal(err)
				}
			}
			if tc.command != "" {
				rc.AddHook(&commandFault{command: tc.command, missing: tc.missing})
			}
			replay, owner, err := s.Begin(context.Background(), "p", "k", "fp")
			if !errors.Is(err, tc.want) || replay != nil || owner != "" {
				t.Fatalf("replay=%s owner=%q err=%v", replay, owner, err)
			}
		})
	}
}

func TestStoreOwnedMutationRejectsCorruptionAndBackendFailure(t *testing.T) {
	for _, mode := range []string{"missing", "corrupt", "read unavailable", "watch unavailable", "already sending"} {
		t.Run(mode, func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
			defer func() { _ = rc.Close() }()
			s := NewStore(rc, "", []byte("secret"), time.Minute)
			ctx := context.Background()
			_, owner, err := s.Begin(ctx, "p", "k", "fp")
			if err != nil {
				t.Fatal(err)
			}
			want := ErrBackendUnavailable
			switch mode {
			case "missing":
				mr.Del(s.redisKey("p", "k"))
				want = ErrOwnershipLost
			case "corrupt":
				if err := mr.Set(s.redisKey("p", "k"), "broken"); err != nil {
					t.Fatal(err)
				}
			case "read unavailable":
				rc.AddHook(&commandFault{command: "get"})
			case "watch unavailable":
				rc.AddHook(&commandFault{command: "watch"})
			case "already sending":
				if err := s.MarkSending(ctx, "p", "k", "fp", owner, "ch"); err != nil {
					t.Fatal(err)
				}
				want = ErrDeliveryUnknown
			}
			if err := s.MarkSending(ctx, "p", "k", "fp", owner, "ch"); !errors.Is(err, want) {
				t.Fatalf("err=%v want %v", err, want)
			}
		})
	}
}
