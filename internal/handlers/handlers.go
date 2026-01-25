package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	rediskitclient "github.com/soulteary/redis-kit/client"
	"go.opentelemetry.io/otel/attribute"

	"github.com/soulteary/herald/internal/audit"
	"github.com/soulteary/herald/internal/audit/storage"
	"github.com/soulteary/herald/internal/audit/types"
	"github.com/soulteary/herald/internal/challenge"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/otp"
	"github.com/soulteary/herald/internal/provider"
	"github.com/soulteary/herald/internal/ratelimit"
	"github.com/soulteary/herald/internal/session"
	"github.com/soulteary/herald/internal/template"
	"github.com/soulteary/tracing-kit"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	challengeManager *challenge.Manager
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	auditManager     *audit.Manager
	templateManager  *template.Manager
	redis            *redis.Client
	testCodeCache    rediskitcache.Cache // For test mode code storage
	idempotencyCache rediskitcache.Cache // For idempotency key storage
	sessionManager   *session.Manager    // Optional: nil if session storage is disabled
}

// StopAuditWriter stops the audit writer gracefully
func (h *Handlers) StopAuditWriter() error {
	if h.auditManager != nil {
		return h.auditManager.Stop()
	}
	return nil
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

	// Initialize audit manager
	// Initialize audit manager with persistent storage if configured
	var auditMgr *audit.Manager
	persistentStorage, err := storage.NewStorageFromConfig()
	if err != nil {
		logrus.Warnf("Failed to initialize persistent audit storage: %v, using Redis only", err)
		auditMgr = audit.NewManager(redisClient)
	} else if persistentStorage != nil {
		logrus.Info("Persistent audit storage enabled")
		auditMgr = audit.NewManagerWithStorage(
			redisClient,
			persistentStorage,
			config.AuditWriterQueueSize,
			config.AuditWriterWorkers,
		)
	} else {
		auditMgr = audit.NewManager(redisClient)
	}

	// Initialize template manager
	templateMgr := template.NewManager(config.TemplateDir)

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

	// Create idempotency cache
	idempotencyCache := rediskitcache.NewCache(redisClient, "otp:idem:")

	return &Handlers{
		challengeManager: challengeMgr,
		rateLimitManager: rateLimitMgr,
		providerRegistry: registry,
		auditManager:     auditMgr,
		templateManager:  templateMgr,
		redis:            redisClient,
		testCodeCache:    testCodeCache,
		idempotencyCache: idempotencyCache,
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

// IdempotencyRecord represents a cached idempotency response
type IdempotencyRecord struct {
	ChallengeID  string `json:"challenge_id"`
	ExpiresIn    int    `json:"expires_in"`
	NextResendIn int    `json:"next_resend_in"`
	CreatedAt    int64  `json:"created_at"`
}

// CreateChallenge handles challenge creation
func (h *Handlers) CreateChallenge(c *fiber.Ctx) error {
	// Get trace context from middleware
	ctx := c.Locals("trace_context")
	if ctx == nil {
		ctx = c.Context()
	}
	traceCtx := ctx.(context.Context)

	// Start span for challenge creation
	spanCtx, span := tracing.StartSpan(traceCtx, "otp.challenge.create")
	defer span.End()

	// Check for idempotency key
	idempotencyKey := c.Get("Idempotency-Key")
	if idempotencyKey != "" {
		var cachedRecord IdempotencyRecord
		if err := h.idempotencyCache.Get(spanCtx, idempotencyKey, &cachedRecord); err == nil {
			// Return cached response
			return c.JSON(fiber.Map{
				"challenge_id":   cachedRecord.ChallengeID,
				"expires_in":     cachedRecord.ExpiresIn,
				"next_resend_in": cachedRecord.NextResendIn,
			})
		}
	}

	var req CreateChallengeRequest
	if err := c.BodyParser(&req); err != nil {
		tracing.RecordError(span, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}

	// Set span attributes
	span.SetAttributes(
		attribute.String("channel", req.Channel),
		attribute.String("purpose", req.Purpose),
		attribute.String("user_id", req.UserID),
		attribute.String("destination", maskDestination(req.Destination)),
	)

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

	// Validate purpose
	if req.Purpose == "" {
		req.Purpose = "login" // Default purpose
	}
	purposeValid := false
	for _, allowed := range config.AllowedPurposes {
		if allowed == req.Purpose {
			purposeValid = true
			break
		}
	}
	if !purposeValid {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_purpose",
			"error":  fmt.Sprintf("Purpose must be one of: %s", strings.Join(config.AllowedPurposes, ", ")),
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
		spanCtx, req.UserID, config.RateLimitPerUser, time.Hour,
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
		spanCtx, clientIP, config.RateLimitPerIP, time.Minute,
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
		spanCtx, req.Destination, config.RateLimitPerDestination, time.Hour,
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
	allowed, _, err = h.rateLimitManager.CheckResendCooldown(spanCtx, cooldownKey, config.ResendCooldown)
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
	if h.challengeManager.IsUserLocked(spanCtx, req.UserID) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"ok":     false,
			"reason": "user_locked",
		})
	}

	// Create challenge
	ch, code, err := h.challengeManager.CreateChallenge(
		spanCtx, req.UserID, req.Channel, req.Destination, req.Purpose, clientIP,
	)
	if err != nil {
		tracing.RecordError(span, err)
		logrus.Errorf("Failed to create challenge: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Update span with challenge ID
	span.SetAttributes(attribute.String("challenge_id", ch.ID))

	// Update span with result
	span.SetAttributes(attribute.String("result", "success"))

	// Audit: challenge created
	h.auditManager.Log(spanCtx, &types.AuditRecord{
		EventType:   types.EventChallengeCreated,
		ChallengeID: ch.ID,
		UserID:      req.UserID,
		Channel:     req.Channel,
		Destination: req.Destination,
		Purpose:     req.Purpose,
		Result:      "success",
		IP:          clientIP,
	})

	// Metrics: challenge created
	metrics.RecordChallengeCreated(req.Channel, req.Purpose, "success")

	// Store code in test mode (for integration testing only)
	if config.TestMode {
		if err := h.testCodeCache.Set(spanCtx, ch.ID, code, config.ChallengeExpiry); err != nil {
			logrus.Warnf("Failed to store test code: %v", err)
		}
	}

	// Send verification code via provider
	channel := provider.Channel(req.Channel)
	msg := &provider.Message{
		To:   req.Destination,
		Code: code,
	}

	// Use template manager to format message
	templateData := template.TemplateData{
		Code:      code,
		ExpiresIn: int(config.ChallengeExpiry.Seconds()),
		Purpose:   req.Purpose,
		Locale:    req.Locale,
	}

	if channel == provider.ChannelEmail {
		subject, body, err := h.templateManager.RenderEmail(req.Locale, req.Purpose, templateData)
		if err != nil {
			// Fallback to built-in formatting
			msg.Subject, msg.Body = provider.FormatVerificationEmail(code, req.Locale)
		} else {
			msg.Subject = subject
			msg.Body = body
		}
	} else {
		body, err := h.templateManager.RenderSMS(req.Locale, req.Purpose, templateData)
		if err != nil {
			// Fallback to built-in formatting
			msg.Body = provider.FormatVerificationSMS(code, req.Locale)
		} else {
			msg.Body = body
		}
	}

	// Determine provider name for audit
	var providerName string
	switch req.Channel {
	case "email":
		providerName = "smtp"
	case "sms":
		providerName = config.SMSProvider
	default:
		providerName = req.Channel
	}

	// Record send duration
	sendStart := time.Now()

	// Start span for provider send
	providerCtx, providerSpan := tracing.StartSpan(spanCtx, "otp.provider.send")
	providerSpan.SetAttributes(
		attribute.String("channel", req.Channel),
		attribute.String("provider", providerName),
	)

	if err := h.providerRegistry.Send(providerCtx, channel, msg); err != nil {
		tracing.RecordError(providerSpan, err)
		providerSpan.End()
		sendDuration := time.Since(sendStart)
		logrus.Errorf("Failed to send verification code via provider: %v", err)

		// Metrics: send failed
		metrics.RecordOTPSend(req.Channel, providerName, "failure", sendDuration)

		// Audit: send failed
		h.auditManager.Log(providerCtx, &types.AuditRecord{
			EventType:   types.EventSendFailed,
			ChallengeID: ch.ID,
			UserID:      req.UserID,
			Channel:     req.Channel,
			Destination: req.Destination,
			Purpose:     req.Purpose,
			Result:      "failure",
			Reason:      "send_failed",
			Provider:    providerName,
			IP:          clientIP,
		})

		// Handle provider failure based on policy
		if config.ProviderFailurePolicy == "strict" {
			// Strict mode: revoke challenge and return error
			_ = h.challengeManager.RevokeChallenge(spanCtx, ch.ID)
			// Also remove idempotency record if it was stored
			if idempotencyKey != "" {
				_ = h.idempotencyCache.Del(spanCtx, idempotencyKey)
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":     false,
				"reason": "send_failed",
				"error":  "Failed to send verification code",
			})
		}
		// Soft mode: log error but continue (challenge is already created)
		// The code can still be verified manually if needed
	} else {
		sendDuration := time.Since(sendStart)
		providerSpan.SetAttributes(
			attribute.String("result", "success"),
			attribute.Int64("duration_ms", sendDuration.Milliseconds()),
		)
		providerSpan.End()

		// Metrics: send success
		metrics.RecordOTPSend(req.Channel, providerName, "success", sendDuration)

		// Audit: send success
		h.auditManager.Log(providerCtx, &types.AuditRecord{
			EventType:   types.EventSendSuccess,
			ChallengeID: ch.ID,
			UserID:      req.UserID,
			Channel:     req.Channel,
			Destination: req.Destination,
			Purpose:     req.Purpose,
			Result:      "success",
			Provider:    providerName,
			IP:          clientIP,
		})
	}

	// Prepare response
	response := fiber.Map{
		"challenge_id":   ch.ID,
		"expires_in":     int(config.ChallengeExpiry.Seconds()),
		"next_resend_in": int(config.ResendCooldown.Seconds()),
	}

	// Store idempotency record if idempotency key is provided
	if idempotencyKey != "" {
		idempotencyRecord := IdempotencyRecord{
			ChallengeID:  ch.ID,
			ExpiresIn:    int(config.ChallengeExpiry.Seconds()),
			NextResendIn: int(config.ResendCooldown.Seconds()),
			CreatedAt:    time.Now().Unix(),
		}
		if err := h.idempotencyCache.Set(spanCtx, idempotencyKey, idempotencyRecord, config.IdempotencyKeyTTL); err != nil {
			logrus.Warnf("Failed to store idempotency record: %v", err)
		}
	}

	// Return response
	return c.JSON(response)
}

// maskDestination masks sensitive destination information for tracing
func maskDestination(dest string) string {
	if len(dest) == 0 {
		return ""
	}
	if len(dest) <= 4 {
		return "***"
	}
	// Mask email: show first 2 chars and domain
	if strings.Contains(dest, "@") {
		parts := strings.Split(dest, "@")
		if len(parts) == 2 {
			if len(parts[0]) <= 2 {
				return "***@" + parts[1]
			}
			return parts[0][:2] + "***@" + parts[1]
		}
	}
	// Mask phone: show last 4 digits
	if len(dest) <= 4 {
		return "***"
	}
	return "***" + dest[len(dest)-4:]
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

	// Get trace context from middleware
	traceCtx := c.Locals("trace_context")
	if traceCtx == nil {
		traceCtx = ctx
	}
	spanCtx := traceCtx.(context.Context)

	// Start span for verification
	verifyCtx, verifySpan := tracing.StartSpan(spanCtx, "otp.verify")
	defer verifySpan.End()

	verifySpan.SetAttributes(attribute.String("challenge_id", req.ChallengeID))

	// Verify challenge
	valid, ch, err := h.challengeManager.VerifyChallenge(verifyCtx, req.ChallengeID, req.Code, req.ClientIP)
	if err != nil {
		tracing.RecordError(verifySpan, err)
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

		// Set span attributes for failure
		verifySpan.SetAttributes(
			attribute.String("result", "failure"),
			attribute.String("reason", reason),
		)

		// Metrics: verification failed
		metrics.RecordVerification("failure", reason)

		// Audit: verification failed
		h.auditManager.Log(verifyCtx, &types.AuditRecord{
			EventType:   types.EventVerificationFailed,
			ChallengeID: req.ChallengeID,
			Result:      "failure",
			Reason:      reason,
			IP:          req.ClientIP,
		})

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": reason,
		})
	}

	if !valid {
		// Set span attributes for failure
		verifySpan.SetAttributes(
			attribute.String("result", "failure"),
			attribute.String("reason", "invalid"),
		)

		// Metrics: verification failed
		metrics.RecordVerification("failure", "invalid")

		// Audit: verification failed
		h.auditManager.Log(verifyCtx, &types.AuditRecord{
			EventType:   types.EventVerificationFailed,
			ChallengeID: req.ChallengeID,
			Result:      "failure",
			Reason:      "invalid",
			IP:          req.ClientIP,
		})

		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid",
		})
	}

	// Set span attributes for success
	verifySpan.SetAttributes(
		attribute.String("result", "success"),
		attribute.String("user_id", ch.UserID),
		attribute.String("channel", string(ch.Channel)),
		attribute.String("purpose", ch.Purpose),
	)

	// Metrics: verification success
	metrics.RecordVerification("success", "")

	// Audit: challenge verified
	h.auditManager.Log(verifyCtx, &types.AuditRecord{
		EventType:   types.EventChallengeVerified,
		ChallengeID: ch.ID,
		UserID:      ch.UserID,
		Channel:     ch.Channel,
		Destination: ch.Destination,
		Purpose:     ch.Purpose,
		Result:      "success",
		IP:          req.ClientIP,
	})

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
	// Get trace context from middleware
	traceCtx := c.Locals("trace_context")
	if traceCtx == nil {
		traceCtx = c.Context()
	}
	spanCtx := traceCtx.(context.Context)

	challengeID := c.Params("id")

	if challengeID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "challenge_id_required",
		})
	}

	if err := h.challengeManager.RevokeChallenge(spanCtx, challengeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Audit: challenge revoked
	h.auditManager.Log(spanCtx, &types.AuditRecord{
		EventType:   types.EventChallengeRevoked,
		ChallengeID: challengeID,
		Result:      "success",
		IP:          c.IP(),
	})

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
