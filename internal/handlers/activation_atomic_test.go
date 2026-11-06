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
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
)

type deliveredOTP struct{ id, code string }
type capturingDelivery struct{ messages chan deliveredOTP }

func (p *capturingDelivery) Send(_ context.Context, msg *provider.Message) (*provider.SendResult, error) {
	p.messages <- deliveredOTP{msg.IdempotencyKey, msg.Code}
	return provider.NewSuccessResult("capture", provider.ChannelEmail, "message"), nil
}
func (*capturingDelivery) Channel() provider.Channel { return provider.ChannelEmail }
func (*capturingDelivery) Name() string              { return "capture" }
func (*capturingDelivery) Validate() error           { return nil }

type failSupersededDelete struct {
	key      string
	failures atomic.Int64
}

func (*failSupersededDelete) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*failSupersededDelete) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *failSupersededDelete) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "del" && len(cmd.Args()) > 1 && cmd.Args()[1] == h.key {
			h.failures.Add(1)
			return errors.New("injected revocation connection failure")
		}
		return next(ctx, cmd)
	}
}

func TestResendCannotLeaveSupersededCodeRedeemable(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
	defer func() { _ = rc.Close() }()
	h := newIdemHandlers(t, rc)
	p := &capturingDelivery{messages: make(chan deliveredOTP, 2)}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(p))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	first, err := idemRequest(app, "first", faultBody)
	require.NoError(t, err)
	require.Equal(t, 200, first.status)
	old := <-p.messages
	hook := &failSupersededDelete{key: challengeRecordPrefix + old.id}
	rc.AddHook(hook)
	mr.FastForward(61 * time.Second)
	second, err := idemRequest(app, "second", faultBody)
	require.NoError(t, err)
	require.Equal(t, 200, second.status)
	replacement := <-p.messages
	oldResult, _ := h.challengeManager.Verify(context.Background(), old.id, old.code, "")
	newResult, _ := h.challengeManager.Verify(context.Background(), replacement.id, replacement.code, "")
	oldOK := oldResult != nil && oldResult.OK
	newOK := newResult != nil && newResult.OK
	if oldOK || !newOK {
		t.Fatalf("successful resend left invalid lifecycle: statuses=[%d %d] old_redeemable=%v new_redeemable=%v failed_revocations=%d", first.status, second.status, oldOK, newOK, hook.failures.Load())
	}
}

type activationReplyFault struct{ after bool }

func (*activationReplyFault) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*activationReplyFault) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *activationReplyFault) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if cmd.Name() == "eval" && len(cmd.Args()) > 1 && (cmd.Args()[1] == activateAndRevokeScript || strings.Contains(fmt.Sprint(cmd.Args()[1]), "challenge changed before activation")) {
			if h.after {
				if err := next(ctx, cmd); err != nil {
					return err
				}
			}
			return errors.New("activation connection lost")
		}
		return next(ctx, cmd)
	}
}

func TestActivationFailureKeepsDeliveryUncertainWithoutResend(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint("after_commit=", after), func(t *testing.T) {
			mr := miniredis.RunT(t)
			rc := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1})
			defer func() { _ = rc.Close() }()
			h := newIdemHandlers(t, rc)
			p := &capturingDelivery{messages: make(chan deliveredOTP, 3)}
			h.providerRegistry = provider.NewRegistry()
			require.NoError(t, h.providerRegistry.Register(p))
			app := fiber.New()
			app.Post("/v1/otp/challenges", h.CreateChallenge)
			first, err := idemRequest(app, "old-key", faultBody)
			require.NoError(t, err)
			require.Equal(t, 200, first.status)
			old := <-p.messages
			mr.FastForward(61 * time.Second)
			rc.AddHook(&activationReplyFault{after: after})
			second, err := idemRequest(app, "new-key", faultBody)
			require.NoError(t, err)
			require.Equal(t, 500, second.status)
			replacement := <-p.messages
			require.Equal(t, after, mr.Exists(challengeRecordPrefix+replacement.id), "committed active code survives ambiguous cleanup")
			require.Equal(t, !after, mr.Exists(challengeRecordPrefix+old.id))
			retry, err := idemRequest(app, "new-key", faultBody)
			require.NoError(t, err)
			require.Equal(t, 409, retry.status)
			require.Contains(t, string(retry.body), "delivery_unknown")
			require.Empty(t, p.messages)
		})
	}
}
