package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"github.com/soulteary/herald/internal/config"
)

// RequireAuth middleware validates service-to-service authentication
func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// If API key is not configured, skip auth (for development)
		if config.APIKey == "" {
			return c.Next()
		}

		// Check API Key header (simple auth)
		apiKey := c.Get("X-API-Key")
		if apiKey != "" && apiKey == config.APIKey {
			return c.Next()
		}

		// Check HMAC signature (more secure)
		signature := c.Get("X-Signature")
		timestamp := c.Get("X-Timestamp")
		service := c.Get("X-Service")

		if signature != "" && timestamp != "" {
			if config.HMACSecret == "" {
				logrus.Warn("HMAC_SECRET is not configured, falling back to API key")
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"ok":     false,
					"reason": "authentication_required",
				})
			}

			// Validate timestamp (prevent replay attacks)
			ts, err := strconv.ParseInt(timestamp, 10, 64)
			if err != nil {
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"ok":     false,
					"reason": "invalid_timestamp",
				})
			}

			// Check timestamp is within 5 minutes
			now := time.Now().Unix()
			if abs(now-ts) > 300 { // 5 minutes
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"ok":     false,
					"reason": "timestamp_expired",
				})
			}

			// Verify HMAC signature
			body := string(c.Body())
			expectedSig := computeHMAC(timestamp, service, body, config.HMACSecret)

			if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
				logrus.Debugf("HMAC signature mismatch. Expected: %s, Got: %s", expectedSig, signature)
				return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
					"ok":     false,
					"reason": "invalid_signature",
				})
			}

			return c.Next()
		}

		// No valid authentication found
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": "authentication_required",
		})
	}
}

// computeHMAC computes HMAC-SHA256 signature
func computeHMAC(timestamp, service, body, secret string) string {
	message := fmt.Sprintf("%s:%s:%s", timestamp, service, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
