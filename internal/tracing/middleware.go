package tracing

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
	common_tracing "github.com/soulteary/tracing-kit"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingMiddleware creates a Fiber middleware for OpenTelemetry tracing
func TracingMiddleware(serviceName string) fiber.Handler {
	propagator := otel.GetTextMapPropagator()
	tracer := common_tracing.GetTracer()

	return func(c fiber.Ctx) error {
		// Extract trace context from request headers
		reqHeaders := make(map[string]string)
		// nolint:staticcheck // SA1019: VisitAll is deprecated but we need to iterate all headers
		c.Request().Header.VisitAll(func(key, value []byte) {
			reqHeaders[string(key)] = string(value)
		})
		ctx := propagator.Extract(c.Context(), &headerCarrier{headers: reqHeaders})

		// Determine span name from the templated route path only. Never fall back
		// to the raw URL/path: it can be high-cardinality and may carry PII in the
		// query string (e.g. ?subject=...).
		spanName := c.Route().Path
		if spanName == "" {
			spanName = c.Method() + " unmatched_route"
		}

		// http.target is the path WITHOUT the query string so PII carried in query
		// params never lands in a span attribute.
		path := c.Path()

		// Start span
		ctx, span := tracer.Start(
			ctx,
			spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(c.Method()),
				semconv.HTTPTarget(path),
				attribute.String("http.remote_addr", c.IP()),
			),
		)
		if c.Route().Path != "" {
			span.SetAttributes(semconv.HTTPRoute(c.Route().Path))
		}
		defer span.End()

		// Store context in Fiber context
		c.Locals("trace_context", ctx)
		c.Locals("trace_span", span)

		// Process request
		err := c.Next()

		// Set span status and attributes based on response
		statusCode := c.Response().StatusCode()
		span.SetAttributes(
			semconv.HTTPStatusCode(statusCode),
			attribute.Int("http.response.size", len(c.Response().Body())),
		)

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else if statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Inject trace context into response headers
		respHeaders := make(map[string]string)
		// nolint:staticcheck // SA1019: VisitAll is deprecated but we need to iterate all headers
		c.Response().Header.VisitAll(func(key, value []byte) {
			respHeaders[string(key)] = string(value)
		})
		propagator.Inject(ctx, &headerCarrier{headers: respHeaders})
		// Update response headers
		for k, v := range respHeaders {
			c.Response().Header.Set(k, v)
		}

		return err
	}
}

// headerCarrier implements the TextMapCarrier interface for Fiber headers
type headerCarrier struct {
	headers map[string]string
}

func (c *headerCarrier) Get(key string) string {
	return c.headers[key]
}

func (c *headerCarrier) Set(key, value string) {
	c.headers[key] = value
}

func (c *headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}
