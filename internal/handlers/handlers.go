package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/soulteary/herald/internal/audit"
	"github.com/soulteary/herald/internal/challenge"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/idempotency"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/middleware"
	"github.com/soulteary/herald/internal/otp"
	"github.com/soulteary/herald/internal/provider"
	"github.com/soulteary/herald/internal/ratelimit"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	challengeManager *challenge.Manager
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	idempotencyMgr   *idempotency.Manager
	auditLogger      *audit.Logger
	redis            *redis.Client
}

// NewHandlers creates a new handlers instance
func NewHandlers(redisClient *redis.Client) *Handlers {
	challengeMgr := challenge.NewManager(
		redisClient,
		config.ChallengeExpiry,
		config.MaxAttempts,
		config.LockoutDuration,
		config.CodeLength,
	)

	rateLimitMgr := ratelimit.NewManager(redisClient)
	idempotencyMgr := idempotency.NewManager(redisClient, config.IdempotencyTTL)

	// Initialize audit logger
	auditLogger, err := audit.NewLogger(
		config.AuditLogEnabled,
		config.AuditLogPath,
		config.AuditMaskDestination,
	)
	if err != nil {
		logrus.Errorf("Failed to initialize audit logger: %v", err)
		auditLogger = nil
	} else if config.AuditLogEnabled {
		logrus.Info("Audit logger initialized")
	}

	// Initialize provider registry
	registry := provider.NewRegistry()

	// Register email provider if configured
	if config.EmailAPIURL != "" {
		smtpProvider := provider.NewSMTPProvider()
		if err := registry.Register(smtpProvider); err != nil {
			logrus.Warnf("Failed to register email provider: %v", err)
		} else {
			logrus.Info("Email provider registered")
		}
	}

	// Register SMS provider if configured
	if config.SMSAPIURL != "" {
		smsProvider := provider.NewSMSProvider()
		if err := registry.Register(smsProvider); err != nil {
			logrus.Warnf("Failed to register SMS provider: %v", err)
		} else {
			logrus.Info("SMS provider registered")
		}
	}

	return &Handlers{
		challengeManager: challengeMgr,
		rateLimitManager: rateLimitMgr,
		providerRegistry: registry,
		idempotencyMgr:   idempotencyMgr,
		auditLogger:      auditLogger,
		redis:            redisClient,
	}
}

// HealthCheck handles health check requests
func (h *Handlers) HealthCheck(c *fiber.Ctx) error {
	ctx := c.Context()

	// Check Redis connection
	if err := h.redis.Ping(ctx).Err(); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"status": "unhealthy",
			"error":  err.Error(),
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

	idempotencyKey := getIdempotencyKey(c)
	requestHash := ""
	reservedIdempotency := false
	completedIdempotency := false

	if idempotencyKey != "" {
		requestHash = hashIdempotencyRequest(req)
		record, err := h.idempotencyMgr.Get(ctx, idempotencyKey)
		if err != nil {
			logrus.Errorf("Failed to load idempotency key: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":     false,
				"reason": "internal_error",
			})
		}
		if record != nil {
			if record.RequestHash != "" && record.RequestHash != requestHash {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"ok":     false,
					"reason": "idempotency_conflict",
				})
			}
			if idempotency.IsCompleted(record) {
				return c.JSON(record.Response)
			}
			if idempotency.IsProcessing(record) {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"ok":     false,
					"reason": "idempotency_in_progress",
				})
			}
		}

		locked, err := h.idempotencyMgr.Begin(ctx, idempotencyKey, requestHash)
		if err != nil {
			logrus.Errorf("Failed to reserve idempotency key: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":     false,
				"reason": "internal_error",
			})
		}
		if !locked {
			record, err := h.idempotencyMgr.Get(ctx, idempotencyKey)
			if err != nil {
				logrus.Errorf("Failed to reload idempotency key: %v", err)
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"ok":     false,
					"reason": "internal_error",
				})
			}
			if record != nil && record.RequestHash != "" && record.RequestHash != requestHash {
				return c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"ok":     false,
					"reason": "idempotency_conflict",
				})
			}
			if idempotency.IsCompleted(record) {
				return c.JSON(record.Response)
			}
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"ok":     false,
				"reason": "idempotency_in_progress",
			})
		}
		reservedIdempotency = true
	}

	if reservedIdempotency {
		defer func() {
			if !completedIdempotency {
				if err := h.idempotencyMgr.Delete(ctx, idempotencyKey); err != nil {
					logrus.Warnf("Failed to cleanup idempotency key: %v", err)
				}
			}
		}()
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
		metrics.RecordRateLimitHit("user")
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
		metrics.RecordRateLimitHit("ip")
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
		metrics.RecordRateLimitHit("destination")
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
		metrics.RecordRateLimitHit("resend_cooldown")
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "resend_cooldown",
		})
	}

	// Check if user is locked
	if h.challengeManager.IsUserLocked(ctx, req.UserID) {
		metrics.RecordRateLimitHit("user_locked")
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
		metrics.RecordChallenge(req.Channel, req.Purpose, "error")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Store code in test mode (for integration testing only)
	if config.TestMode {
		testCodeKey := fmt.Sprintf("otp:test:code:%s", ch.ID)
		if err := h.redis.Set(ctx, testCodeKey, code, config.ChallengeExpiry).Err(); err != nil {
			logrus.Warnf("Failed to store test code: %v", err)
		}
	}

	// Send verification code via provider
	channel := provider.Channel(req.Channel)
	traceparent := middleware.GetTraceparent(c)
	tracestate := middleware.GetTracestate(c)

	msg := &provider.Message{
		To:          req.Destination,
		Code:        code,
		Purpose:     req.Purpose,
		Locale:      req.Locale,
		Traceparent: traceparent,
		Tracestate:  tracestate,
	}
	if idempotencyKey != "" {
		msg.IdempotencyKey = idempotencyKey
	} else {
		msg.IdempotencyKey = ch.ID
	}

	if channel == provider.ChannelEmail {
		msg.Subject, msg.Body = provider.FormatVerificationEmail(code, req.Locale)
	} else {
		msg.Body = provider.FormatVerificationSMS(code, req.Locale)
	}

	sendStart := time.Now()
	sendErr := h.providerRegistry.Send(ctx, channel, msg)
	sendDuration := time.Since(sendStart)
	sendResult := "ok"
	if sendErr != nil {
		sendResult = "failed"
	}
	metrics.RecordSend(req.Channel, sendResult)
	metrics.ObserveSendDuration(req.Channel, sendResult, sendDuration)

	if sendErr != nil {
		logrus.Errorf("Failed to send verification code via provider: %v", sendErr)
		metrics.RecordChallenge(req.Channel, req.Purpose, "send_failed")

		// Audit log: send failed
		if h.auditLogger != nil {
			h.auditLogger.Log(audit.Event{
				Event:       "challenge_send_failed",
				ChallengeID: ch.ID,
				UserID:      req.UserID,
				Channel:     req.Channel,
				Destination: req.Destination,
				Provider:    string(channel),
				Result:      "failed",
				Reason:      sendErr.Error(),
				ClientIP:    clientIP,
				Traceparent: traceparent,
				Tracestate:  tracestate,
			})
		}

		if config.IsProviderFailureStrict() {
			if revokeErr := h.challengeManager.RevokeChallenge(ctx, ch.ID); revokeErr != nil {
				logrus.Warnf("Failed to revoke challenge after send error: %v", revokeErr)
			}
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"ok":     false,
				"reason": "send_failed",
			})
		}
	} else {
		metrics.RecordChallenge(req.Channel, req.Purpose, "ok")

		// Audit log: challenge created and sent
		if h.auditLogger != nil {
			h.auditLogger.Log(audit.Event{
				Event:       "challenge_created",
				ChallengeID: ch.ID,
				UserID:      req.UserID,
				Channel:     req.Channel,
				Destination: req.Destination,
				Provider:    string(channel),
				Result:      "ok",
				ClientIP:    clientIP,
				Traceparent: traceparent,
				Tracestate:  tracestate,
			})
		}
	}

	// Return response
	response := idempotency.Response{
		ChallengeID:  ch.ID,
		ExpiresIn:    int(config.ChallengeExpiry.Seconds()),
		NextResendIn: int(config.ResendCooldown.Seconds()),
	}

	if idempotencyKey != "" {
		if err := h.idempotencyMgr.Complete(ctx, idempotencyKey, requestHash, response); err != nil {
			logrus.Warnf("Failed to store idempotency response: %v", err)
		} else {
			completedIdempotency = true
		}
	}

	return c.JSON(response)
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

	// Get trace context
	traceparent := middleware.GetTraceparent(c)
	tracestate := middleware.GetTracestate(c)
	clientIP := req.ClientIP
	if clientIP == "" {
		clientIP = c.IP()
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

		metrics.RecordVerification("failed", reason)

		// Audit log: verification failed
		if h.auditLogger != nil {
			h.auditLogger.Log(audit.Event{
				Event:       "challenge_verify_failed",
				ChallengeID: req.ChallengeID,
				Result:      "failed",
				Reason:      reason,
				ClientIP:    clientIP,
				Traceparent: traceparent,
				Tracestate:  tracestate,
			})
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": reason,
		})
	}

	if !valid {
		metrics.RecordVerification("failed", "invalid")

		// Audit log: verification failed (invalid code)
		if h.auditLogger != nil {
			h.auditLogger.Log(audit.Event{
				Event:       "challenge_verify_failed",
				ChallengeID: req.ChallengeID,
				Result:      "failed",
				Reason:      "invalid",
				ClientIP:    clientIP,
				Traceparent: traceparent,
				Tracestate:  tracestate,
			})
		}

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid",
		})
	}

	// Success
	metrics.RecordVerification("ok", "")

	// Audit log: verification succeeded
	if h.auditLogger != nil {
		h.auditLogger.Log(audit.Event{
			Event:       "challenge_verified",
			ChallengeID: req.ChallengeID,
			UserID:      ch.UserID,
			Channel:     ch.Channel,
			Result:      "ok",
			ClientIP:    clientIP,
			Traceparent: traceparent,
			Tracestate:  tracestate,
		})
	}

	return c.JSON(fiber.Map{
		"ok":        true,
		"user_id":   ch.UserID,
		"amr":       []string{"otp"},
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

	traceparent := middleware.GetTraceparent(c)
	tracestate := middleware.GetTracestate(c)
	clientIP := c.IP()

	if err := h.challengeManager.RevokeChallenge(ctx, challengeID); err != nil {
		// Audit log: revoke failed
		if h.auditLogger != nil {
			h.auditLogger.Log(audit.Event{
				Event:       "challenge_revoke_failed",
				ChallengeID: challengeID,
				Result:      "failed",
				Reason:      err.Error(),
				ClientIP:    clientIP,
				Traceparent: traceparent,
				Tracestate:  tracestate,
			})
		}

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Audit log: revoke succeeded
	if h.auditLogger != nil {
		h.auditLogger.Log(audit.Event{
			Event:       "challenge_revoked",
			ChallengeID: challengeID,
			Result:      "ok",
			ClientIP:    clientIP,
			Traceparent: traceparent,
			Tracestate:  tracestate,
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
	testCodeKey := fmt.Sprintf("otp:test:code:%s", challengeID)
	code, err := h.redis.Get(ctx, testCodeKey).Result()
	if err != nil {
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

func getIdempotencyKey(c *fiber.Ctx) string {
	key := strings.TrimSpace(c.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(c.Get("X-Idempotency-Key"))
	}
	return key
}

func hashIdempotencyRequest(req CreateChallengeRequest) string {
	data := strings.Join([]string{
		strings.TrimSpace(req.UserID),
		strings.TrimSpace(req.Channel),
		strings.TrimSpace(req.Destination),
		strings.TrimSpace(req.Purpose),
		strings.TrimSpace(req.Locale),
	}, "|")
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}
