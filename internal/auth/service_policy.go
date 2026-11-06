package auth

import (
	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/serviceauth"
)

// AuthorizeService checks an authenticated HMAC identity before a route runs.
func AuthorizeService(policies serviceauth.ServicePolicies, action string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if policies == nil {
			return c.Next()
		}
		service, keyID, ok := VerifiedHMACIdentity(c)
		policy, found := policies[keyID]
		if ok && found && policy.Service == service {
			for _, allowed := range policy.Actions {
				if allowed == action {
					return c.Next()
				}
			}
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ok": false, "reason": "forbidden"})
	}
}
