package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	logger "github.com/soulteary/logger-kit"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit"
	"go.opentelemetry.io/otel/attribute"

	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald-totp/pkg/heraldtotp"
	provider "github.com/soulteary/provider-kit"
	"github.com/soulteary/tracing-kit"

	"github.com/soulteary/herald/internal/auditlog"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/destination"
	"github.com/soulteary/herald/internal/idempotency"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/ratelimit"
	"github.com/soulteary/herald/internal/security"
	"github.com/soulteary/herald/internal/template"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	challengeManager challengekit.ManagerInterface
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	templateManager  *template.Manager
	redis            *redis.Client
	testCodeCache    rediskitcache.Cache // For test mode code storage
	idempotencyCache rediskitcache.Cache // For idempotency key storage (legacy replay cache)
	idempotencyStore *idempotency.Store  // Atomic, principal-namespaced idempotency store
	totpClient       *heraldtotp.Client  // Optional: nil when TOTP is not enabled
	digester         *security.Digester  // Peppered digester for privacy-preserving keys
	log              *logger.Logger
}

// StopAuditWriter stops the audit writer gracefully
func (h *Handlers) StopAuditWriter() error {
	return auditlog.Stop()
}

// hardenedHTTPConfig returns a provider-kit HTTPConfig with the Phase 3
// transport hardening applied (explicit timeout, bounded response, redirect
// policy, and HTTPS-only in production).
func hardenedHTTPConfig(baseURL, apiKey string, channel provider.Channel, name string) *provider.HTTPConfig {
	redirect := provider.RedirectDeny
	if strings.EqualFold(config.ProviderRedirectPolicy, "same-origin") {
		redirect = provider.RedirectSameOrigin
	}
	return &provider.HTTPConfig{
		BaseURL:          baseURL,
		SendEndpoint:     "/v1/send",
		APIKey:           apiKey,
		ChannelType:      channel,
		ProviderName:     name,
		Timeout:          config.ProviderTimeout,
		MaxResponseBytes: int64(config.ProviderMaxResponseBytes),
		Redirect:         redirect,
		RequireHTTPS:     config.IsProduction(),
	}
}

// NewHandlers creates a new handlers instance
func NewHandlers(redisClient *redis.Client, log *logger.Logger) *Handlers {
	h, err := NewHandlersWithError(redisClient, log)
	if err != nil {
		// Backward-compatible constructor: in non-production this degrades to a
		// warning so local/dev flows are not blocked. Production callers should
		// use NewHandlersWithError and fail closed on error.
		log.Warn().Err(err).Msg("Handler initialization reported a provider error (continuing; use NewHandlersWithError to fail closed)")
	}
	return h
}

// NewHandlersWithError creates a new handlers instance and returns any provider
// registration/configuration error so callers can fail closed. A provider that
// is configured but cannot be constructed/registered is treated as a
// configuration failure rather than silently disabling that channel.
func NewHandlersWithError(redisClient *redis.Client, log *logger.Logger) (*Handlers, error) {
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

	// Initialize audit logger with Redis client. In production a broken audit
	// backend is a hard startup error rather than a silent downgrade to no-op.
	if err := auditlog.InitWithError(redisClient); err != nil {
		return nil, err
	}

	// Initialize template manager
	templateMgr := template.NewManager(config.TemplateDir)

	// Initialize provider registry (using provider-kit)
	registry := provider.NewRegistry()

	// Register email channel: herald-smtp HTTP provider takes precedence over built-in SMTP
	if config.HeraldSMTPAPIURL != "" {
		httpProvider, err := provider.NewHTTPProvider(hardenedHTTPConfig(config.HeraldSMTPAPIURL, config.HeraldSMTPAPIKey, provider.ChannelEmail, "smtp"))
		if err != nil {
			return nil, fmt.Errorf("herald-smtp provider config invalid: %w", err)
		}
		if err := registry.Register(httpProvider); err != nil {
			return nil, fmt.Errorf("failed to register herald-smtp provider: %w", err)
		}
		log.Info().Msg("Email HTTP provider registered (herald-smtp)")
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
			return nil, fmt.Errorf("SMTP provider config invalid: %w", err)
		}
		if err := registry.Register(smtpProvider); err != nil {
			return nil, fmt.Errorf("failed to register SMTP provider: %w", err)
		}
		log.Info().Msg("SMTP provider registered")
	}

	// Register HTTP SMS provider if configured (using HTTP API for SMS delivery)
	if config.SMSProvider != "" {
		httpProvider, err := provider.NewHTTPProvider(hardenedHTTPConfig(config.SMSAPIBaseURL, config.SMSAPIKey, provider.ChannelSMS, config.SMSProvider))
		if err != nil {
			return nil, fmt.Errorf("SMS provider config invalid: %w", err)
		}
		if err := registry.Register(httpProvider); err != nil {
			return nil, fmt.Errorf("failed to register SMS provider: %w", err)
		}
		log.Info().Str("provider", config.SMSProvider).Msg("HTTP SMS provider registered")
	}

	// Register DingTalk channel via herald-dingtalk HTTP service (no DingTalk credentials in Herald)
	if config.HeraldDingtalkAPIURL != "" {
		httpProvider, err := provider.NewHTTPProvider(hardenedHTTPConfig(config.HeraldDingtalkAPIURL, config.HeraldDingtalkAPIKey, provider.ChannelDingTalk, "dingtalk"))
		if err != nil {
			return nil, fmt.Errorf("herald-dingtalk provider config invalid: %w", err)
		}
		if err := registry.Register(httpProvider); err != nil {
			return nil, fmt.Errorf("failed to register herald-dingtalk provider: %w", err)
		}
		log.Info().Msg("DingTalk HTTP provider registered (herald-dingtalk)")
	}

	// Create test code cache for test mode
	testCodeCache := rediskitcache.NewCache(redisClient, "otp:test:code:")

	// Create idempotency cache
	idempotencyCache := rediskitcache.NewCache(redisClient, "otp:idem:")

	// Atomic idempotency store (Phase 3): principal-namespaced, fingerprinted,
	// SET NX pending placeholder.
	idempotencyStore := idempotency.NewStore(redisClient, "otp:idem2:", []byte(config.IdempotencySecret), config.IdempotencyKeyTTL)

	// TOTP client: when Herald proxies TOTP to herald-totp
	var totpClient *heraldtotp.Client
	if config.TOTPEnabled && config.TOTPBaseURL != "" {
		opts := heraldtotp.DefaultOptions().
			WithBaseURL(strings.TrimSuffix(config.TOTPBaseURL, "/")).
			WithAPIKey(config.TOTPAPIKey).
			WithTimeout(10 * time.Second)
		if config.TOTPHMACSecret != "" {
			opts = opts.WithHMACSecret(config.TOTPHMACSecret)
		}
		if c, err := heraldtotp.NewClient(opts); err != nil {
			if config.IsProduction() {
				return nil, fmt.Errorf("herald-totp client init failed: %w", err)
			}
			log.Warn().Err(err).Msg("Failed to create herald-totp client, TOTP proxy will be disabled")
		} else {
			totpClient = c
			log.Info().Msg("TOTP proxy enabled (herald-totp)")
		}
	}

	return &Handlers{
		challengeManager: challengeMgr,
		rateLimitManager: rateLimitMgr,
		providerRegistry: registry,
		templateManager:  templateMgr,
		redis:            redisClient,
		testCodeCache:    testCodeCache,
		idempotencyCache: idempotencyCache,
		idempotencyStore: idempotencyStore,
		totpClient:       totpClient,
		digester:         security.NewDigester([]byte(config.PIIPepper)),
		log:              log,
	}, nil
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

	// Check for idempotency key. The atomic store claims a pending slot before
	// any side effects so concurrent duplicates collapse to a single provider
	// send, and a reused key with a different body is rejected. The claim
	// happens after body parse + validation so we can fingerprint the request.
	idempotencyKey := c.Get("Idempotency-Key")

	var req CreateChallengeRequest
	if err := parseStrictJSON(c, &req); err != nil {
		tracing.RecordError(span, err)
		return writeStrictBodyError(c, err)
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

	// Validate + normalize the destination once. The normalized value is used
	// for the provider call, rate-limit keys, and idempotency fingerprint so
	// trivial variants (case/spacing/separators) cannot bypass dedup or limits.
	normDest, derr := destination.Validate(req.Channel, req.Destination)
	if derr != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_destination",
		})
	}
	req.Destination = normDest

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

	// Client IP trust split (Phase 6):
	//   - peerIP is the transport-level source address (or the leftmost trusted
	//     forwarded hop, per Fiber's ProxyHeader/TrustedProxies config). This is
	//     the ONLY value used for IP rate limiting so a caller cannot evade the
	//     per-IP limit by rotating a self-reported field.
	//   - reportedClientIP is advisory (supplied by the trusted upstream service
	//     in the request body) and is only recorded in audit logs.
	peerIP := c.IP()
	reportedClientIP := req.ClientIP
	// The reported client IP is advisory only. Record a peppered digest at debug
	// level for troubleshooting mismatches without trusting or exposing it.
	if reportedClientIP != "" && reportedClientIP != peerIP {
		h.log.Debug().
			Str("reported_client_ip_digest", h.digester.Digest(reportedClientIP)).
			Msg("reported client_ip differs from trusted peer IP (advisory only)")
	}

	// Atomic idempotency claim (Phase 3). Principal = authenticated service +
	// key id when present, else the client IP, so keys are namespaced per
	// caller. On a duplicate success we replay the stored response; on a reused
	// key with a different body we return 409; on a concurrent in-flight request
	// we return 409 (retryable); on backend failure we fail closed.
	principal := idempotencyPrincipal(c)
	fingerprint := idempotency.Fingerprint(
		req.UserID, strings.ToLower(req.Channel), destination.Normalize(req.Channel, req.Destination), req.Purpose, req.Locale,
	)
	if idempotencyKey != "" {
		replay, owned, err := h.idempotencyStore.Begin(spanCtx, principal, idempotencyKey, fingerprint)
		switch {
		case err == nil && !owned && replay != nil:
			metrics.RecordIdempotency("replay")
			c.Set("Content-Type", "application/json")
			return c.Status(fiber.StatusOK).Send(replay)
		case err == idempotency.ErrConflict:
			metrics.RecordIdempotency("conflict")
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "idempotency_conflict"})
		case err == idempotency.ErrInFlight:
			metrics.RecordIdempotency("in_flight")
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "idempotency_in_flight"})
		case err == idempotency.ErrBackendUnavailable:
			metrics.RecordIdempotency("backend_error")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
		case err != nil:
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
		}
		// owned == true: we hold the pending slot and must Succeed/Fail below.
	}

	// failIdem clears the pending slot on a terminal failure so a retry can
	// proceed. It is a no-op when there is no idempotency key.
	failIdem := func() {
		if idempotencyKey != "" {
			_ = h.idempotencyStore.Fail(spanCtx, principal, idempotencyKey)
		}
	}

	// Check rate limits
	// 1. Per user (peppered digest so raw user id never lands in a Redis key)
	allowed, _, _, err := h.rateLimitManager.CheckUserRateLimit(
		spanCtx, h.digester.Digest(req.UserID), config.RateLimitPerUser, time.Hour,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
	}
	if !allowed {
		metrics.RecordRateLimitHit("user")
		failIdem()
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 2. Per IP (peppered). Uses the trusted peer IP, never the self-reported one.
	allowed, _, _, err = h.rateLimitManager.CheckIPRateLimit(
		spanCtx, h.digester.Digest(peerIP), config.RateLimitPerIP, time.Minute,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
	}
	if !allowed {
		metrics.RecordRateLimitHit("ip")
		failIdem()
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 3. Per destination (peppered digest of the normalized destination)
	allowed, _, _, err = h.rateLimitManager.CheckDestinationRateLimit(
		spanCtx, h.digester.Digest(req.Destination), config.RateLimitPerDestination, time.Hour,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
	}
	if !allowed {
		metrics.RecordRateLimitHit("destination")
		failIdem()
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "rate_limit_exceeded",
		})
	}

	// 4. Resend cooldown (peppered digest of user+destination)
	cooldownKey := h.digester.DigestParts(req.UserID, req.Destination)
	allowed, _, err = h.rateLimitManager.CheckResendCooldown(spanCtx, cooldownKey, config.ResendCooldown)
	if err != nil {
		h.log.Error().Err(err).Msg("Cooldown check failed")
	}
	if !allowed {
		metrics.RecordRateLimitHit("resend_cooldown")
		failIdem()
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
			"ok":     false,
			"reason": "resend_cooldown",
		})
	}

	// Check if user is locked
	if h.challengeManager.IsUserLocked(spanCtx, req.UserID) {
		failIdem()
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
		ClientIP:    peerIP,
	}
	ch, code, err := h.challengeManager.Create(spanCtx, createReq)
	if err != nil {
		tracing.RecordError(span, err)
		h.log.Error().Err(err).Msg("Failed to create challenge")
		failIdem()
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
	auditlog.LogChallengeCreated(spanCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, peerIP)

	// Metrics: challenge created
	metrics.RecordChallengeCreated(req.Channel, req.Purpose, "success")

	// Store code in test mode (for integration testing only). Guarded by the
	// combined test-environment + test-mode switch so plaintext codes can never
	// be exposed in production even if HERALD_TEST_MODE is accidentally set.
	if config.TestCodeExposureEnabled() {
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
	softDeliveryFailed := false

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
		// Record provider timeouts distinctly for reliability dashboards.
		if errors.Is(err, context.DeadlineExceeded) || errorReason == "timeout" {
			metrics.RecordProviderTimeout(req.Channel)
		}

		// Metrics: send failed
		metrics.RecordOTPSend(req.Channel, providerName, "failure", sendDuration)

		// Audit: send failed
		auditlog.LogSendFailed(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, errorReason, peerIP)

		// Handle provider failure based on policy
		if config.ProviderFailurePolicy == "strict" {
			// Strict mode: revoke challenge and return error
			_ = h.challengeManager.Revoke(spanCtx, ch.ID)
			metrics.RecordChallengeState("revoked")
			// Also clear the idempotency slot so a retry can proceed.
			failIdem()
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":     false,
				"reason": "send_failed",
				"error":  "Failed to send verification code",
			})
		}
		// Soft mode (only permitted outside production / degraded): the challenge
		// stays created but we surface delivery_status=failed so the caller knows
		// the code may not have been delivered. Still promote it to active so it
		// can be verified if it did arrive.
		softDeliveryFailed = true
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
		auditlog.LogSendSuccess(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, messageID, peerIP)
	}

	// Single active challenge (two-phase activate): now that the create+send
	// step is done (success, or soft-failed but still verifiable), promote this
	// challenge to be the only active one for its identity and invalidate the
	// previously active challenge so an older, still-live code cannot be
	// redeemed. A backend error here must not leave a redeemable-but-unindexed
	// code, so on failure we revoke this pending challenge and fail closed.
	prevID, actErr := h.challengeManager.SwapActive(spanCtx, ch)
	if actErr != nil {
		h.log.Error().Err(actErr).Str("challenge_id", ch.ID).Msg("Failed to activate challenge")
		_ = h.challengeManager.RevokePending(spanCtx, ch.ID)
		failIdem()
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}
	if prevID != "" && prevID != ch.ID {
		if err := h.challengeManager.Revoke(spanCtx, prevID); err != nil {
			h.log.Warn().Err(err).Str("challenge_id", prevID).Msg("Failed to revoke superseded challenge")
		} else {
			metrics.RecordChallengeState("superseded")
		}
	}
	metrics.RecordChallengeState("activated")

	// Prepare response
	response := fiber.Map{
		"challenge_id":   ch.ID,
		"expires_in":     int(config.ChallengeExpiry.Seconds()),
		"next_resend_in": int(config.ResendCooldown.Seconds()),
	}
	if softDeliveryFailed {
		response["delivery_status"] = "failed"
	}
	if config.TestCodeExposureEnabled() {
		response["debug_code"] = code
	}

	// Store the terminal idempotency result so concurrent/subsequent duplicates
	// replay this exact response instead of sending another code.
	if idempotencyKey != "" {
		if payload, mErr := json.Marshal(response); mErr == nil {
			if err := h.idempotencyStore.Succeed(spanCtx, principal, idempotencyKey, fingerprint, payload); err != nil {
				h.log.Warn().Err(err).Msg("Failed to persist idempotency success record")
			}
		}
	}

	// Return response
	return c.JSON(response)
}

// idempotencyPrincipal derives a per-caller principal for idempotency
// namespacing from the authenticated service/key headers, falling back to the
// client IP when unauthenticated. It never returns an empty string.
func idempotencyPrincipal(c *fiber.Ctx) string {
	svc := c.Get("X-Service")
	keyID := c.Get("X-Key-Id")
	if svc != "" || keyID != "" {
		return "svc:" + svc + "|kid:" + keyID
	}
	return "ip:" + c.IP()
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
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
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
		if reason == "contended" || contains(reason, "contention") || contains(reason, "try_again") {
			metrics.RecordVerificationContention("contended")
		}

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
	metrics.RecordVerificationContention("acquired")

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

// VerifyChallengeV2Request is the /v2/otp/verifications request. It adds
// explicit context binding: the caller states the purpose/user/channel it
// expects the challenge to have been minted for, and the server enforces it
// atomically inside the verification lock before consuming the code.
type VerifyChallengeV2Request struct {
	ChallengeID     string `json:"challenge_id"`
	Code            string `json:"code"`
	ClientIP        string `json:"client_ip"`
	ExpectedUserID  string `json:"expected_user_id"`
	ExpectedPurpose string `json:"expected_purpose"`
	ExpectedChannel string `json:"expected_channel"`
}

// VerifyChallengeV2 handles context-bound challenge verification.
func (h *Handlers) VerifyChallengeV2(c *fiber.Ctx) error {
	ctx := c.Context()

	var req VerifyChallengeV2Request
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
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
	if !challengekit.ValidateCodeFormat(req.Code, config.CodeLength) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_code_format",
		})
	}

	traceCtx := c.Locals("trace_context")
	if traceCtx == nil {
		traceCtx = ctx
	}
	spanCtx := traceCtx.(context.Context)

	verifyCtx, verifySpan := tracing.StartSpan(spanCtx, "otp.verify")
	defer verifySpan.End()
	verifySpan.SetAttributes(attribute.String("challenge_id", req.ChallengeID))

	opts := challengekit.VerifyOptions{
		ExpectedUserID:  req.ExpectedUserID,
		ExpectedPurpose: req.ExpectedPurpose,
		ExpectedChannel: challengekit.Channel(req.ExpectedChannel),
	}

	result, err := h.challengeManager.VerifyWithOptions(verifyCtx, req.ChallengeID, req.Code, req.ClientIP, opts)
	if err != nil || result == nil || !result.OK {
		reason := "verification_failed"
		if result != nil && result.Reason != "" {
			reason = result.Reason
		}
		tracing.RecordError(verifySpan, err)
		h.log.Debug().Err(err).Msg("Challenge verification (v2) failed")
		verifySpan.SetAttributes(
			attribute.String("result", "failure"),
			attribute.String("reason", reason),
		)
		metrics.RecordVerification("failure", reason)
		auditlog.LogVerificationFailed(verifyCtx, req.ChallengeID, reason, req.ClientIP)

		response := fiber.Map{"ok": false, "reason": reason}
		if result != nil && result.RemainingAttempts != nil {
			response["remaining_attempts"] = *result.RemainingAttempts
		}
		// A context mismatch is a client error (wrong expectations), not an auth
		// failure of the code itself.
		status := fiber.StatusUnauthorized
		if reason == "context_mismatch" {
			status = fiber.StatusConflict
		}
		return c.Status(status).JSON(response)
	}

	ch := result.Challenge
	verifySpan.SetAttributes(
		attribute.String("result", "success"),
		attribute.String("user_id", ch.UserID),
		attribute.String("channel", string(ch.Channel)),
		attribute.String("purpose", ch.Purpose),
	)
	metrics.RecordVerification("success", "")
	auditlog.LogVerificationSuccess(verifyCtx, ch.ID, ch.UserID, string(ch.Channel), ch.Destination, ch.Purpose, req.ClientIP)

	amr := []string{"otp"}
	switch string(ch.Channel) {
	case "sms":
		amr = append(amr, "sms")
	case "email":
		amr = append(amr, "email")
	case "dingtalk":
		amr = append(amr, "dingtalk")
	}

	return c.JSON(fiber.Map{
		"ok":           true,
		"user_id":      ch.UserID,
		"purpose":      ch.Purpose,
		"channel":      string(ch.Channel),
		"challenge_id": ch.ID,
		"amr":          amr,
		"verified_at":  time.Now().Unix(),
		"issued_at":    time.Now().Unix(),
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
	metrics.RecordChallengeState("revoked")

	// Audit: challenge revoked
	auditlog.LogChallengeRevoked(spanCtx, challengeID, c.IP())

	return c.JSON(fiber.Map{
		"ok": true,
	})
}

// GetTestCode retrieves the verification code for a challenge in test mode
// This endpoint is only available when HERALD_TEST_MODE=true
func (h *Handlers) GetTestCode(c *fiber.Ctx) error {
	if !config.TestCodeExposureEnabled() {
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
