package handlers

import (
	"context"
	"errors"

	provider "github.com/soulteary/provider-kit"
)

// Provider messages and underlying errors are untrusted diagnostic text. Only
// known reason codes may cross the logging, tracing and audit boundary.
func providerFailureReason(result *provider.SendResult, err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return string(provider.ReasonTimeout)
	}
	var failure *provider.ProviderError
	if result != nil && result.Error != nil {
		failure = result.Error
	} else {
		_ = errors.As(err, &failure)
	}
	if failure != nil {
		switch failure.Reason {
		case provider.ReasonSendFailed, provider.ReasonProviderDown,
			provider.ReasonInvalidConfig, provider.ReasonRateLimited,
			provider.ReasonInvalidDestination, provider.ReasonTimeout,
			provider.ReasonUnauthorized, provider.ReasonNotRegistered,
			provider.ReasonValidationFailed, provider.ReasonIdempotencyConflict:
			return string(failure.Reason)
		}
	}
	return string(provider.ReasonSendFailed)
}
