package middleware

import (
	"github.com/gofiber/fiber/v2"
)

const (
	TraceparentKey = "traceparent"
	TracestateKey  = "tracestate"
)

// TraceContext extracts and stores trace context from headers
func TraceContext() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Extract traceparent and tracestate from headers
		traceparent := c.Get("traceparent")
		tracestate := c.Get("tracestate")

		// Store in locals for use in handlers
		if traceparent != "" {
			c.Locals(TraceparentKey, traceparent)
		}
		if tracestate != "" {
			c.Locals(TracestateKey, tracestate)
		}

		return c.Next()
	}
}

// GetTraceparent retrieves traceparent from context
func GetTraceparent(c *fiber.Ctx) string {
	if val, ok := c.Locals(TraceparentKey).(string); ok {
		return val
	}
	return ""
}

// GetTracestate retrieves tracestate from context
func GetTracestate(c *fiber.Ctx) string {
	if val, ok := c.Locals(TracestateKey).(string); ok {
		return val
	}
	return ""
}
