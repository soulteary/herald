package handlers

import (
	"github.com/gofiber/fiber/v3"
	logger "github.com/soulteary/logger-kit/v3"
	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
)

// deliveryChannels are the channels a create request may name.
var deliveryChannels = []provider.Channel{provider.ChannelEmail, provider.ChannelSMS, provider.ChannelDingTalk}

// logProviderCoverage reports at startup which channels have a provider and
// which do not. A channel without one is not an error: most deployments use
// one or two channels. It is still worth one line, because the alternative is
// finding out from the first request for that channel.
func logProviderCoverage(registry *provider.Registry, log *logger.Logger) {
	var configured, missing []string
	for _, channel := range deliveryChannels {
		if registry.Has(channel) {
			configured = append(configured, string(channel))
		} else {
			missing = append(missing, string(channel))
		}
	}
	if config.SMSProvider == "" && config.SMSAPIBaseURL != "" {
		log.Warn().Str("check", "providers.sms").
			Msg("SMS_API_BASE_URL is set but SMS_PROVIDER is empty: no SMS provider is registered; set SMS_PROVIDER to enable the sms channel")
	}
	if len(configured) == 0 {
		log.Warn().Str("check", "providers").Strs("channels_without_provider", missing).
			Msg("no delivery provider is configured: no verification code can be delivered on any channel")
		return
	}
	log.Info().Strs("channels_with_provider", configured).Strs("channels_without_provider", missing).
		Msg("Delivery providers registered")
}

// refuseUnconfiguredChannel answers a create request for a channel that has
// no provider, when the provider failure policy is strict. Under the soft
// policy the request continues as before and completes with
// delivery_status=failed, which development and test setups without a
// provider rely on.
func (h *Handlers) refuseUnconfiguredChannel(c fiber.Ctx, req CreateChallengeRequest) (bool, error) {
	if config.NormalizedProviderFailurePolicy() != "strict" || h.providerRegistry.Has(provider.Channel(req.Channel)) {
		return false, nil
	}
	metrics.RecordChallengeCreated(req.Channel, req.Purpose, "channel_not_configured")
	return true, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
		"ok":     false,
		"reason": "channel_not_configured",
	})
}
