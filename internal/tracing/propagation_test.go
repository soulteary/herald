package tracing

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	common_tracing "github.com/soulteary/tracing-kit/v2"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestMiddlewareJoinsIncomingTrace sends a W3C traceparent header through a
// real Fiber request and checks that the server span continues that trace.
// Fiber hands header names to the middleware in normalized form
// ("Traceparent"), so a case-sensitive lookup of "traceparent" starts a new
// trace instead.
func TestMiddlewareJoinsIncomingTrace(t *testing.T) {
	const (
		traceID      = "4bf92f3577b34da6a3ce929d0e0e4736"
		parentSpanID = "00f067aa0ba902b7"
	)

	oldPropagator := otel.GetTextMapPropagator()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	common_tracing.Install(tp, "propagation-test")
	common_tracing.SetDefaultPropagator()
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		common_tracing.Uninstall()
		otel.SetTextMapPropagator(oldPropagator)
	})

	app := fiber.New()
	app.Use(TracingMiddleware("propagation-test"))
	app.Get("/test", func(c fiber.Ctx) error { return c.SendString("OK") })

	for _, header := range []string{"traceparent", "Traceparent", "TRACEPARENT"} {
		t.Run(header, func(t *testing.T) {
			before := len(recorder.Ended())
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set(header, "00-"+traceID+"-"+parentSpanID+"-01")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test() error = %v", err)
			}
			_ = resp.Body.Close()

			spans := recorder.Ended()
			if len(spans) != before+1 {
				t.Fatalf("recorded %d new spans, want 1", len(spans)-before)
			}
			span := spans[len(spans)-1]
			if got := span.SpanContext().TraceID().String(); got != traceID {
				t.Errorf("span trace ID = %s, want incoming trace ID %s", got, traceID)
			}
			if got := span.Parent().SpanID().String(); got != parentSpanID {
				t.Errorf("span parent ID = %s, want incoming span ID %s", got, parentSpanID)
			}
			if !span.Parent().IsRemote() {
				t.Error("span parent is not marked remote")
			}
			if got := resp.Header.Get("Traceparent"); len(got) < 35 || got[3:35] != traceID {
				t.Errorf("response traceparent = %q, want trace ID %s", got, traceID)
			}
		})
	}
}
