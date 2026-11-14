package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	logger "github.com/soulteary/logger-kit/v3"
	tracing "github.com/soulteary/tracing-kit/v2"
	"go.opentelemetry.io/otel"

	"github.com/soulteary/herald/internal/config"
)

// TestInitTracingLogsInitializedOnlyWhenSpansCanBeExported runs the startup
// step for OTLP_ENABLED=true with an endpoint in the documented URL form, in
// the host:port form, empty and unusable. Only the first two may log
// "OpenTelemetry tracing initialized", and both must reach the collector.
// The empty case logs nothing here.
func TestInitTracingLogsInitializedOnlyWhenSpansCanBeExported(t *testing.T) {
	var hits atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/traces" {
			hits.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	oldEndpoint, oldPropagator := config.OTLPEndpoint, otel.GetTextMapPropagator()
	t.Cleanup(func() {
		config.OTLPEndpoint = oldEndpoint
		tracing.Uninstall()
		otel.SetTextMapPropagator(oldPropagator)
	})

	run := func(endpoint string) (bool, string) {
		t.Helper()
		var logs bytes.Buffer
		log := logger.New(logger.Config{Level: logger.InfoLevel, Format: logger.FormatJSON, Output: &logs})
		config.OTLPEndpoint = endpoint
		hits.Store(0)
		exporting := initTracing(log)
		_, span := tracing.GetTracer().Start(context.Background(), "probe")
		span.End()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = tracing.Shutdown(ctx)
		return exporting, logs.String()
	}

	for _, endpoint := range []string{collector.URL, strings.TrimPrefix(collector.URL, "http://")} {
		exporting, logs := run(endpoint)
		if !exporting || !strings.Contains(logs, "OpenTelemetry tracing initialized") {
			t.Errorf("OTLP_ENDPOINT=%q: exporting=%v, logs: %s", endpoint, exporting, logs)
		}
		if hits.Load() != 1 {
			t.Errorf("OTLP_ENDPOINT=%q: the collector received %d exports, want 1", endpoint, hits.Load())
		}
	}

	// The empty endpoint is reported by the config.otlp startup advisory, so
	// initTracing itself must stay silent about it; it used to repeat the
	// advisory as an info line.
	if exporting, logs := run(""); exporting || logs != "" {
		t.Errorf("OTLP_ENDPOINT empty: exporting=%v, want no log line from initTracing, got: %s", exporting, logs)
	}
	if hits.Load() != 0 {
		t.Errorf("OTLP_ENDPOINT empty: the collector received %d exports", hits.Load())
	}

	for _, tc := range []struct{ endpoint, wantLog string }{
		{"grpc://collector:4317", "OTLP_ENDPOINT cannot be used"},
		{"http://user:secret-password@collector:4318", "OTLP_ENDPOINT cannot be used"},
	} {
		exporting, logs := run(tc.endpoint)
		if exporting || strings.Contains(logs, "OpenTelemetry tracing initialized") {
			t.Errorf("OTLP_ENDPOINT=%q: reported as initialized (exporting=%v): %s", tc.endpoint, exporting, logs)
		}
		if !strings.Contains(logs, tc.wantLog) {
			t.Errorf("OTLP_ENDPOINT=%q: log does not contain %q: %s", tc.endpoint, tc.wantLog, logs)
		}
		if strings.Contains(logs, "secret-password") {
			t.Errorf("the log repeats the endpoint value: %s", logs)
		}
		if hits.Load() != 0 {
			t.Errorf("OTLP_ENDPOINT=%q: the collector received %d exports", tc.endpoint, hits.Load())
		}
	}
}
