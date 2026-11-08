package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
	logger "github.com/soulteary/logger-kit/v3"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
	provider "github.com/soulteary/provider-kit"
	"github.com/soulteary/tracing-kit/v2"

	"github.com/soulteary/herald/internal/auditlog"
	"github.com/soulteary/herald/internal/auth"
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
	hashBudget       *hashBudget
	challengeManager challengekit.ManagerInterface
	rateLimitManager *ratelimit.Manager
	providerRegistry *provider.Registry
	templateManager  *template.Manager
	redis            *redis.Client
	testCodeCache    rediskitcache.Cache // For test mode code storage
	idempotencyStore *idempotency.Store  // Atomic, principal-namespaced idempotency store
	totpClient       *totpProxyClient    // Optional: nil when TOTP is not enabled
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
	hashWork, err := newHashBudget(config.MaxConcurrentHashOperations)
	if err != nil {
		return nil, err
	}
	challengeConfig := challengekit.Config{
		RequireActiveVerification:  true,
		VerifyLockPrefix:           challengeVerifyLockPrefix,
		ActiveIndexPrefix:          challengeActiveIndexPrefix,
		MaxConcurrentVerifications: config.MaxConcurrentHashOperations,
		// Do not poll a contended challenge lock while holding the shared budget.
		// A positive expired deadline permits one immediate lock attempt.
		VerifyLockWait:     time.Nanosecond,
		Expiry:             config.ChallengeExpiry,
		MaxAttempts:        config.MaxAttempts,
		LockoutDuration:    config.LockoutDuration,
		CodeLength:         config.CodeLength,
		ChallengeKeyPrefix: challengeRecordPrefix,
		LockKeyPrefix:      "otp:lock:",
	}
	challengeMgr := newAtomicChallengeManager(redisClient, challengeConfig)

	rateLimitMgr := ratelimit.NewManager(redisClient)

	// Wire the application logger before initialization so asynchronous
	// enqueue/write failures are visible as well as counted.
	auditlog.SetLogger(log)
	// Initialize audit logger with Redis client. In production a broken audit
	// backend is a hard startup error rather than a silent downgrade to no-op.
	if err := auditlog.InitWithError(redisClient); err != nil {
		return nil, err
	}

	// Initialize template manager
	templateMgr := template.NewManager(config.TemplateDir)

	// Initialize provider registry (using provider-kit)
	registry := provider.NewRegistry()

	if err := registerEmailProvider(registry, log); err != nil {
		return nil, err
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

	// Atomic idempotency store (Phase 3): principal-namespaced, fingerprinted,
	// SET NX pending placeholder.
	idempotencyStore := idempotency.NewStore(redisClient, "otp:idem2:", []byte(config.IdempotencySecret), config.IdempotencyKeyTTL)

	// TOTP client: when Herald proxies TOTP to herald-totp
	var totpClient *totpProxyClient
	if config.TOTPEnabled && config.TOTPBaseURL != "" {
		opts := heraldtotp.DefaultOptions().
			WithBaseURL(strings.TrimSuffix(config.TOTPBaseURL, "/")).
			WithAPIKey(config.TOTPAPIKey).
			WithTimeout(10 * time.Second)
		if config.TOTPHMACSecret != "" {
			opts = opts.WithHMACSecret(config.TOTPHMACSecret).WithHMACKeyID(config.TOTPHMACKeyID)
			if config.TOTPHMACService != "" {
				opts.Service = config.TOTPHMACService
			}
		}
		if c, err := newTOTPProxyClient(opts, int64(config.ProviderMaxResponseBytes), config.IsProduction()); err != nil {
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
		hashBudget:       hashWork,
		challengeManager: challengeMgr,
		rateLimitManager: rateLimitMgr,
		providerRegistry: registry,
		templateManager:  templateMgr,
		redis:            redisClient,
		testCodeCache:    testCodeCache,
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
func (h *Handlers) CreateChallenge(c fiber.Ctx) error {
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

	if valid, err := h.validateCreateRequest(c, &req); !valid {
		return err
	}

	// Client IP trust split (Phase 6):
	//   - peerIP is the transport-level source address (or the leftmost trusted
	//     forwarded hop, per Fiber's ProxyHeader/TrustedProxies config). This is
	//     the ONLY value used for IP rate limiting so a caller cannot evade the
	//     per-IP limit by rotating a self-reported field.
	//   - reportedClientIP is advisory (supplied by the trusted upstream service
	//     in the request body) and is only recorded in audit logs.
	peerIP := c.IP()
	h.logReportedClientIP(req.ClientIP, peerIP)

	// Atomic idempotency claim (Phase 3). Principal = authenticated service +
	// key id when present, else the client IP, so keys are namespaced per
	// caller. On a duplicate success we replay the stored response; on a reused
	// key with a different body we return 409; on a concurrent in-flight request
	// we return 409 (retryable); on backend failure we fail closed.
	principal := idempotencyPrincipal(c)
	fingerprint := idempotency.Fingerprint(
		req.UserID, strings.ToLower(req.Channel), destination.Normalize(req.Channel, req.Destination), req.Purpose, req.Locale,
	)
	idempotencyOwner, handled, err := h.beginCreation(c, spanCtx, principal, idempotencyKey, fingerprint)
	if handled {
		return err
	}

	// failIdem clears the pending slot on a terminal failure so a retry can
	// proceed. It is a no-op when there is no idempotency key.
	failIdem := func() {
		h.releaseCreationClaim(spanCtx, principal, idempotencyKey, fingerprint, idempotencyOwner)
	}

	ch, code, handled, err := h.prepareChallenge(c, spanCtx, req, peerIP, failIdem)
	if handled {
		return err
	}
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
	if err := auditlog.LogChallengeCreated(spanCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, peerIP); err != nil {
		_ = h.challengeManager.RevokePending(spanCtx, ch.ID)
		failIdem()
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	// Metrics: challenge created
	metrics.RecordChallengeCreated(req.Channel, req.Purpose, "success")

	h.storeTestCode(spanCtx, ch.ID, code)

	msg, providerName := h.buildDeliveryMessage(req, ch.ID, code)

	if persisted, err := h.persistSendBoundary(c, spanCtx, ch, principal, idempotencyKey, fingerprint, idempotencyOwner); !persisted {
		return err
	}

	softDeliveryFailed, delivered, err := h.deliverChallenge(c, spanCtx, req, ch, msg, providerName, peerIP)
	if !delivered {
		return err
	}

	if activated, err := h.activateChallenge(c, spanCtx, ch, principal, idempotencyKey, fingerprint, idempotencyOwner); !activated {
		return err
	}

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
	if completed, err := h.completeCreation(c, spanCtx, response, principal, idempotencyKey, fingerprint, idempotencyOwner); !completed {
		return err
	}

	// Return response
	return c.JSON(response)
}

// idempotencyPrincipal derives a per-caller principal for idempotency
// namespacing from the verified HMAC service/resolved key ID, otherwise the
// caller headers, falling back to the
// client IP when unauthenticated. It never returns an empty string.
// Request auth rejects the reserved |kid: delimiter in either header before
// this function is reached, so distinct component pairs cannot alias.
func idempotencyPrincipal(c fiber.Ctx) string {
	if svc, keyID, ok := auth.VerifiedHMACIdentity(c); ok {
		return "svc:" + svc + "|kid:" + keyID
	}
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
func (h *Handlers) VerifyChallenge(c fiber.Ctx) error {
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
	peerIP := c.IP()
	if req.ClientIP != "" && req.ClientIP != peerIP {
		h.log.Debug().
			Str("reported_client_ip_digest", h.digester.Digest(req.ClientIP)).
			Msg("verification client_ip differs from trusted peer IP (advisory only)")
	}

	if err := auditlog.LogVerificationAttempt(verifyCtx, req.ChallengeID, peerIP); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	// Verify challenge
	result, err := h.verifyWithBudget(verifyCtx, req.ChallengeID, req.Code, peerIP, nil)
	if err != nil || result == nil || !result.OK {
		return h.writeVerificationFailure(c, verifyCtx, verifySpan, req.ChallengeID, peerIP, result, err, false)
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
	if err := auditlog.LogVerificationSuccess(verifyCtx, ch.ID, ch.UserID, string(ch.Channel), ch.Destination, ch.Purpose, peerIP); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	// Generate AMR based on channel (use string to avoid depending on challengekit.ChannelDingTalk in v1.0.0)
	amr := verificationAMR(string(ch.Channel))

	// Success
	return c.JSON(fiber.Map{
		"ok":        true,
		"user_id":   ch.UserID,
		"amr":       amr,
		"issued_at": time.Now().Unix(),
	})
}

// VerifyChallengeV2Request is the /v2/otp/verifications request. It adds
// required context binding: the caller states the purpose/user/channel it
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
func (h *Handlers) VerifyChallengeV2(c fiber.Ctx) error {
	ctx := c.Context()

	var req VerifyChallengeV2Request
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
	}

	if valid, err := validateBoundVerification(c, req); !valid {
		return err
	}

	traceCtx := c.Locals("trace_context")
	if traceCtx == nil {
		traceCtx = ctx
	}
	spanCtx := traceCtx.(context.Context)

	verifyCtx, verifySpan := tracing.StartSpan(spanCtx, "otp.verify")
	defer verifySpan.End()
	verifySpan.SetAttributes(attribute.String("challenge_id", req.ChallengeID))
	peerIP := c.IP()
	if req.ClientIP != "" && req.ClientIP != peerIP {
		h.log.Debug().
			Str("reported_client_ip_digest", h.digester.Digest(req.ClientIP)).
			Msg("verification client_ip differs from trusted peer IP (advisory only)")
	}

	if err := auditlog.LogVerificationAttempt(verifyCtx, req.ChallengeID, peerIP); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	opts := challengekit.VerifyOptions{
		ExpectedUserID:  req.ExpectedUserID,
		ExpectedPurpose: req.ExpectedPurpose,
		ExpectedChannel: challengekit.Channel(req.ExpectedChannel),
	}

	result, err := h.verifyWithBudget(verifyCtx, req.ChallengeID, req.Code, peerIP, &opts)
	if err != nil || result == nil || !result.OK {
		return h.writeVerificationFailure(c, verifyCtx, verifySpan, req.ChallengeID, peerIP, result, err, true)
	}

	ch := result.Challenge
	verifySpan.SetAttributes(
		attribute.String("result", "success"),
		attribute.String("user_id", ch.UserID),
		attribute.String("channel", string(ch.Channel)),
		attribute.String("purpose", ch.Purpose),
	)
	metrics.RecordVerification("success", "")
	if err := auditlog.LogVerificationSuccess(verifyCtx, ch.ID, ch.UserID, string(ch.Channel), ch.Destination, ch.Purpose, peerIP); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	amr := verificationAMR(string(ch.Channel))

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
func (h *Handlers) RevokeChallenge(c fiber.Ctx) error {
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

	if err := auditlog.LogRevokeAttempt(spanCtx, challengeID, c.IP()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	if err := h.challengeManager.Revoke(spanCtx, challengeID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}
	metrics.RecordChallengeState("revoked")

	// Audit: challenge revoked
	if err := auditlog.LogChallengeRevoked(spanCtx, challengeID, c.IP()); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	return c.JSON(fiber.Map{
		"ok": true,
	})
}

// GetTestCode retrieves the verification code for a challenge in test mode
// This endpoint is only available when HERALD_TEST_MODE=true
func (h *Handlers) GetTestCode(c fiber.Ctx) error {
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

func verificationFailureStatus(reason string, err error) int {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fiber.StatusServiceUnavailable
	}
	normalized := strings.ToLower(reason)
	if normalized == "backend_unavailable" || normalized == "internal_error" ||
		contains(normalized, "storage_error") || contains(normalized, "unavailable") {
		return fiber.StatusServiceUnavailable
	}
	if normalized == "context_mismatch" || normalized == "contended" ||
		contains(normalized, "contention") || contains(normalized, "try_again") {
		return fiber.StatusConflict
	}
	return fiber.StatusUnauthorized
}

func verificationReasonFromError(err error) string {
	if err == nil {
		return "verification_failed"
	}
	normalized := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case normalized == "expired" || contains(normalized, "challenge expired"):
		return "expired"
	case normalized == "locked" || contains(normalized, "challenge locked") || contains(normalized, "user locked"):
		return "locked"
	case normalized == "invalid" || contains(normalized, "invalid code"):
		return "invalid"
	case normalized == "not_found" || contains(normalized, "challenge not found"):
		return "not_found"
	default:
		// Unknown errors are infrastructure failures. In particular, do not
		// classify Redis errors such as WRONGPASS (which contains "invalid") as
		// a bad verification code.
		return "backend_unavailable"
	}
}

func verificationFailureRetryable(reason string, status int) bool {
	if status == fiber.StatusServiceUnavailable {
		return true
	}
	normalized := strings.ToLower(reason)
	return normalized == "contended" || contains(normalized, "contention") || contains(normalized, "try_again")
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func (h *Handlers) validateCreateRequest(c fiber.Ctx, req *CreateChallengeRequest) (bool, error) {
	// Validate request
	if req.UserID == "" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "user_id_required",
		})
	}

	if req.Channel != "sms" && req.Channel != "email" && req.Channel != "dingtalk" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_channel",
		})
	}

	if req.Destination == "" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "destination_required",
		})
	}

	// Validate + normalize the destination once. The normalized value is used
	// for the provider call, rate-limit keys, and idempotency fingerprint so
	// trivial variants (case/spacing/separators) cannot bypass dedup or limits.
	normDest, derr := destination.Validate(req.Channel, req.Destination)
	if derr != nil {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
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
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_purpose",
			"error":  fmt.Sprintf("Purpose must be one of: %s", strings.Join(config.AllowedPurposes, ", ")),
		})
	}

	return true, nil
}

func (h *Handlers) checkCreationLimits(c fiber.Ctx, spanCtx context.Context, req CreateChallengeRequest, peerIP string, failIdem func()) (bool, error) {
	// Check rate limits
	// 1. Per user (peppered digest so raw user id never lands in a Redis key)
	allowed, _, resetTime, err := h.rateLimitManager.CheckUserRateLimit(
		spanCtx, h.digester.Digest(req.UserID), config.RateLimitPerUser, time.Hour,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
		failIdem()
		return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "backend_unavailable",
		})
	}
	if !allowed {
		metrics.RecordRateLimitHit("user")
		failIdem()
		return challengeRateLimitResponse(c, "rate_limit_exceeded", resetTime)
	}

	// 2. Per IP (peppered). Uses the trusted peer IP, never the self-reported one.
	allowed, _, resetTime, err = h.rateLimitManager.CheckIPRateLimit(
		spanCtx, h.digester.Digest(peerIP), config.RateLimitPerIP, time.Minute,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
		failIdem()
		return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "backend_unavailable",
		})
	}
	if !allowed {
		metrics.RecordRateLimitHit("ip")
		failIdem()
		return challengeRateLimitResponse(c, "rate_limit_exceeded", resetTime)
	}

	return h.checkDestinationLimits(c, spanCtx, req, failIdem)
}

func (h *Handlers) checkDestinationLimits(c fiber.Ctx, spanCtx context.Context, req CreateChallengeRequest, failIdem func()) (bool, error) {
	// 3. Per destination (peppered digest of the normalized destination)
	allowed, _, resetTime, err := h.rateLimitManager.CheckDestinationRateLimit(
		spanCtx, h.digester.Digest(req.Destination), config.RateLimitPerDestination, time.Hour,
	)
	if err != nil {
		h.log.Error().Err(err).Msg("Rate limit check failed")
		failIdem()
		return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "backend_unavailable",
		})
	}
	if !allowed {
		metrics.RecordRateLimitHit("destination")
		failIdem()
		return challengeRateLimitResponse(c, "rate_limit_exceeded", resetTime)
	}

	// 4. Resend cooldown (peppered digest of user+destination)
	cooldownKey := h.digester.DigestParts(req.UserID, req.Destination)
	allowed, resetTime, err = h.rateLimitManager.CheckResendCooldown(spanCtx, cooldownKey, config.ResendCooldown)
	if err != nil {
		h.log.Error().Err(err).Msg("Cooldown check failed")
		failIdem()
		return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "backend_unavailable",
		})
	}
	if !allowed {
		metrics.RecordRateLimitHit("resend_cooldown")
		failIdem()
		return challengeRateLimitResponse(c, "resend_cooldown", resetTime)
	}

	return true, nil
}

func (h *Handlers) deliverChallenge(c fiber.Ctx, spanCtx context.Context, req CreateChallengeRequest, ch *challengekit.Challenge, msg *provider.Message, providerName, peerIP string) (bool, bool, error) {
	softDeliveryFailed := false
	channel := provider.Channel(req.Channel)
	// Start provider observability only after the send boundary is persisted.
	providerCtx, providerSpan := tracing.StartSpan(spanCtx, "otp.provider.send")
	providerSpan.SetAttributes(
		attribute.String("channel", req.Channel),
		attribute.String("provider", providerName),
	)

	// Record only provider latency; send-boundary persistence belongs to Redis.
	sendStart := time.Now()

	// Send using provider-kit Registry (returns *SendResult, error)
	sendResult, err := h.providerRegistry.Send(providerCtx, channel, msg)
	sendDuration := time.Since(sendStart)

	if err != nil || (sendResult != nil && !sendResult.OK) {
		// Provider errors may echo OTP bodies, destinations or credentials. Keep
		// only a bounded reason code in logs, span status/events and audit data.
		errorReason := providerFailureReason(sendResult, err)
		tracing.RecordError(providerSpan, errors.New("provider send failed: "+errorReason))
		providerSpan.End()
		h.log.Error().Str("reason", errorReason).Msg("Failed to send verification code via provider")
		// Record provider timeouts distinctly for reliability dashboards.
		if errors.Is(err, context.DeadlineExceeded) || errorReason == "timeout" {
			metrics.RecordProviderTimeout(req.Channel)
		}

		// Metrics: send failed
		metrics.RecordOTPSend(req.Channel, providerName, "failure", sendDuration)

		// Audit: send failed
		auditErr := auditlog.LogSendFailed(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, errorReason, peerIP)

		// Handle provider failure based on policy
		if config.NormalizedProviderFailurePolicy() == "strict" {
			// Strict mode: revoke challenge and return error
			_ = h.challengeManager.Revoke(spanCtx, ch.ID)
			metrics.RecordChallengeState("revoked")
			// The provider may have delivered before reporting an error. Keep
			// the sending record so a retry cannot send a second code.
			// Audit failure must not skip strict-policy revocation.
			if auditErr != nil {
				return false, false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
			}
			return false, false, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":     false,
				"reason": "send_failed",
				"error":  "Failed to send verification code",
			})
		}
		if auditErr != nil {
			return false, false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
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
		if err := auditlog.LogSendSuccess(providerCtx, ch.ID, req.UserID, req.Channel, req.Destination, req.Purpose, providerName, messageID, peerIP); err != nil {
			return false, false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
		}
	}

	return softDeliveryFailed, true, nil
}

func (h *Handlers) activateChallenge(c fiber.Ctx, spanCtx context.Context, ch *challengekit.Challenge, principal, idempotencyKey, fingerprint, idempotencyOwner string) (bool, error) {
	// Single active challenge (two-phase activate): now that the create+send
	// step is done (success, or soft-failed but still verifiable), promote this
	// challenge to be the only active one for its identity and invalidate the
	// previously active challenge so an older, still-live code cannot be
	// redeemed. A backend error here must not leave a redeemable-but-unindexed
	// code, so on failure we revoke this pending challenge and fail closed.
	// Re-check ownership immediately before activation. Sending records remain
	// non-expiring; a backend error must not reopen the key for another send.
	if idempotencyKey != "" {
		if err := h.idempotencyStore.Refresh(spanCtx, principal, idempotencyKey, fingerprint, idempotencyOwner); err != nil {
			_ = h.challengeManager.RevokePending(spanCtx, ch.ID)
			if errors.Is(err, idempotency.ErrOwnershipLost) {
				metrics.RecordIdempotency("ownership_lost")
				return false, c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"ok":     false,
					"reason": "idempotency_ownership_lost",
				})
			}
			h.log.Error().Err(err).Msg("Failed to refresh idempotency ownership")
			return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"ok":     false,
				"reason": "backend_unavailable",
			})
		}
	}
	prevID, actErr := h.challengeManager.SwapActive(spanCtx, ch)
	if actErr != nil {
		h.log.Error().Err(actErr).Str("challenge_id", ch.ID).Msg("Failed to activate challenge")
		_ = h.challengeManager.RevokePending(spanCtx, ch.ID)
		return false, c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":     false,
			"reason": "internal_error",
		})
	}
	if prevID != "" && prevID != ch.ID {
		metrics.RecordChallengeState("superseded")
	}
	metrics.RecordChallengeState("activated")

	return true, nil
}

func (h *Handlers) beginCreation(c fiber.Ctx, spanCtx context.Context, principal, idempotencyKey, fingerprint string) (string, bool, error) {
	if idempotencyKey != "" {
		replay, owner, err := h.beginIdempotency(c, spanCtx, principal, idempotencyKey, fingerprint)
		switch {
		case err == nil && owner == "" && replay != nil:
			metrics.RecordIdempotency("replay")
			c.Set("Content-Type", "application/json")
			return "", true, c.Status(fiber.StatusOK).Send(replay)
		case err == idempotency.ErrNamespaceConflict:
			metrics.RecordIdempotency("conflict")
			return "", true, c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "idempotency_namespace_conflict"})
		case err == idempotency.ErrConflict:
			metrics.RecordIdempotency("conflict")
			return "", true, c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "idempotency_conflict"})
		case err == idempotency.ErrDeliveryUnknown:
			metrics.RecordIdempotency("delivery_unknown")
			return "", true, c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "delivery_unknown"})
		case err == idempotency.ErrInFlight:
			metrics.RecordIdempotency("in_flight")
			return "", true, c.Status(fiber.StatusConflict).JSON(fiber.Map{"ok": false, "reason": "idempotency_in_flight"})
		case err == idempotency.ErrBackendUnavailable:
			metrics.RecordIdempotency("backend_error")
			return "", true, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
		case err != nil:
			return "", true, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
		}
		return owner, false, nil
		// A non-empty owner token proves this handler holds the pending slot and
		// must be supplied to Succeed/Fail below.
	}

	return "", false, nil
}

func (h *Handlers) buildDeliveryMessage(req CreateChallengeRequest, challengeID, code string) (*provider.Message, string) {
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
		WithIdempotencyKey(challengeID) // Use challenge ID as idempotency key

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

	return msg, providerName
}

func (h *Handlers) persistSendBoundary(c fiber.Ctx, spanCtx context.Context, ch *challengekit.Challenge, principal, idempotencyKey, fingerprint, idempotencyOwner string) (bool, error) {
	// Persist the send boundary before invoking the provider. A TTL lease alone
	// would expire after a process exit and allow a second delivery.
	if idempotencyKey != "" {
		if err := h.idempotencyStore.MarkSending(spanCtx, principal, idempotencyKey, fingerprint, idempotencyOwner, ch.ID); err != nil {
			_ = h.challengeManager.RevokePending(spanCtx, ch.ID)
			if errors.Is(err, idempotency.ErrOwnershipLost) {
				metrics.RecordIdempotency("ownership_lost")
				return false, c.Status(fiber.StatusConflict).JSON(fiber.Map{
					"ok":     false,
					"reason": "idempotency_ownership_lost",
				})
			}
			h.log.Error().Err(err).Msg("Failed to persist idempotency send boundary")
			return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"ok":     false,
				"reason": "backend_unavailable",
			})
		}
	}

	return true, nil
}

func (h *Handlers) writeVerificationFailure(c fiber.Ctx, verifyCtx context.Context, verifySpan trace.Span, challengeID, peerIP string, result *challengekit.VerifyResult, err error, bound bool) error {
	reason := failedVerificationReason(result, err, bound)
	tracing.RecordError(verifySpan, err)
	h.log.Debug().Err(err).Msg("Challenge verification failed")

	// Set span attributes for failure
	verifySpan.SetAttributes(
		attribute.String("result", "failure"),
		attribute.String("reason", reason),
	)

	// Metrics: verification failed
	metrics.RecordVerification("failure", reason)
	if !bound && (reason == "contended" || contains(reason, "contention") || contains(reason, "try_again")) {
		metrics.RecordVerificationContention("contended")
	}

	// Audit: verification failed
	if err := auditlog.LogVerificationFailed(verifyCtx, challengeID, reason, peerIP); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "audit_unavailable"})
	}

	response := fiber.Map{
		"ok":     false,
		"reason": reason,
	}
	if result != nil && result.RemainingAttempts != nil {
		response["remaining_attempts"] = *result.RemainingAttempts
	}

	status := verificationFailureStatus(reason, err)
	if verificationFailureRetryable(reason, status) {
		c.Set("Retry-After", "1")
	}
	return c.Status(status).JSON(response)
}

func failedVerificationReason(result *challengekit.VerifyResult, err error, bound bool) string {
	if result != nil && result.Reason != "" {
		return result.Reason
	}
	if err != nil {
		if bound {
			return "backend_unavailable"
		}
		return verificationReasonFromError(err)
	}
	if result == nil {
		return "backend_unavailable"
	}
	return "verification_failed"
}

func verificationAMR(channel string) []string {
	amr := []string{"otp"}
	switch channel {
	case "sms":
		amr = append(amr, "sms")
	case "email":
		amr = append(amr, "email")
	case "dingtalk":
		amr = append(amr, "dingtalk")
	}
	return amr
}

func validateBoundVerification(c fiber.Ctx, req VerifyChallengeV2Request) (bool, error) {
	if req.ChallengeID == "" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "challenge_id_required",
		})
	}
	if req.Code == "" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "code_required",
		})
	}
	if !challengekit.ValidateCodeFormat(req.Code, config.CodeLength) {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_code_format",
		})
	}

	if strings.TrimSpace(req.ExpectedUserID) == "" ||
		strings.TrimSpace(req.ExpectedPurpose) == "" ||
		strings.TrimSpace(req.ExpectedChannel) == "" {
		return false, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "context_required",
		})
	}

	return true, nil
}

func registerEmailProvider(registry *provider.Registry, log *logger.Logger) error {
	// Register email channel: herald-smtp HTTP provider takes precedence over built-in SMTP
	if config.HeraldSMTPAPIURL != "" {
		httpProvider, err := provider.NewHTTPProvider(hardenedHTTPConfig(config.HeraldSMTPAPIURL, config.HeraldSMTPAPIKey, provider.ChannelEmail, "smtp"))
		if err != nil {
			return fmt.Errorf("herald-smtp provider config invalid: %w", err)
		}
		if err := registry.Register(httpProvider); err != nil {
			return fmt.Errorf("failed to register herald-smtp provider: %w", err)
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
			Timeout:     config.ProviderTimeout,
		}
		smtpProvider, err := provider.NewSMTPProvider(smtpConfig)
		if err != nil {
			return fmt.Errorf("SMTP provider config invalid: %w", err)
		}
		if err := registry.Register(smtpProvider); err != nil {
			return fmt.Errorf("failed to register SMTP provider: %w", err)
		}
		log.Info().Msg("SMTP provider registered")
	}

	return nil
}

func (h *Handlers) logReportedClientIP(reportedClientIP, peerIP string) {
	// The reported client IP is advisory only. Record a peppered digest at debug
	// level for troubleshooting mismatches without trusting or exposing it.
	if reportedClientIP != "" && reportedClientIP != peerIP {
		h.log.Debug().
			Str("reported_client_ip_digest", h.digester.Digest(reportedClientIP)).
			Msg("reported client_ip differs from trusted peer IP (advisory only)")
	}

}

func (h *Handlers) storeTestCode(spanCtx context.Context, challengeID, code string) {
	// Store code in test mode (for integration testing only). Guarded by the
	// combined test-environment + test-mode switch so plaintext codes can never
	// be exposed in production even if HERALD_TEST_MODE is accidentally set.
	if config.TestCodeExposureEnabled() {
		if err := h.testCodeCache.Set(spanCtx, challengeID, code, config.ChallengeExpiry); err != nil {
			h.log.Warn().Err(err).Msg("Failed to store test code")
		}
	}

}

func (h *Handlers) completeCreation(c fiber.Ctx, spanCtx context.Context, response fiber.Map, principal, idempotencyKey, fingerprint, idempotencyOwner string) (bool, error) {
	if idempotencyKey != "" {
		payload, err := json.Marshal(response)
		if err == nil {
			err = h.idempotencyStore.Succeed(spanCtx, principal, idempotencyKey, fingerprint, idempotencyOwner, payload)
		}
		if err != nil {
			h.log.Error().Err(err).Msg("Failed to persist idempotency success record")
			return false, c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "backend_unavailable"})
		}
	}

	return true, nil
}

func (h *Handlers) releaseCreationClaim(ctx context.Context, principal, key, fingerprint, owner string) {
	if key != "" {
		_ = h.idempotencyStore.Fail(ctx, principal, key, fingerprint, owner)
	}
}
