package handlers

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

type recordingSMSProvider struct {
	mu sync.Mutex
	to []string
}

func (p *recordingSMSProvider) Send(_ context.Context, msg *provider.Message) (*provider.SendResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.to = append(p.to, msg.To)
	return provider.NewSuccessResult("recording", provider.ChannelSMS, "msg-1"), nil
}
func (p *recordingSMSProvider) Channel() provider.Channel { return provider.ChannelSMS }
func (p *recordingSMSProvider) Name() string              { return "recording" }
func (p *recordingSMSProvider) Validate() error           { return nil }

func newSMSLimitApp(t *testing.T, perDestination int, cooldown time.Duration) (*fiber.App, *recordingSMSProvider) {
	t.Helper()
	// newIdemHandlers overwrites these package-level settings; restore them so
	// later tests in the package see their original values.
	oldUser, oldIP, oldDest, oldCooldown := config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination, config.ResendCooldown
	oldCodeLength, oldAttempts, oldExpiry := config.CodeLength, config.MaxAttempts, config.ChallengeExpiry
	oldPurposes, oldPolicy, oldSecret := config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret
	t.Cleanup(func() {
		config.RateLimitPerUser, config.RateLimitPerIP, config.RateLimitPerDestination, config.ResendCooldown = oldUser, oldIP, oldDest, oldCooldown
		config.CodeLength, config.MaxAttempts, config.ChallengeExpiry = oldCodeLength, oldAttempts, oldExpiry
		config.AllowedPurposes, config.ProviderFailurePolicy, config.IdempotencySecret = oldPurposes, oldPolicy, oldSecret
	})
	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	h := newIdemHandlers(t, rc)
	config.RateLimitPerDestination = perDestination
	config.ResendCooldown = cooldown
	rp := &recordingSMSProvider{}
	h.providerRegistry = provider.NewRegistry()
	require.NoError(t, h.providerRegistry.Register(rp))
	app := fiber.New()
	app.Post("/v1/otp/challenges", h.CreateChallenge)
	return app, rp
}

func smsChallengeBody(userID, dest string) string {
	return fmt.Sprintf(`{"user_id":%q,"channel":"sms","destination":%q,"purpose":"login"}`, userID, dest)
}

// TestSMSPrefixVariantsShareDestinationQuota proves that the '+', bare and
// "00" spellings of one number consume a single per-destination quota even
// when each request names a different user.
func TestSMSPrefixVariantsShareDestinationQuota(t *testing.T) {
	app, rp := newSMSLimitApp(t, 1, time.Second)

	first, err := idemRequest(app, "", smsChallengeBody("quota-user-1", "+8613800138000"))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, first.status, string(first.body))

	for i, variant := range []string{"8613800138000", "008613800138000", "+86 138 0013 8000"} {
		resp, err := idemRequest(app, "", smsChallengeBody(fmt.Sprintf("quota-user-v%d", i), variant))
		require.NoError(t, err)
		require.Equal(t, fiber.StatusTooManyRequests, resp.status, "variant %q got its own quota: %s", variant, resp.body)
		require.Contains(t, string(resp.body), "rate_limit_exceeded")
	}

	// A different subscriber and the national form keep their own quota.
	for i, other := range []string{"+8613800138001", "13800138000"} {
		resp, err := idemRequest(app, "", smsChallengeBody(fmt.Sprintf("quota-user-o%d", i), other))
		require.NoError(t, err)
		require.Equal(t, fiber.StatusOK, resp.status, "distinct number %q was limited: %s", other, resp.body)
	}

	// The provider still receives the destination as validated, not the
	// limiter identity.
	require.Equal(t, []string{"+8613800138000", "+8613800138001", "13800138000"}, rp.to)
}

// TestSMSPrefixVariantsShareResendCooldown proves the per-user resend cooldown
// cannot be skipped by respelling the number.
func TestSMSPrefixVariantsShareResendCooldown(t *testing.T) {
	app, rp := newSMSLimitApp(t, 100000, time.Minute)

	first, err := idemRequest(app, "", smsChallengeBody("cooldown-user", "008613800138000"))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, first.status, string(first.body))

	for _, variant := range []string{"+8613800138000", "8613800138000"} {
		resp, err := idemRequest(app, "", smsChallengeBody("cooldown-user", variant))
		require.NoError(t, err)
		require.Equal(t, fiber.StatusTooManyRequests, resp.status, "variant %q skipped the cooldown: %s", variant, resp.body)
		require.Contains(t, string(resp.body), "resend_cooldown")
	}

	other, err := idemRequest(app, "", smsChallengeBody("cooldown-user", "+8613800138001"))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, other.status, string(other.body))
	require.Equal(t, []string{"008613800138000", "+8613800138001"}, rp.to)
}
