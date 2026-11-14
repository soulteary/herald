package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func tokenAdmission(user, ip, dest, token string) CreationAdmission {
	a := admission(user, ip, dest)
	a.CooldownToken = token
	return a
}

// A release removes the cooldown keys of the admission that set them and
// leaves the three counters as they are.
func TestReleaseCooldowns_RemovesOnlyTheCooldownKeys(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()
	a := tokenAdmission("u", "i", "d", "token-a")
	denied, _, err := m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	require.Equal(t, "token-a", counterValue(t, mr, "ratelimit:cooldown:u+d"))

	released, err := m.ReleaseCooldowns(ctx, a.CooldownKeys, "token-a")
	require.NoError(t, err)
	require.Equal(t, 2, released)
	require.False(t, mr.Exists("ratelimit:cooldown:u+d"))
	require.False(t, mr.Exists("ratelimit:cooldown:dest:d"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:user:u"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:ip:i"))
	require.Equal(t, "1", counterValue(t, mr, "ratelimit:dest:d"))

	// The next admission is not held off, and counts.
	denied, _, err = m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	require.Equal(t, "2", counterValue(t, mr, "ratelimit:dest:d"))
}

// A request whose own cooldown has expired must not remove the cooldown a
// later, delivered request started under the same keys.
func TestReleaseCooldowns_LeavesCooldownsOfOtherAdmissions(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()
	first := tokenAdmission("u", "i", "d", "token-first")
	denied, _, err := m.AdmitCreation(ctx, first)
	require.NoError(t, err)
	require.Empty(t, denied)

	mr.FastForward(first.Cooldown + time.Second)
	second := tokenAdmission("u", "i", "d", "token-second")
	denied, _, err = m.AdmitCreation(ctx, second)
	require.NoError(t, err)
	require.Empty(t, denied)

	for _, token := range []string{"token-first", "unknown", "", plainCooldownMarker} {
		released, err := m.ReleaseCooldowns(ctx, first.CooldownKeys, token)
		require.NoError(t, err)
		require.Zero(t, released, "token %q", token)
	}
	require.Equal(t, "token-second", counterValue(t, mr, "ratelimit:cooldown:u+d"))
	require.Equal(t, "token-second", counterValue(t, mr, "ratelimit:cooldown:dest:d"))
	denied, _, err = m.AdmitCreation(ctx, tokenAdmission("u", "i", "d", "token-third"))
	require.NoError(t, err)
	require.Equal(t, DeniedByCooldown, denied)
}

// Cooldowns written without a token, as releases before this one write them,
// hold the plain marker and cannot be released.
func TestReleaseCooldowns_PlainMarkerIsNeverReleased(t *testing.T) {
	m, mr := admissionFixture(t)
	ctx := context.Background()
	a := admission("u", "i", "d")
	denied, _, err := m.AdmitCreation(ctx, a)
	require.NoError(t, err)
	require.Empty(t, denied)
	require.Equal(t, plainCooldownMarker, counterValue(t, mr, "ratelimit:cooldown:u+d"))

	released, err := m.ReleaseCooldowns(ctx, a.CooldownKeys, plainCooldownMarker)
	require.NoError(t, err)
	require.Zero(t, released)
	require.True(t, mr.Exists("ratelimit:cooldown:u+d"))

	_, err = (&Manager{}).ReleaseCooldowns(ctx, a.CooldownKeys, "token")
	require.Error(t, err)
	mr.Close()
	_, err = m.ReleaseCooldowns(ctx, a.CooldownKeys, "token")
	require.Error(t, err)
}
