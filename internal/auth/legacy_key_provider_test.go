package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// legacyTestSecrets is the key map behind the provider under test. The empty
// key ID resolves to a default, as config.GetHMACSecret does when
// HERALD_HMAC_DEFAULT_KEY_ID or HMAC_SECRET is set.
var legacyTestSecrets = map[string]string{
	"":      "default-secret",
	"key-1": "secret-1",
	"key-2": "secret-2",
}

func legacyTestProvider(calls *[]string) KeyProvider {
	return func(keyID string) string {
		*calls = append(*calls, keyID)
		return legacyTestSecrets[keyID]
	}
}

// TestLegacyKeyProvider_KnownKeyIDReturnsItsSecret proves a configured key ID
// resolves to the inner provider's secret, with one lookup and no
// invalid_key_id count.
func TestLegacyKeyProvider_KnownKeyIDReturnsItsSecret(t *testing.T) {
	var calls []string
	provider := LegacyKeyProvider(legacyTestProvider(&calls))
	before := authFailureCount(t, "invalid_key_id")

	require.Equal(t, "secret-1", provider("key-1"))
	require.Equal(t, "secret-2", provider("key-2"))

	require.Equal(t, []string{"key-1", "key-2"}, calls)
	require.InDelta(t, before, authFailureCount(t, "invalid_key_id"), 0)
}

// TestLegacyKeyProvider_UnknownKeyIDReturnsProcessSecret proves a non-empty
// key ID the inner provider does not know resolves to one non-empty value
// that is the same for every unknown ID and every call, differs from every
// configured secret, and is counted as invalid_key_id once per lookup.
func TestLegacyKeyProvider_UnknownKeyIDReturnsProcessSecret(t *testing.T) {
	var calls []string
	provider := LegacyKeyProvider(legacyTestProvider(&calls))
	before := authFailureCount(t, "invalid_key_id")

	first := provider("no-such-key")
	second := provider("no-such-key")
	other := provider("another-missing-key")

	require.NotEmpty(t, first)
	require.Len(t, first, 64, "32 random bytes, hex encoded")
	require.Equal(t, first, second, "repeated lookups of one unknown ID")
	require.Equal(t, first, other, "lookups of different unknown IDs")
	for keyID, secret := range legacyTestSecrets {
		require.NotEqual(t, secret, first, "configured key %q", keyID)
	}
	require.Equal(t, unknownKeySecret(), first, "same secret as the v2/v3 path")
	require.Equal(t, first, LegacyKeyProvider(legacyTestProvider(&calls))("no-such-key"),
		"a second wrapper in the same process")

	require.InDelta(t, before+4, authFailureCount(t, "invalid_key_id"), 0)
	require.Equal(t, []string{"no-such-key", "no-such-key", "another-missing-key", "no-such-key"}, calls)
}

// TestLegacyKeyProvider_EmptyKeyIDIsPassedThrough proves a lookup that names
// no key gets the inner provider's answer unchanged: its default secret when
// it has one, and an empty string, not the process secret, when it has none.
// Neither case is counted as invalid_key_id.
func TestLegacyKeyProvider_EmptyKeyIDIsPassedThrough(t *testing.T) {
	before := authFailureCount(t, "invalid_key_id")

	var calls []string
	withDefault := LegacyKeyProvider(legacyTestProvider(&calls))
	require.Equal(t, "default-secret", withDefault(""))
	require.Equal(t, []string{""}, calls)

	withoutDefault := LegacyKeyProvider(func(keyID string) string {
		if keyID == "key-1" {
			return "secret-1"
		}
		return ""
	})
	require.Empty(t, withoutDefault(""))

	require.InDelta(t, before, authFailureCount(t, "invalid_key_id"), 0)
}

// TestLegacyKeyProvider_NilInnerProvider proves a nil inner provider is
// treated as one that knows no key: no panic, an empty key ID stays empty,
// and any other key ID resolves to the process secret and is counted.
func TestLegacyKeyProvider_NilInnerProvider(t *testing.T) {
	provider := LegacyKeyProvider(nil)
	require.NotNil(t, provider)
	before := authFailureCount(t, "invalid_key_id")

	require.Empty(t, provider(""))
	require.InDelta(t, before, authFailureCount(t, "invalid_key_id"), 0)

	secret := provider("key-1")
	require.NotEmpty(t, secret)
	require.Equal(t, unknownKeySecret(), secret)
	require.InDelta(t, before+1, authFailureCount(t, "invalid_key_id"), 0)
}
