package handlers

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/soulteary/herald-totp/pkg/heraldtotp"
)

// maskSubject returns a short, non-reversible tag for a TOTP subject so logs
// never contain the raw subject (which is PII, e.g. an email or user id). It
// uses the same peppered digester as rate-limit keys.
func (h *Handlers) maskSubject(subject string) string {
	if subject == "" {
		return ""
	}
	d := h.digester.Digest(subject)
	if len(d) > 12 {
		d = d[:12]
	}
	return "sub_" + d
}

// TOTPStatus proxies GET /v1/totp/status to herald-totp.
func (h *Handlers) TOTPStatus(c fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	subject := c.Query("subject")
	if subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject required",
		})
	}
	ctx := c.Context()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Status(ctx, subject)
	if err != nil {
		h.log.Warn().Str("subject", h.maskSubject(subject)).Msg("TOTP status proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}

// TOTPVerify proxies POST /v1/totp/verify to herald-totp.
func (h *Handlers) TOTPVerify(c fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.VerifyRequest
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
	}
	if req.Subject == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject and code required",
		})
	}
	ctx := c.Context()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Verify(ctx, &req)
	if err != nil {
		h.log.Warn().Str("subject", h.maskSubject(req.Subject)).Msg("TOTP verify proxy failed")
		// Return 200 with ok:false when verify fails (same as herald-totp)
		if resp != nil {
			return c.JSON(resp)
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}

// TOTPEnrollStart proxies POST /v1/totp/enroll/start to herald-totp.
func (h *Handlers) TOTPEnrollStart(c fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.EnrollStartRequest
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
	}
	if req.Subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject required",
		})
	}
	ctx := c.Context()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.EnrollStart(ctx, &req)
	if err != nil {
		// Do NOT log err (it may contain the upstream body) nor the raw subject.
		h.log.Warn().Str("subject", h.maskSubject(req.Subject)).Msg("TOTP enroll/start proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}

// classifyEnrollConfirmError preserves herald-totp's explicit 400 contract.
 // herald-totp v1.0.0 exposes upstream status only in this stable error prefix;
 // all other failures (transport, malformed response, or 5xx) are proxy failures.
func classifyEnrollConfirmError(err error) (int, string) {
	if err != nil && strings.HasPrefix(err.Error(), "enroll/confirm returned 400:") {
		return fiber.StatusBadRequest, "invalid"
	}
	return fiber.StatusBadGateway, "proxy_failed"
}

// TOTPEnrollConfirm proxies POST /v1/totp/enroll/confirm to herald-totp.
func (h *Handlers) TOTPEnrollConfirm(c fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.EnrollConfirmRequest
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
	}
	if req.EnrollID == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "enroll_id and code required",
		})
	}
	ctx := c.Context()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.EnrollConfirm(ctx, &req)
	if err != nil {
		// enroll_id is a random opaque token (not PII) so it is safe to log; the
		// upstream error is not echoed to avoid leaking response bodies.
		status, reason := classifyEnrollConfirmError(err)
		h.log.Warn().Str("enroll_id", req.EnrollID).Int("upstream_status", status).Msg("TOTP enroll/confirm proxy failed")
		return c.Status(status).JSON(fiber.Map{
			"ok":     false,
			"reason": reason,
		})
	}
	return c.JSON(resp)
}

// TOTPRevoke proxies POST /v1/totp/revoke to herald-totp.
func (h *Handlers) TOTPRevoke(c fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req struct {
		Subject string `json:"subject"`
	}
	if err := parseStrictJSON(c, &req); err != nil {
		return writeStrictBodyError(c, err)
	}
	if req.Subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject required",
		})
	}
	ctx := c.Context()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Revoke(ctx, req.Subject)
	if err != nil {
		h.log.Warn().Str("subject", h.maskSubject(req.Subject)).Msg("TOTP revoke proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}
