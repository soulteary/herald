package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	rediskitclient "github.com/soulteary/redis-kit/client"

	"github.com/soulteary/herald/internal/challenge"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/otp"
	"github.com/soulteary/herald/internal/provider"
	"github.com/soulteary/herald/internal/ratelimit"
	"github.com/soulteary/herald/internal/session"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	challengeManager *challenge.Manager
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	redis            *redis.Client
	testCodeCache    rediskitcache.Cache // For test mode code storage
	sessionManager   *session.Manager    // Optional: nil if session storage is disabled
}

// NewHandlers creates a new handlers instance
func NewHandlers(redisClient *redis.Client, sessionManager *session.Manager) *Handlers {
	challengeMgr := challenge.NewManager(
		redisClient,
		config.ChallengeExpiry,
		config.MaxAttempts,
		config.LockoutDuration,
		config.CodeLength,
	)

	rateLimitMgr := ratelimit.NewManager(redisClient)

	// Initialize provider registry
	registry := provider.NewRegistry()

	// Register SMTP provider if configured
	if config.SMTPHost != "" {
		smtpProvider := provider.NewSMTPProvider()
		if err := registry.Register(smtpProvider); err != nil {
			logrus.Warnf("Failed to register SMTP provider: %v", err)
		} else {
			logrus.Info("SMTP provider registered")
		}
	}

	// Register SMS provider if configured
	if config.SMSProvider != "" {
		smsProvider := provider.NewSMSProvider()
		if err := registry.Register(smsProvider); err != nil {
			logrus.Warnf("Failed to register SMS provider: %v", err)
		} else {
			logrus.Info("SMS provider registered")
		}
	}

	// Create test code cache for test mode
	testCodeCache := rediskitcache.NewCache(redisClient, "otp:test:code:")

	return &Handlers{
		challengeManager: challengeMgr,
		rateLimitManager: rateLimitMgr,
		providerRegistry: registry,
		redis:            redisClient,
		testCodeCache:    testCodeCache,
		sessionManager:   sessionManager,
	}
}

// HealthCheck handles health check requests
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	ctx := c.Context()

	// Check Redis connection using redis-kit
	if !rediskitclient.HealthCheck(ctx, h.redis) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  "Redis connection failed",
		})
	}

	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "herald",
	})
}

// CreateChallengeRequest represents the request to create a challenge
type CreateChallengeRequest struct {
	UserID      string `json:"user_id"`
	Channel     string `json:"channel"` // "sms" | "email"
	Destination string `json:"destination"`
	Purpose     string `json:"purpose"`
	Locale      string `json:"locale"`
	ClientIP    string `json:"client_ip"`
	UA          string `json:"ua"`
}

// CreateChallenge handles challenge creation
func (h *Handlers) CreateChallenge(c *fiber.Ctx) error {
	ctx := c.Context()

	var req CreateChallengeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}

	// Validate request
	if req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "user_id_required",
		})
	}

	if req.Channel != "sms" && req.Channel != "email" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_channel",
		})
	}

	if req.Destination == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "destination_required",
		})
	}

	// Get client IP
	clientIP := req.ClientIP
	if clientIP == "" {
		clientIP = c.IP()
	}

	// Check rate limits
	// 1. Per user
	allowed, _, _, err := h.rateLimitManager.CheckUserRateLimit(
		ctx, req.UserID, config.RateLimitPerUser, time.Hour,
	)
	if err != nil {
		logrus.Errorf("Rate limit check failed: %v", err)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 2. Per IP
	allowed, _, _, err = h.rateLimitManager.CheckIPRateLimit(
		ctx, clientIP, config.RateLimitPerIP, time.Minute,
	)
	if err != nil {
		logrus.Errorf("Rate limit check failed: %v", err)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 3. Per destination
	allowed, _, _, err = h.rateLimitManager.CheckDestinationRateLimit(
		ctx, req.Destination, config.RateLimitPerDestination, time.Hour,
	)
	if err != nil {
		logrus.Errorf("Rate limit check failed: %v", err)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 4. Resend cooldown
	cooldownKey := fmt.Sprintf("%s:%s", req.UserID, req.Destination)
	allowed, _, err = h.rateLimitManager.CheckResendCooldown(ctx, cooldownKey, config.ResendCooldown)
	if err != nil {
		logrus.Errorf("Cooldown check failed: %v", err)
	}
	if !allowed {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "resend_cooldown",
		})
	}

	// Check if user is locked
	if h.challengeManager.IsUserLocked(ctx, req.UserID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ok":     false,
			"reason": "user_locked",
		})
	}

	// Create challenge
	ch, code, err := h.challengeManager.CreateChallenge(
		ctx, req.UserID, req.Channel, req.Destination, req.Purpose, clientIP,
	)
	if err != nil {
		logrus.Errorf("Failed to create challenge: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Store code in test mode (for integration testing only)
	if config.TestMode {
		if err := h.testCodeCache.Set(ctx, ch.ID, code, config.ChallengeExpiry); err != nil {
			logrus.Warnf("Failed to store test code: %v", err)
		}
	}

	// Send verification code via provider
	channel := provider.Channel(req.Channel)
	msg := &provider.Message{
		To:   req.Destination,
		Code: code,
	}

	if channel == provider.ChannelEmail {
		msg.Subject, msg.Body = provider.FormatVerificationEmail(code, req.Locale)
	} else {
		msg.Body = provider.FormatVerificationSMS(code, req.Locale)
	}

	if err := h.providerRegistry.Send(ctx, channel, msg); err != nil {
		logrus.Errorf("Failed to send verification code via provider: %v", err)
		// Don't fail the request, challenge is already created
		// The code can still be verified manually if needed
		// However, in production, you may want to return an error here
		// to prevent creating challenges that cannot be delivered
	}

	// Return response
	return c.JSON(fiber.Map{
		"challenge_id":   ch.ID,
		"expires_in":     int(config.ChallengeExpiry.Seconds()),
		"next_resend_in": int(config.ResendCooldown.Seconds()),
	})
}

// VerifyChallengeRequest represents the request to verify a challenge
type VerifyChallengeRequest struct {
	ChallengeID string `json:"challenge_id"`
	Code        string `json:"code"`
	ClientIP    string `json:"client_ip"`
}

// VerifyChallenge handles challenge verification
func (h *Handlers) VerifyChallenge(c *fiber.Ctx) error {
	ctx := c.Context()

	var req VerifyChallengeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
		})
	}

	if req.ChallengeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "challenge_id_required",
		})
	}

	if req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "code_required",
		})
	}

	// Validate code format
	if !otp.ValidateCodeFormat(req.Code, config.CodeLength) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_code_format",
		})
	}

	// Verify challenge
	valid, ch, err := h.challengeManager.VerifyChallenge(ctx, req.ChallengeID, req.Code, req.ClientIP)
	if err != nil {
		logrus.Debugf("Challenge verification failed: %v", err)

		// Determine error reason
		reason := "verification_failed"
		errStr := err.Error()
		if contains(errStr, "expired") {
			reason = "expired"
		} else if contains(errStr, "locked") {
			reason = "locked"
		} else if contains(errStr, "invalid") {
			reason = "invalid"
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": reason,
		})
	}

	if !valid {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid",
		})
	}

	// Generate AMR based on channel
	amr := []string{"otp"}
	switch ch.Channel {
	case "sms":
		amr = append(amr, "sms")
	case "email":
		amr = append(amr, "email")
	}

	// Success
	return c.JSON(fiber.Map{
		"ok":        true,
		"user_id":   ch.UserID,
		"amr":       amr,
		"issued_at": time.Now().Unix(),
	})
}

// RevokeChallenge handles challenge revocation
func (h *Handlers) RevokeChallenge(c *fiber.Ctx) error {
	ctx := c.Context()
	challengeID := c.Params("id")

	if challengeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "challenge_id_required",
		})
	}

	if err := h.challengeManager.RevokeChallenge(ctx, challengeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
	})
}

// GetTestCode retrieves the verification code for a challenge in test mode
// This endpoint is only available when HERALD_TEST_MODE=true
func (h *Handlers) GetTestCode(c *fiber.Ctx) error {
	if !config.TestMode {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"ok":     false,
			"reason": "not_found",
		})
	}

	challengeID := c.Params("challenge_id")
	if challengeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "challenge_id_required",
		})
	}

	ctx := c.Context()
	var code string
	if err := h.testCodeCache.Get(ctx, challengeID, &code); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"ok":     false,
			"reason": "code_not_found",
		})
	}

	return c.JSON(fiber.Map{
		"ok":           true,
		"challenge_id": challengeID,
		"code":         code,
	})
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
