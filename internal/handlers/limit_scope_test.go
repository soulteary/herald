package handlers

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

// TestRateLimitResponseNamesTheRefusingLimit drives each creation quota to
// its limit and checks the 429 says which one refused the request. A cooldown
// denial already has its own reason and carries no limit field.
func TestRateLimitResponseNamesTheRefusingLimit(t *testing.T) {
	type request struct{ user, dest string }
	for _, tc := range []struct {
		name          string
		configure     func()
		first, second request
		reason, limit string
	}{
		{"user", func() { config.RateLimitPerUser = 1 },
			request{"scope-user", "scope-a@example.com"}, request{"scope-user", "scope-b@example.com"}, "rate_limit_exceeded", "user"},
		{"ip", func() { config.RateLimitPerIP = 1 },
			request{"scope-user-1", "scope-a@example.com"}, request{"scope-user-2", "scope-b@example.com"}, "rate_limit_exceeded", "ip"},
		{"destination", func() { config.RateLimitPerDestination = 1 },
			request{"scope-user-1", "scope-a@example.com"}, request{"scope-user-2", "scope-a@example.com"}, "rate_limit_exceeded", "destination"},
		{"cooldown", func() {},
			request{"scope-user", "scope-a@example.com"}, request{"scope-user", "scope-a@example.com"}, "resend_cooldown", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCreationLimitFixture(t)
			tc.configure()

			first := f.create(t, tc.first.user, tc.first.dest)
			require.Equal(t, fiber.StatusOK, first.status, string(first.body))

			second := f.create(t, tc.second.user, tc.second.dest)
			require.Equal(t, fiber.StatusTooManyRequests, second.status, string(second.body))
			var body map[string]any
			require.NoError(t, json.Unmarshal(second.body, &body))
			require.Equal(t, false, body["ok"])
			require.Equal(t, tc.reason, body["reason"])
			if tc.limit == "" {
				require.NotContains(t, body, "limit")
			} else {
				require.Equal(t, tc.limit, body["limit"])
			}
			require.Contains(t, body, "next_resend_in", "retry timing must stay in the response")
		})
	}
}
