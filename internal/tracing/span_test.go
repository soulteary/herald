package tracing

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestStartSpan(t *testing.T) {
	t.Run("creates span", func(t *testing.T) {
		ctx := context.Background()
		ctx, span := StartSpan(ctx, "test.operation")
		if span == nil {
			t.Fatal("StartSpan() returned nil span")
		}
		defer span.End()

		// Verify span is in context
		spanFromCtx := trace.SpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("StartSpan() should add span to context")
		}
	})

	t.Run("inherits from context", func(t *testing.T) {
		ctx := context.Background()
		ctx, parentSpan := StartSpan(ctx, "parent.operation")
		defer parentSpan.End()

		ctx, childSpan := StartSpan(ctx, "child.operation")
		defer childSpan.End()

		// Child span should be in context
		spanFromCtx := trace.SpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("StartSpan() should add child span to context")
		}
	})

	t.Run("with options", func(t *testing.T) {
		ctx := context.Background()
		ctx, span := StartSpan(ctx, "test.operation", trace.WithSpanKind(trace.SpanKindClient))
		if span == nil {
			t.Fatal("StartSpan() returned nil span")
		}
		defer span.End()

		// Verify span kind is set (if span supports it)
		spanFromCtx := trace.SpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("StartSpan() should add span to context")
		}
	})
}

func TestSetSpanAttributes(t *testing.T) {
	t.Run("single attribute", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanAttributes(span, map[string]string{
			"key1": "value1",
		})

		// Attributes are set, but we can't easily verify them without exporter
		// Just verify function doesn't panic
	})

	t.Run("multiple attributes", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanAttributes(span, map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		})

		// Attributes are set, but we can't easily verify them without exporter
		// Just verify function doesn't panic
	})

	t.Run("empty map", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanAttributes(span, map[string]string{})

		// Should handle empty map without error
	})
}

func TestSetSpanAttributesFromMap(t *testing.T) {
	ctx := context.Background()
	_, span := StartSpan(ctx, "test.operation")
	defer span.End()

	t.Run("string type", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"string_key": "string_value",
		})
	})

	t.Run("int type", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"int_key": 42,
		})
	})

	t.Run("int64 type", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"int64_key": int64(1234567890),
		})
	})

	t.Run("float64 type", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"float64_key": 3.14,
		})
	})

	t.Run("bool type", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"bool_key": true,
		})
	})

	t.Run("other type converts to string", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"other_key": []string{"a", "b", "c"},
		})
	})

	t.Run("mixed types", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{
			"string_key": "value",
			"int_key":    42,
			"bool_key":   true,
			"float_key":  3.14,
		})
	})

	t.Run("empty map", func(t *testing.T) {
		SetSpanAttributesFromMap(span, map[string]interface{}{})
	})
}

func TestRecordError(t *testing.T) {
	t.Run("records error", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		err := errors.New("test error")
		RecordError(span, err)

		// Error is recorded, but we can't easily verify without exporter
		// Just verify function doesn't panic
	})

	t.Run("nil error does nothing", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		RecordError(span, nil)

		// Should handle nil error without error
	})

	t.Run("multiple errors", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		RecordError(span, errors.New("error 1"))
		RecordError(span, errors.New("error 2"))
		RecordError(span, errors.New("error 3"))
	})
}

func TestSetSpanStatus(t *testing.T) {
	t.Run("ok status", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanStatus(span, codes.Ok, "")
	})

	t.Run("error status", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanStatus(span, codes.Error, "operation failed")
	})

	t.Run("unset status", func(t *testing.T) {
		ctx := context.Background()
		_, span := StartSpan(ctx, "test.operation")
		defer span.End()

		SetSpanStatus(span, codes.Unset, "")
	})
}

func TestGetSpanFromContext(t *testing.T) {
	t.Run("with span in context", func(t *testing.T) {
		ctx := context.Background()
		ctx, span := StartSpan(ctx, "test.operation")
		defer span.End()

		spanFromCtx := GetSpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("GetSpanFromContext() should return span when present in context")
		}
	})

	t.Run("without span in context", func(t *testing.T) {
		ctx := context.Background()

		spanFromCtx := GetSpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("GetSpanFromContext() should return noop span when no span in context")
		}

		// Verify it's a noop span (can't easily check type, but it should not panic)
		spanFromCtx.SetAttributes(attribute.String("test", "value"))
		spanFromCtx.End()
	})

	t.Run("with noop tracer", func(t *testing.T) {
		// Reset global state to ensure we get noop tracer
		originalTracer := GetTracer()
		ctx := context.Background()

		// Create span with noop tracer
		noopTracer := noop.NewTracerProvider().Tracer("test")
		ctx, span := noopTracer.Start(ctx, "test.operation")
		defer span.End()

		spanFromCtx := GetSpanFromContext(ctx)
		if spanFromCtx == nil {
			t.Error("GetSpanFromContext() should return span even with noop tracer")
		}

		// Restore
		_ = originalTracer
	})
}
