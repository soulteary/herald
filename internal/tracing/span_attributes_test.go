package tracing

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	common_tracing "github.com/soulteary/tracing-kit/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestServerSpanOmitsClientAddress checks that the request span does not
// export the client address. Other spans identify callers by peppered digest
// only, and the middleware has no digester.
func TestServerSpanOmitsClientAddress(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	common_tracing.Install(tp, "client-address-test")
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		common_tracing.Uninstall()
	})

	var clientIP string
	app := fiber.New()
	app.Use(TracingMiddleware("client-address-test"))
	app.Get("/test", func(c fiber.Ctx) error {
		clientIP = c.IP()
		return c.SendString("OK")
	})
	resp, err := app.Test(httptest.NewRequest("GET", "/test", nil))
	if err != nil {
		t.Fatalf("app.Test() error = %v", err)
	}
	_ = resp.Body.Close()

	if clientIP == "" {
		t.Fatal("test request has no client address to look for")
	}
	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	for _, attr := range spans[0].Attributes() {
		if attr.Key == "http.remote_addr" || attr.Value.String() == clientIP {
			t.Errorf("span attribute %s exports the client address %q", attr.Key, clientIP)
		}
	}
}
