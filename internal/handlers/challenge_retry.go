package handlers

import (
	"math"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
)

// challengeRateLimitResponse reports the gate that rejected this request. Later
// gates can still reject a future request; this does not reserve quota or reset it.
func challengeRateLimitResponse(c fiber.Ctx, reason string, resetTime time.Time) (bool, error) {
	response := fiber.Map{"ok": false, "reason": reason}
	seconds := int64(math.Ceil(time.Until(resetTime).Seconds()))
	if !resetTime.IsZero() && seconds > 0 && seconds <= 24*60*60 {
		response["next_resend_in"] = seconds
		c.Set("Retry-After", strconv.FormatInt(seconds, 10))
	}
	return false, c.Status(fiber.StatusTooManyRequests).JSON(response)
}
