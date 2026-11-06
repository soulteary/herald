package router

import (
	"crypto/subtle"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// metricsAuthMiddleware keeps scraping credentials separate from OTP access.
func metricsAuthMiddleware(key string, production bool) fiber.Handler {
	return func(c fiber.Ctx) error {
		if key == "" && !production {
			return c.Next()
		}
		provided := c.Get("X-Metrics-API-Key")
		if provided == "" && strings.HasPrefix(c.Get("Authorization"), "Bearer ") {
			provided = strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		}
		if key == "" || provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(key)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ok": false, "reason": "unauthorized"})
		}
		return c.Next()
	}
}
