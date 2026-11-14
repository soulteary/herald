package tracing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	common_tracing "github.com/soulteary/tracing-kit/v2"
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
			// The server normalizes header names ("Traceparent"), while
			// propagators look fields up in lower case ("traceparent").
			reqHeaders[strings.ToLower(string(key))] = string(value)
		})
		ctx := propagator.Extract(c.Context(), &headerCarrier{headers: reqHeaders})

		// The span is named after the matched route template, which is only
		// known once the handler chain has run: inside a middleware
		// registered with Use, c.Route() is the middleware's own route ("/")
		// until then. It starts under the name an unmatched request keeps.
		// The raw path is never recorded, neither as the name nor as an
		// attribute: it is high-cardinality and carries caller-supplied
		// segments, such as a challenge ID that has not been validated yet.
		own := c.Route()
		ctx, span := tracer.Start(
			ctx,
			unmatchedSpanName(c.Method()),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(semconv.HTTPMethod(c.Method())),
		)
		defer span.End()

		// Store context in Fiber context
		c.Locals("trace_context", ctx)
		c.Locals("trace_span", span)

		// Process request
		err := c.Next()

		// A route other than this middleware's own was matched.
		if route := c.Route(); route != own && route.Path != "" {
			span.SetName(c.Method() + " " + route.Path)
			span.SetAttributes(semconv.HTTPRoute(route.Path))
		}

		// Set span status and attributes based on response. An error
		// returned by the chain is turned into the response by the error
		// handler after this middleware returns, so its status is taken
		// from the error.
		statusCode := c.Response().StatusCode()
		if fiberErr := new(fiber.Error); errors.As(err, &fiberErr) {
			statusCode = fiberErr.Code
		}
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
			respHeaders[strings.ToLower(string(key))] = string(value)
		})
		propagator.Inject(ctx, &headerCarrier{headers: respHeaders})
		// Update response headers
		for k, v := range respHeaders {
			c.Response().Header.Set(k, v)
		}

		return err
	}
}

// unmatchedSpanName names the span of a request that no route matched. It
// contains the method only, never the path.
func unmatchedSpanName(method string) string {
	return method + " unmatched_route"
}

// headerCarrier implements the TextMapCarrier interface for Fiber headers.
// HTTP header names are case-insensitive, so keys are held in lower case and
// looked up in lower case.
type headerCarrier struct {
	headers map[string]string
}

func (c *headerCarrier) Get(key string) string {
	return c.headers[strings.ToLower(key)]
}

func (c *headerCarrier) Set(key, value string) {
	c.headers[strings.ToLower(key)] = value
}

func (c *headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for k := range c.headers {
		keys = append(keys, k)
	}
	return keys
}
