package handlers

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	logger "github.com/soulteary/logger-kit/v3"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

// TestStrictCreateRefusesChannelWithoutProviderBeforeSpendingAnything asks for
// a channel that has no provider. The request is refused before the
// idempotency claim, the hash, the quotas and the cooldowns: Redis holds no
// key afterwards and the same user and a configured channel are still
// admitted.
func TestStrictCreateRefusesChannelWithoutProviderBeforeSpendingAnything(t *testing.T) {
	f := newCreationLimitFixture(t) // strict policy, email provider only
	config.RateLimitPerUser = 1

	before := f.mr.CommandCount()
	resp, err := idemRequest(f.app, "unconfigured-channel-key", smsChallengeBody("coverage-user", "+8613800138000"))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusServiceUnavailable, resp.status, string(resp.body))
	require.JSONEq(t, `{"ok":false,"reason":"channel_not_configured"}`, string(resp.body))
	require.Equal(t, before, f.mr.CommandCount(), "a request for an unconfigured channel reached Redis")
	require.Empty(t, f.mr.Keys(), "quota, cooldown, idempotency or challenge keys were written")
	require.EqualValues(t, 0, f.provider.calls)

	// The single unit of user quota is still there for a channel that works.
	ok := f.create(t, "coverage-user", "coverage@example.com")
	require.Equal(t, fiber.StatusOK, ok.status, string(ok.body))
	require.EqualValues(t, 1, f.provider.calls)
}

// TestSoftCreateWithoutProviderKeepsDeliveryStatusFailed pins the soft-policy
// behaviour that setups without a provider rely on: the challenge is created
// and the response reports the failed delivery.
func TestSoftCreateWithoutProviderKeepsDeliveryStatusFailed(t *testing.T) {
	f := newCreationLimitFixture(t)
	config.ProviderFailurePolicy = "soft"

	resp, err := idemRequest(f.app, "", smsChallengeBody("soft-coverage-user", "+8613800138001"))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))
	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.body, &out))
	require.Equal(t, "failed", out["delivery_status"])
	require.NotEmpty(t, out["challenge_id"])
}

func providerLogLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &line), raw)
		lines = append(lines, line)
	}
	return lines
}

func findLogLine(lines []map[string]any, fragment string) map[string]any {
	for _, line := range lines {
		if msg, _ := line["message"].(string); strings.Contains(msg, fragment) {
			return line
		}
	}
	return nil
}

func TestStartupLogsProviderCoverage(t *testing.T) {
	oldProvider, oldURL := config.SMSProvider, config.SMSAPIBaseURL
	t.Cleanup(func() { config.SMSProvider, config.SMSAPIBaseURL = oldProvider, oldURL })
	newLogger := func(logs *bytes.Buffer) *logger.Logger {
		return logger.New(logger.Config{Level: logger.InfoLevel, Format: logger.FormatJSON, Output: logs})
	}

	t.Run("some channels", func(t *testing.T) {
		config.SMSProvider, config.SMSAPIBaseURL = "", ""
		registry := provider.NewRegistry()
		require.NoError(t, registry.Register(&countingProvider{channel: provider.ChannelEmail}))
		var logs bytes.Buffer
		logProviderCoverage(registry, newLogger(&logs))
		lines := providerLogLines(t, &logs)
		line := findLogLine(lines, "Delivery providers registered")
		require.NotNil(t, line, logs.String())
		require.Equal(t, "info", line["level"])
		require.Equal(t, []any{"email"}, line["channels_with_provider"])
		require.Equal(t, []any{"sms", "dingtalk"}, line["channels_without_provider"])
		require.Nil(t, findLogLine(lines, "SMS_API_BASE_URL"))
	})

	t.Run("no channel", func(t *testing.T) {
		config.SMSProvider, config.SMSAPIBaseURL = "", ""
		var logs bytes.Buffer
		logProviderCoverage(provider.NewRegistry(), newLogger(&logs))
		line := findLogLine(providerLogLines(t, &logs), "no delivery provider is configured")
		require.NotNil(t, line, logs.String())
		require.Equal(t, "warn", line["level"])
		require.Equal(t, []any{"email", "sms", "dingtalk"}, line["channels_without_provider"])
	})

	t.Run("SMS base URL without provider name", func(t *testing.T) {
		config.SMSProvider, config.SMSAPIBaseURL = "", "https://sms.example.com"
		rc := testRedisClient(t)
		t.Cleanup(func() { _ = rc.Close() })
		var logs bytes.Buffer
		h, err := NewHandlersWithError(rc, newLogger(&logs))
		require.NoError(t, err)
		require.False(t, h.providerRegistry.Has(provider.ChannelSMS))
		line := findLogLine(providerLogLines(t, &logs), "SMS_API_BASE_URL is set but SMS_PROVIDER is empty")
		require.NotNil(t, line, logs.String())
		require.Equal(t, "warn", line["level"])
	})
}
