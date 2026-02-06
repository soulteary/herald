package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	logger "github.com/soulteary/logger-kit"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit"
	"go.opentelemetry.io/otel/attribute"

	provider "github.com/soulteary/provider-kit"
	"github.com/soulteary/tracing-kit"

	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/auditlog"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/ratelimit"
	"github.com/soulteary/herald/internal/template"
	sessionkit "github.com/soulteary/session-kit"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	challengeManager challengekit.ManagerInterface
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	templateManager  *template.Manager
	redis            *redis.Client
	testCodeCache    rediskitcache.Cache   // For test mode code storage
	idempotencyCache rediskitcache.Cache   // For idempotency key storage
	sessionManager   *sessionkit.KVManager // Optional: nil if session storage is disabled
	log              *logger.Logger
}

// StopAuditWriter stops the audit writer gracefully
func (h *Handlers) StopAuditWriter() error {
	return auditlog.Stop()
}

// NewHandlers creates a new handlers instance
func NewHandlers(redisClient *redis.Client, sessionManager *sessionkit.KVManager, log *logger.Logger) *Handlers {
	challengeConfig := challengekit.Config{
		Expiry:             config.ChallengeExpiry,
		MaxAttempts:        config.MaxAttempts,
		LockoutDuration:    config.LockoutDuration,
		CodeLength:         config.CodeLength,
		ChallengeKeyPrefix: "otp:ch:",
		LockKeyPrefix:      "otp:lock:",
	}
	challengeMgr := challengekit.NewManager(redisClient, challengeConfig)

	rateLimitMgr := ratelimit.NewManager(redisClient)

	// Initialize audit logger with Redis client
	auditlog.Init(redisClient)

	// Initialize template manager
	templateMgr := template.NewManager(config.TemplateDir)

	// Initialize provider registry (using provider-kit)
	registry := provider.NewRegistry()

	// Register email channel: herald-smtp HTTP provider takes precedence over built-in SMTP
	if config.HeraldSMTPAPIURL != "" {
		httpConfig := &provider.HTTPConfig{
			BaseURL:      config.HeraldSMTPAPIURL,
			SendEndpoint: "/v1/send",
			APIKey:       config.HeraldSMTPAPIKey,
			ChannelType:  provider.ChannelEmail,
			ProviderName: "smtp",
		}
		httpProvider, err := provider.NewHTTPProvider(httpConfig)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create herald-smtp HTTP provider")
		} else if err := registry.Register(httpProvider); err != nil {
			log.Warn().Err(err).Msg("Failed to register herald-smtp HTTP provider")
		} else {
			log.Info().Msg("Email HTTP provider registered (herald-smtp)")
		}
	} else if config.SMTPHost != "" {
		// Built-in SMTP provider when herald-smtp URL is not set
		smtpConfig := &provider.SMTPConfig{
			Host:        config.SMTPHost,
			Port:        config.SMTPPort,
			Username:    config.SMTPUser,
			Password:    config.SMTPPassword,
			From:        config.SMTPFrom,
			UseStartTLS: true,
		}
		smtpProvider, err := provider.NewSMTPProvider(smtpConfig)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create SMTP provider")
		} else if err := registry.Register(smtpProvider); err != nil {
			log.Warn().Err(err).Msg("Failed to register SMTP provider")
		} else {
			log.Info().Msg("SMTP provider registered")
		}
	}

	// Register HTTP SMS provider if configured (using HTTP API for SMS delivery)
	if config.SMSProvider != "" {
		httpConfig := &provider.HTTPConfig{
			BaseURL:      config.SMSAPIBaseURL,
			SendEndpoint: "/v1/send",
			APIKey:       config.SMSAPIKey,
			ChannelType:  provider.ChannelSMS,
			ProviderName: config.SMSProvider,
		}
		httpProvider, err := provider.NewHTTPProvider(httpConfig)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create HTTP SMS provider")
		} else if err := registry.Register(httpProvider); err != nil {
			log.Warn().Err(err).Msg("Failed to register HTTP SMS provider")
		} else {
			log.Info().Str("provider", config.SMSProvider).Msg("HTTP SMS provider registered")
		}
	}

	// Register DingTalk channel via herald-dingtalk HTTP service (no DingTalk credentials in Herald)
	if config.HeraldDingtalkAPIURL != "" {
		httpConfig := &provider.HTTPConfig{
			BaseURL:      config.HeraldDingtalkAPIURL,
			SendEndpoint: "/v1/send",
			APIKey:       config.HeraldDingtalkAPIKey,
			ChannelType:  provider.ChannelDingTalk,
			ProviderName: "dingtalk",
		}
		httpProvider, err := provider.NewHTTPProvider(httpConfig)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create DingTalk HTTP provider")
		} else if err := registry.Register(httpProvider); err != nil {
			log.Warn().Err(err).Msg("Failed to register DingTalk HTTP provider")
		} else {
			log.Info().Msg("DingTalk HTTP provider registered (herald-dingtalk)")
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
		templateManager:  templateMgr,
		redis:            redisClient,
		testCodeCache:    testCodeCache,
		idempotencyCache: idempotencyCache,
		sessionManager:   sessionManager,
		log:              log,
	}
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

	if req.Channel != "sms" && req.Channel != "email" && req.Channel != "dingtalk" {
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
		h.log.Error().Err(err).Msg("Rate limit check failed")
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
		h.log.Error().Err(err).Msg("Rate limit check failed")
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
		h.log.Error().Err(err).Msg("Rate limit check failed")
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
		h.log.Error().Err(err).Msg("Cooldown check failed")
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
	createReq := challengekit.CreateRequest{
		UserID:      req.UserID,
		Channel:     challengekit.Channel(req.Channel),
		Destination: req.Destination,
		Purpose:     req.Purpose,
		ClientIP:    clientIP,
	}
	ch, code, err := h.challengeManager.Create(spanCtx, createReq)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.Error().Err(err).Msg("Failed to create challenge")
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
	auditlog.LogChallengeCreated(spanCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, clientIP)

	// Metrics: challenge created
	metrics.RecordChallengeCreated(req.Channel, req.Purpose, "success")

	// Store code in test mode (for integration testing only)
	if config.TestMode {
		if err := h.testCodeCache.Set(spanCtx, ch.ID, code, config.ChallengeExpiry); err != nil {
			h.log.Warn().Err(err).Msg("Failed to store test code")
		}
	}

	// Send verification code via provider
	channel := provider.Channel(req.Channel)

	// Use template manager to format message
	templateData := template.TemplateData{
		Code:      code,
		ExpiresIn: int(config.ChallengeExpiry.Seconds()),
		Purpose:   req.Purpose,
		Locale:    req.Locale,
	}

	// Build message using provider-kit fluent API
	msg := provider.NewMessage(req.Destination).
		WithCode(code).
		WithLocale(req.Locale).
		WithIdempotencyKey(ch.ID) // Use challenge ID as idempotency key

	if channel == provider.ChannelEmail {
		subject, body, err := h.templateManager.RenderEmail(req.Locale, req.Purpose, templateData)
		if err != nil {
			// Fallback to built-in formatting from provider-kit
			subject, body = provider.FormatVerificationEmail(code, req.Locale)
		}
		msg.WithSubject(subject).WithBody(body)
	} else {
		// SMS and DingTalk: body only (DingTalk via herald-dingtalk receives body)
		body, err := h.templateManager.RenderSMS(req.Locale, req.Purpose, templateData)
		if err != nil {
			// Fallback to built-in formatting from provider-kit
			body = provider.FormatVerificationSMS(code, req.Locale)
		}
		msg.WithBody(body)
	}

	// Determine provider name for audit
	var providerName string
	switch req.Channel {
	case "email":
		providerName = "smtp"
	case "sms":
		providerName = config.SMSProvider
	case "dingtalk":
		providerName = "dingtalk"
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

	// Send using provider-kit Registry (returns *SendResult, error)
	sendResult, err := h.providerRegistry.Send(providerCtx, channel, msg)
	sendDuration := time.Since(sendStart)

	if err != nil || (sendResult != nil && !sendResult.OK) {
		tracing.RecordError(providerSpan, err)
		providerSpan.End()
		h.log.Error().Err(err).Msg("Failed to send verification code via provider")

		// Get error reason from provider-kit result
		errorReason := "send_failed"
		if sendResult != nil && sendResult.Error != nil {
			errorReason = string(sendResult.Error.Reason)
		}

		// Metrics: send failed
		metrics.RecordOTPSend(req.Channel, providerName, "failure", sendDuration)

		// Audit: send failed
		auditlog.LogSendFailed(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, errorReason, clientIP)

		// Handle provider failure based on policy
		if config.ProviderFailurePolicy == "strict" {
			// Strict mode: revoke challenge and return error
			_ = h.challengeManager.Revoke(spanCtx, ch.ID)
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
		providerSpan.SetAttributes(
			attribute.String("result", "success"),
			attribute.Int64("duration_ms", sendDuration.Milliseconds()),
		)
		providerSpan.End()

		// Metrics: send success
		metrics.RecordOTPSend(req.Channel, providerName, "success", sendDuration)

		// Audit: send success (now includes messageID from provider-kit)
		messageID := ""
		if sendResult != nil {
			messageID = sendResult.MessageID
		}
		auditlog.LogSendSuccess(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, messageID, clientIP)
	}

	// Prepare response
	response := fiber.Map{
		"challenge_id":   ch.ID,
		"expires_in":     int(config.ChallengeExpiry.Seconds()),
		"next_resend_in": int(config.ResendCooldown.Seconds()),
	}
	if config.TestMode {
		response["debug_code"] = code
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
			h.log.Warn().Err(err).Msg("Failed to store idempotency record")
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
	// Mask email: show first 2 chars and domain
	if strings.Contains(dest, "@") {
		return secure.MaskEmailPartial(dest)
	}
	// Mask phone: use secure-kit MaskString for generic masking
	return secure.MaskString(dest, 3)
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
	if !challengekit.ValidateCodeFormat(req.Code, config.CodeLength) {
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
	result, err := h.challengeManager.Verify(verifyCtx, req.ChallengeID, req.Code, req.ClientIP)
	if err != nil || !result.OK {
		reason := "verification_failed"
		if result != nil && result.Reason != "" {
			reason = result.Reason
		} else if err != nil {
			errStr := err.Error()
			if contains(errStr, "expired") {
				reason = "expired"
			} else if contains(errStr, "locked") {
				reason = "locked"
			} else if contains(errStr, "invalid") {
				reason = "invalid"
			}
		}

		tracing.RecordError(verifySpan, err)
		h.log.Debug().Err(err).Msg("Challenge verification failed")

		// Set span attributes for failure
		verifySpan.SetAttributes(
			attribute.String("result", "failure"),
			attribute.String("reason", reason),
		)

		// Metrics: verification failed
		metrics.RecordVerification("failure", reason)

		// Audit: verification failed
		auditlog.LogVerificationFailed(verifyCtx, req.ChallengeID, reason, req.ClientIP)

		response := fiber.Map{
			"ok":     false,
			"reason": reason,
		}
		if result != nil && result.RemainingAttempts != nil {
			response["remaining_attempts"] = *result.RemainingAttempts
		}

		return c.Status(fiber.StatusUnauthorized).JSON(response)
	}

	// Set span attributes for success
	ch := result.Challenge
	verifySpan.SetAttributes(
		attribute.String("result", "success"),
		attribute.String("user_id", ch.UserID),
		attribute.String("channel", string(ch.Channel)),
		attribute.String("purpose", ch.Purpose),
	)

	// Metrics: verification success
	metrics.RecordVerification("success", "")

	// Audit: challenge verified
	auditlog.LogVerificationSuccess(verifyCtx, ch.ID, ch.UserID, string(ch.Channel), ch.Destination, ch.Purpose, req.ClientIP)

	// Generate AMR based on channel (use string to avoid depending on challengekit.ChannelDingTalk in v1.0.0)
	amr := []string{"otp"}
	switch string(ch.Channel) {
	case "sms":
		amr = append(amr, "sms")
	case "email":
		amr = append(amr, "email")
	case "dingtalk":
		amr = append(amr, "dingtalk")
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

	if err := h.challengeManager.Revoke(spanCtx, challengeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}

	// Audit: challenge revoked
	auditlog.LogChallengeRevoked(spanCtx, challengeID, c.IP())

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
