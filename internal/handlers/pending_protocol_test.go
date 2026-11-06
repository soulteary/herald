package handlers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	challengekit "github.com/soulteary/challenge-kit"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
)

type pendingProtocolFault struct{ enabled atomic.Bool }

func (*pendingProtocolFault) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*pendingProtocolFault) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *pendingProtocolFault) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if h.enabled.Load() && cmd.Name() == "eval" && len(cmd.Args()) > 1 {
			script := fmt.Sprint(cmd.Args()[1])
			if strings.Contains(script, "challenge changed before activation") || strings.Contains(script, "local value = redis.call('GET', KEYS[1])") {
				return errors.New("injected activation and cleanup connection failure")
			}
		}
		return next(ctx, cmd)
	}
}

type pendingProtocolDelivery struct {
	capturingDelivery
	fault *pendingProtocolFault
	arm   bool
}

func (p *pendingProtocolDelivery) Send(ctx context.Context, msg *provider.Message) (*provider.SendResult, error) {
	result, err := p.capturingDelivery.Send(ctx, msg)
	if p.arm {
		p.fault.enabled.Store(true)
	}
	return result, err
}

func TestUnactivatedCodeCannotVerifyAfterActivationAndCleanupFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	defer func() { _ = rc.Close() }()
	h := newIdemHandlers(t, rc)
	fault := &pendingProtocolFault{}
	rc.AddHook(fault)
	p := &pendingProtocolDelivery{capturingDelivery: capturingDelivery{messages: make(chan deliveredOTP, 2)}, fault: fault}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	first, err := idemRequest(app, "active", faultBody)
	require.NoError(t, err)
	require.Equal(t, 200, first.status)
	old := <-p.messages
	mr.FastForward(61 * time.Second)
	p.arm = true
	second, err := idemRequest(app, "pending", faultBody)
	require.NoError(t, err)
	require.Equal(t, 500, second.status)
	pending := <-p.messages
	require.True(t, mr.Exists(challengeRecordPrefix+pending.id), "cleanup also failed, so pending must remain harmless")
	fault.enabled.Store(false)
	oldResult, oldErr := h.challengeManager.Verify(context.Background(), old.id, old.code, "")
	require.NoError(t, oldErr)
	require.True(t, oldResult.OK)
	pendingResult, pendingErr := h.challengeManager.Verify(context.Background(), pending.id, pending.code, "")
	require.Error(t, pendingErr)
	require.False(t, pendingResult.OK)
	require.Equal(t, challengekit.ReasonNotActive, pendingResult.Reason)
	legacy := challengekit.NewManager(rc, challengekit.DefaultConfig())
	legacyResult, legacyErr := legacy.Verify(context.Background(), pending.id, pending.code, "")
	require.Error(t, legacyErr)
	require.False(t, legacyResult.OK)
	require.Equal(t, challengekit.ReasonExpired, legacyResult.Reason)
	retry, err := idemRequest(app, "pending", faultBody)
	require.NoError(t, err)
	require.Equal(t, 409, retry.status)
	require.Contains(t, string(retry.body), "delivery_unknown")
}
