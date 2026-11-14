package tracing

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	common_tracing "github.com/soulteary/tracing-kit/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// serveWithRecorder sends one request through an app that has the tracing
// middleware and the given routes, and returns the server span.
func serveWithRecorder(t *testing.T, register func(app *fiber.App), method, target string) sdktrace.ReadOnlySpan {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	common_tracing.Install(tp, "span-route-test")
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		common_tracing.Uninstall()
	})

	app := fiber.New()
	app.Use(TracingMiddleware("span-route-test"))
	register(app)
	resp, err := app.Test(httptest.NewRequest(method, target, nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	_ = resp.Body.Close()

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	return spans[0]
}

func spanAttribute(span sdktrace.ReadOnlySpan, key string) (string, bool) {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.String(), true
		}
	}
	return "", false
}

func requireNoRawSegment(t *testing.T, span sdktrace.ReadOnlySpan, segments ...string) {
	t.Helper()
	for _, segment := range segments {
		if strings.Contains(span.Name(), segment) {
			t.Errorf("span name %q contains the raw path segment %q", span.Name(), segment)
		}
		for _, attr := range span.Attributes() {
			if strings.Contains(attr.Value.String(), segment) {
				t.Errorf("span attribute %s=%q contains the raw path segment %q", attr.Key, attr.Value.String(), segment)
			}
		}
	}
	if _, ok := spanAttribute(span, "http.target"); ok {
		t.Error("span carries http.target, which is the raw request path")
	}
}

// TestServerSpanIsNamedAfterTheMatchedRoute sends a revoke request whose ID
// segment is not a challenge ID and that is refused before the handler, as a
// request without credentials is. The span must be named after the route
// template and must not carry the segment the caller chose. It used to be
// named "/" for every request and to export the raw path as http.target.
func TestServerSpanIsNamedAfterTheMatchedRoute(t *testing.T) {
	routes := func(app *fiber.App) {
		refuse := func(c fiber.Ctx) error { return c.Status(fiber.StatusUnauthorized).SendString("no") }
		app.Group("/v1").Post("/otp/challenges/:id/revoke", refuse, func(c fiber.Ctx) error { return c.SendString("OK") })
	}
	span := serveWithRecorder(t, routes, "POST", "/v1/otp/challenges/alice@example.com%0Ainjected/revoke?subject=bob@example.com")

	const template = "/v1/otp/challenges/:id/revoke"
	if span.Name() != "POST "+template {
		t.Errorf("span name = %q, want %q", span.Name(), "POST "+template)
	}
	if route, _ := spanAttribute(span, "http.route"); route != template {
		t.Errorf("http.route = %q, want %q", route, template)
	}
	if status, _ := spanAttribute(span, "http.status_code"); status != "401" {
		t.Errorf("http.status_code = %q, want 401", status)
	}
	requireNoRawSegment(t, span, "alice", "injected", "bob")
}

// A request no route matches gets a fixed name that holds the method only,
// and the status the error handler will answer with.
func TestServerSpanOfAnUnmatchedRequestHasAFixedName(t *testing.T) {
	routes := func(app *fiber.App) {
		app.Get("/healthz", func(c fiber.Ctx) error { return c.SendString("OK") })
	}
	span := serveWithRecorder(t, routes, "GET", "/wp-admin/secret-token-123")

	if span.Name() != "GET unmatched_route" {
		t.Errorf("span name = %q, want %q", span.Name(), "GET unmatched_route")
	}
	if route, ok := spanAttribute(span, "http.route"); ok {
		t.Errorf("unmatched request has http.route = %q", route)
	}
	if status, _ := spanAttribute(span, "http.status_code"); status != "404" {
		t.Errorf("http.status_code = %q, want 404", status)
	}
	requireNoRawSegment(t, span, "wp-admin", "secret-token-123")
}
