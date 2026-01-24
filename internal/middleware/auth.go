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
// Priority: mTLS (if client cert verified) > HMAC > API Key
func RequireAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check mTLS first (if TLS connection with verified client certificate)
		if c.Protocol() == "https" {
			// Get TLS connection state
			tlsConn := c.Context().TLSConnectionState()
			if tlsConn != nil && len(tlsConn.PeerCertificates) > 0 {
				// Client certificate is present and verified (by TLS layer)
				logrus.Debug("Request authenticated via mTLS")
				return c.Next()
			}
		}

		// Check HMAC signature (more secure than API Key)
		signature := c.Get("X-Signature")
		timestamp := c.Get("X-Timestamp")
		service := c.Get("X-Service")
		keyID := c.Get("X-Key-Id") // Support key rotation

		if signature != "" && timestamp != "" {
			// Get HMAC secret based on key ID (if provided)
			hmacSecret := config.GetHMACSecret(keyID)

			if hmacSecret == "" {
				if config.HasHMACKeys() {
					// Multiple keys configured but key ID not found or invalid
					return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
						"ok":     false,
						"reason": "invalid_key_id",
					})
				}
				logrus.Debug("HMAC_SECRET/HERALD_HMAC_KEYS is not configured, trying API key")
			} else {
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
				expectedSig := computeHMAC(timestamp, service, body, hmacSecret)

				if hmac.Equal([]byte(signature), []byte(expectedSig)) {
					if keyID != "" {
						logrus.Debugf("Request authenticated via HMAC with key ID: %s", keyID)
					} else {
						logrus.Debug("Request authenticated via HMAC")
					}
					return c.Next()
				}

				logrus.Debugf("HMAC signature mismatch. Expected: %s, Got: %s", expectedSig, signature)
			}
		}

		// Check API Key header (simple auth)
		apiKey := c.Get("X-API-Key")
		if apiKey != "" && config.APIKey != "" && apiKey == config.APIKey {
			logrus.Debug("Request authenticated via API Key")
			return c.Next()
		}

		// If no authentication method is configured, allow in development but warn
		if config.APIKey == "" && config.HMACSecret == "" && !config.HasHMACKeys() {
			logrus.Warn("No authentication method configured (API_KEY, HMAC_SECRET, or HERALD_HMAC_KEYS), allowing request (development mode)")
			return c.Next()
		}

		// No valid authentication found
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": "unauthorized",
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
