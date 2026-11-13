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
	return challengeLimitResponse(c, reason, "", resetTime)
}

// challengeLimitResponse also names the limit that refused the request
// ("user", "ip" or "destination") when one is given. Each is an identity the
// caller supplied or is, so the field tells it nothing it could not already
// work out by varying its own requests; it saves the operator from guessing
// which of three quotas a 429 came from.
func challengeLimitResponse(c fiber.Ctx, reason, limit string, resetTime time.Time) (bool, error) {
	response := fiber.Map{"ok": false, "reason": reason}
	if limit != "" {
		response["limit"] = limit
	}
	seconds := int64(math.Ceil(time.Until(resetTime).Seconds()))
	if !resetTime.IsZero() && seconds > 0 && seconds <= 24*60*60 {
		response["next_resend_in"] = seconds
		c.Set("Retry-After", strconv.FormatInt(seconds, 10))
	}
	return false, c.Status(fiber.StatusTooManyRequests).JSON(response)
}
