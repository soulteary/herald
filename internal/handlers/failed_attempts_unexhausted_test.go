package handlers

import (
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

// TestWrongCodesAcrossUnexhaustedChallengesLockUser guesses against one user
// while never using the last attempt of any challenge: MAX_ATTEMPTS-1 wrong
// codes, then a new challenge, repeatedly. No challenge is ever exhausted, so
// the exhausted-challenge policy records nothing. With the default settings
// the fifteenth wrong code locks the user all the same: a correct code is then
// refused with user_locked and so is a new challenge.
func TestWrongCodesAcrossUnexhaustedChallengesLockUser(t *testing.T) {
	const user = "unexhausted-guesser@tenant.example"
	const wrongCodes = 15 // default LOCKOUT_FAILED_ATTEMPT_THRESHOLD

	f := &userLockFixture{creationLimitFixture: newCreationLimitFixture(t)}
	f.rc = f.h.redis
	f.app.Post("/v1/otp/verifications", f.h.VerifyChallenge)

	submitted := 0
	for submitted < wrongCodes {
		ch, code := f.newChallenge(t, user)
		for attempt := 1; attempt < config.MaxAttempts && submitted < wrongCodes; attempt++ {
			status, body := f.verify(t, ch.ID, wrongCode(code))
			require.Equal(t, fiber.StatusUnauthorized, status, body)
			require.Equal(t, "invalid", body["reason"], "wrong code %d must not exhaust its challenge", submitted+1)
			submitted++
		}
	}
	require.Equal(t, 0, f.exhaustedCount(t, user), "no challenge may have been exhausted")

	// A correct code for a fresh challenge is refused: the user is locked.
	ch, code := f.newChallenge(t, user)
	status, body := f.verify(t, ch.ID, code)
	require.Equal(t, fiber.StatusUnauthorized, status, body)
	require.Equal(t, "user_locked", body["reason"])

	// So is asking for another code.
	created := f.create(t, user, "unexhausted-guesser@example.com")
	require.Equal(t, fiber.StatusForbidden, created.status, string(created.body))
	require.Contains(t, string(created.body), `"reason":"user_locked"`)

	// Another user is unaffected.
	other, otherCode := f.newChallenge(t, "someone-else@tenant.example")
	status, body = f.verify(t, other.ID, otherCode)
	require.Equal(t, fiber.StatusOK, status, body)
}
