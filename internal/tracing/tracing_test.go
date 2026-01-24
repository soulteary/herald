package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// resetGlobalState resets the global tracer state for testing
func resetGlobalState() {
	tracerProvider = nil
	tracer = nil
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
}

// setupTestTracer creates a test tracer provider with in-memory exporter
func setupTestTracer(_ *testing.T) (*sdktrace.TracerProvider, *tracetest.InMemoryExporter) {
	resetGlobalState()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	tracerProvider = tp
	tracer = tp.Tracer("test")

	return tp, exporter
}

// teardownTestTracer cleans up test tracer
func teardownTestTracer() {
	if tracerProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tracerProvider.Shutdown(ctx) // nolint:errcheck // test cleanup
	}
	resetGlobalState()
}

func TestInitTracer(t *testing.T) {
	t.Run("with endpoint", func(t *testing.T) {
		defer teardownTestTracer()
		resetGlobalState()

		// Use a mock endpoint that won't actually connect
		// We'll use an invalid endpoint to test initialization logic
		// but we need to handle the error case separately
		endpoint := "http://localhost:4318"
		tp, err := InitTracer("test-service", "1.0.0", endpoint)

		// InitTracer will try to create an OTLP exporter, which may fail
		// if the endpoint is not reachable, but we're testing the logic
		if err != nil {
			// If exporter creation fails, that's expected for invalid endpoints
			// We can still test that the function handles it correctly
			if tp != nil {
				t.Error("InitTracer() should return nil provider on error")
			}
		} else {
			// If it succeeds (e.g., in CI with mock server), verify setup
			if tp == nil {
				t.Fatal("InitTracer() returned nil provider")
			}
			if tracerProvider == nil {
				t.Error("InitTracer() should set global tracerProvider")
			}
			if tracer == nil {
				t.Error("InitTracer() should set global tracer")
			}
			if !IsEnabled() {
				t.Error("IsEnabled() should return true after initialization")
			}
		}
	})

	t.Run("empty endpoint returns nil", func(t *testing.T) {
		defer teardownTestTracer()
		resetGlobalState()

		tp, err := InitTracer("test-service", "1.0.0", "")
		if err != nil {
			t.Errorf("InitTracer() with empty endpoint error = %v, want nil", err)
		}
		if tp != nil {
			t.Error("InitTracer() with empty endpoint should return nil provider")
		}
		if IsEnabled() {
			t.Error("IsEnabled() should return false when endpoint is empty")
		}
		// Should return noop tracer
		testTracer := GetTracer()
		if testTracer == nil {
			t.Error("GetTracer() should return noop tracer when not initialized")
		}
	})

	t.Run("invalid endpoint handles error", func(t *testing.T) {
		defer teardownTestTracer()
		resetGlobalState()

		// Use an invalid endpoint format
		endpoint := "invalid://endpoint"
		tp, err := InitTracer("test-service", "1.0.0", endpoint)

		// Should return error for invalid endpoint
		if err == nil {
			t.Log("InitTracer() with invalid endpoint may not error immediately (depends on OTLP client)")
		} else {
			if tp != nil {
				t.Error("InitTracer() should return nil provider on error")
			}
			if err.Error() == "" {
				t.Error("InitTracer() error message should not be empty")
			}
		}
	})
}

func TestShutdown(t *testing.T) {
	t.Run("with provider", func(t *testing.T) {
		defer resetGlobalState()
		_, _ = setupTestTracer(t)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := Shutdown(ctx)
		if err != nil {
			t.Errorf("Shutdown() error = %v, want nil", err)
		}
	})

	t.Run("without provider", func(t *testing.T) {
		defer resetGlobalState()
		resetGlobalState()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		err := Shutdown(ctx)
		if err != nil {
			t.Errorf("Shutdown() without provider error = %v, want nil", err)
		}
	})
}

func TestGetTracer(t *testing.T) {
	t.Run("initialized", func(t *testing.T) {
		defer teardownTestTracer()
		tp, _ := setupTestTracer(t)
		if tp == nil {
			t.Fatal("setupTestTracer() returned nil")
		}

		testTracer := GetTracer()
		if testTracer == nil {
			t.Error("GetTracer() should not return nil when initialized")
		}
		if !IsEnabled() {
			t.Error("IsEnabled() should return true when tracer is initialized")
		}
	})

	t.Run("not initialized", func(t *testing.T) {
		defer resetGlobalState()
		resetGlobalState()

		testTracer := GetTracer()
		if testTracer == nil {
			t.Error("GetTracer() should return noop tracer when not initialized")
		}
		if IsEnabled() {
			t.Error("IsEnabled() should return false when not initialized")
		}
	})
}

func TestIsEnabled(t *testing.T) {
	t.Run("true when initialized", func(t *testing.T) {
		defer teardownTestTracer()
		tp, _ := setupTestTracer(t)
		if tp == nil {
			t.Fatal("setupTestTracer() returned nil")
		}

		if !IsEnabled() {
			t.Error("IsEnabled() should return true when initialized")
		}
	})

	t.Run("false when no provider", func(t *testing.T) {
		defer resetGlobalState()
		resetGlobalState()
		tracerProvider = nil
		tracer = nil

		if IsEnabled() {
			t.Error("IsEnabled() should return false when no provider")
		}
	})

	t.Run("false when no tracer", func(t *testing.T) {
		defer resetGlobalState()
		resetGlobalState()
		// Set provider but not tracer
		tp, _ := setupTestTracer(t)
		if tp == nil {
			t.Fatal("setupTestTracer() returned nil")
		}
		tracer = nil

		if IsEnabled() {
			t.Error("IsEnabled() should return false when no tracer")
		}
	})
}

func TestInitTracer_GlobalState(t *testing.T) {
	defer teardownTestTracer()
	resetGlobalState()

	// Test that InitTracer sets global state correctly
	// We'll use an endpoint that may fail, but test the state management
	endpoint := "http://localhost:4318"
	tp, err := InitTracer("herald", "1.0.0", endpoint)

	if err == nil && tp != nil {
		// If initialization succeeds, verify global state
		if tracerProvider != tp {
			t.Error("InitTracer() should set global tracerProvider")
		}
		if tracer == nil {
			t.Error("InitTracer() should set global tracer")
		}
		if otel.GetTracerProvider() == nil {
			t.Error("InitTracer() should set global OTel tracer provider")
		}
		if otel.GetTextMapPropagator() == nil {
			t.Error("InitTracer() should set global text map propagator")
		}
	}
}
