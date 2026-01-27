package handlers

import (
	"testing"
)

// TestProviderIdempotency_NotDuplicateSend documents that provider-level "same idempotency_key
// must not trigger duplicate Send" is not yet asserted by a mock. Current coverage:
// TestHandlers_CreateChallenge_Idempotency verifies that the same Idempotency-Key returns the
// same challenge_id (no new challenge created), which implies the provider Send path is not
// invoked again for the duplicate request. A full guarantee would inject a mock provider
// and assert Send was called exactly once for that idempotency_key.
func TestProviderIdempotency_NotDuplicateSend(t *testing.T) {
	t.Skip("Provider call-count assertion requires injectable mock provider; idempotency is currently verified via challenge_id equality in TestHandlers_CreateChallenge_Idempotency")
}
