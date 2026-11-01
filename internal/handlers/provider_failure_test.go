package handlers

import (
	"context"
	"errors"
	"fmt"
	"testing"

	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"
)

func TestProviderFailureReasonBoundsUntrustedDiagnostics(t *testing.T) {
	for _, reason := range []provider.ErrorReason{
		provider.ReasonSendFailed, provider.ReasonProviderDown, provider.ReasonInvalidConfig,
		provider.ReasonRateLimited, provider.ReasonInvalidDestination, provider.ReasonTimeout,
		provider.ReasonUnauthorized, provider.ReasonNotRegistered, provider.ReasonValidationFailed,
		provider.ReasonIdempotencyConflict,
	} {
		t.Run(string(reason), func(t *testing.T) {
			failure := provider.NewProviderError(reason, "otp=123456 credential=private")
			require.Equal(t, string(reason), providerFailureReason(provider.NewFailureResult("test", provider.ChannelSMS, failure), nil))
			require.Equal(t, string(reason), providerFailureReason(nil, fmt.Errorf("secret wrapper: %w", failure)))
		})
	}
	malicious := provider.NewProviderError(provider.ErrorReason("otp=123456 credential=private"), "private")
	require.Equal(t, "send_failed", providerFailureReason(provider.NewFailureResult("test", provider.ChannelSMS, malicious), malicious))
	require.Equal(t, "send_failed", providerFailureReason(nil, errors.New("otp=123456")))
	require.Equal(t, "send_failed", providerFailureReason(nil, nil))
	require.Equal(t, "timeout", providerFailureReason(nil, fmt.Errorf("private wrapper: %w", context.DeadlineExceeded)))
}
