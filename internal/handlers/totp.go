package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/soulteary/herald-totp/pkg/heraldtotp"
)

// TOTPStatus proxies GET /v1/totp/status to herald-totp.
func (h *Handlers) TOTPStatus(c *fiber.Ctx) error {
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
	ctx := c.UserContext()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Status(ctx, subject)
	if err != nil {
		h.log.Warn().Err(err).Str("subject", subject).Msg("TOTP status proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}

// TOTPVerify proxies POST /v1/totp/verify to herald-totp.
func (h *Handlers) TOTPVerify(c *fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.VerifyRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}
	if req.Subject == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject and code required",
		})
	}
	ctx := c.UserContext()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Verify(ctx, &req)
	if err != nil {
		h.log.Warn().Err(err).Str("subject", req.Subject).Msg("TOTP verify proxy failed")
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
func (h *Handlers) TOTPEnrollStart(c *fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.EnrollStartRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}
	if req.Subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject required",
		})
	}
	ctx := c.UserContext()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.EnrollStart(ctx, &req)
	if err != nil {
		h.log.Warn().Err(err).Str("subject", req.Subject).Msg("TOTP enroll/start proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}

// TOTPEnrollConfirm proxies POST /v1/totp/enroll/confirm to herald-totp.
func (h *Handlers) TOTPEnrollConfirm(c *fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req heraldtotp.EnrollConfirmRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}
	if req.EnrollID == "" || req.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "enroll_id and code required",
		})
	}
	ctx := c.UserContext()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.EnrollConfirm(ctx, &req)
	if err != nil {
		h.log.Warn().Err(err).Str("enroll_id", req.EnrollID).Msg("TOTP enroll/confirm proxy failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid",
			"error":  err.Error(),
		})
	}
	return c.JSON(resp)
}

// TOTPRevoke proxies POST /v1/totp/revoke to herald-totp.
func (h *Handlers) TOTPRevoke(c *fiber.Ctx) error {
	if h.totpClient == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":     false,
			"reason": "totp_not_configured",
		})
	}
	var req struct {
		Subject string `json:"subject"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  err.Error(),
		})
	}
	if req.Subject == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":     false,
			"reason": "invalid_request",
			"error":  "subject required",
		})
	}
	ctx := c.UserContext()
	if v := c.Locals("trace_context"); v != nil {
		if cc, ok := v.(context.Context); ok {
			ctx = cc
		}
	}
	resp, err := h.totpClient.Revoke(ctx, req.Subject)
	if err != nil {
		h.log.Warn().Err(err).Str("subject", req.Subject).Msg("TOTP revoke proxy failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":     false,
			"reason": "proxy_failed",
		})
	}
	return c.JSON(resp)
}
