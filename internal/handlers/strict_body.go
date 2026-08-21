package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// errStrictBody is returned by parseStrictJSON when the request body is not a
// single, well-formed JSON object with only known fields and the correct
// content type. The caller maps it to a 400 with a stable reason.
type strictBodyError struct{ reason, msg string }

func (e *strictBodyError) Error() string { return e.msg }

// parseStrictJSON enforces a strict body contract on core routes:
//   - Content-Type must be application/json (optionally with charset params).
//   - The body must be a single JSON value (no trailing tokens / concatenated
//     JSON).
//   - Unknown fields are rejected (DisallowUnknownFields) so typos or attempts
//     to smuggle extra fields are surfaced instead of silently ignored.
//
// It intentionally does NOT read from c.Body() lazily; it copies the (already
// body-limited by Fiber's BodyLimit) buffer so the decoder can enforce the
// single-value rule.
func parseStrictJSON(c *fiber.Ctx, dst interface{}) error {
	ct := strings.ToLower(strings.TrimSpace(c.Get("Content-Type")))
	// Allow "application/json" and "application/json; charset=utf-8".
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if ct != "application/json" {
		return &strictBodyError{reason: "unsupported_media_type", msg: "Content-Type must be application/json"}
	}

	body := c.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		return &strictBodyError{reason: "empty_body", msg: "request body is empty"}
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return &strictBodyError{reason: "invalid_request", msg: fmt.Sprintf("invalid JSON: %v", err)}
	}
	// Reject any trailing data after the first JSON value (e.g. "{}{}" or "{} x").
	if _, err := dec.Token(); err != io.EOF {
		return &strictBodyError{reason: "invalid_request", msg: "unexpected trailing data after JSON body"}
	}
	return nil
}

// writeStrictBodyError writes the appropriate 4xx status for a strict body
// error. Unsupported media type -> 415; everything else -> 400.
func writeStrictBodyError(c *fiber.Ctx, err error) error {
	be, ok := err.(*strictBodyError)
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ok": false, "reason": "invalid_request"})
	}
	status := fiber.StatusBadRequest
	if be.reason == "unsupported_media_type" {
		status = fiber.StatusUnsupportedMediaType
	}
	return c.Status(status).JSON(fiber.Map{"ok": false, "reason": be.reason})
}
