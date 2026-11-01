package handlers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/config"
)

type rejectRateKey struct{ prefix string }

func (*rejectRateKey) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*rejectRateKey) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *rejectRateKey) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		for _, arg := range cmd.Args() {
			if key, ok := arg.(string); ok && strings.HasPrefix(key, h.prefix) {
				return errors.New("injected backend outage")
			}
		}
		return next(ctx, cmd)
	}
}

type creationBackend struct {
	challengekit.ManagerInterface
	locked bool
}

func (b creationBackend) IsUserLocked(context.Context, string) bool { return b.locked }
func (b creationBackend) Create(context.Context, challengekit.CreateRequest) (*challengekit.Challenge, string, error) {
	return nil, "", errors.New("backend unavailable")
}

func TestCreateChallengeRateLimitFailures(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		deny         string
		want         int
		reason       string
	}{
		{"IP backend", "ratelimit:ip:", "", 503, "backend_unavailable"},
		{"destination backend", "ratelimit:dest:", "", 503, "backend_unavailable"},
		{"cooldown backend", "ratelimit:cooldown:", "", 503, "backend_unavailable"},
		{"IP budget", "", "ip", 429, "rate_limit_exceeded"},
		{"destination budget", "", "destination", 429, "rate_limit_exceeded"},
		{"locked user", "", "locked", 403, "user_locked"},
		{"challenge backend", "", "create", 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rc := testRedisClient(t)
			defer func() { _ = rc.Close() }()
			h := newIdemHandlers(t, rc)
			if tc.prefix != "" {
				rc.AddHook(&rejectRateKey{prefix: tc.prefix})
			}
			oldIP, oldDest := config.RateLimitPerIP, config.RateLimitPerDestination
			defer func() { config.RateLimitPerIP, config.RateLimitPerDestination = oldIP, oldDest }()
			switch tc.deny {
			case "ip":
				config.RateLimitPerIP = 0
			case "destination":
				config.RateLimitPerDestination = 0
			case "locked", "create":
				h.challengeManager = creationBackend{locked: tc.deny == "locked"}
			}
			app := fiber.New()
			app.Post("/v1/otp/challenges", h.CreateChallenge)
			resp, err := idemRequest(app, "fault-key", faultBody)
			if err != nil {
				t.Fatal(err)
			}
			if resp.status != tc.want || !strings.Contains(string(resp.body), tc.reason) {
				t.Fatalf("status=%d body=%s", resp.status, resp.body)
			}
			// Failures before delivery must release the claim, allowing a later retry.
			keys, err := rc.Keys(context.Background(), "otp:idem2:*").Result()
			if err != nil || len(keys) != 0 {
				t.Fatalf("claim retained: keys=%v err=%v", keys, err)
			}
		})
	}
}
