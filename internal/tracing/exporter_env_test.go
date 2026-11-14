package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	common_tracing "github.com/soulteary/tracing-kit/v2"
	"go.opentelemetry.io/otel"
)

// recordingCollector is a plain-HTTP OTLP collector that records the path
// and two headers of each request it receives.
type recordingCollector struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []string
}

func newRecordingCollector(t *testing.T) *recordingCollector {
	t.Helper()
	c := &recordingCollector{}
	c.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		c.seen = append(c.seen, r.Method+" "+r.URL.Path+
			" authorization="+r.Header.Get("Authorization")+" x-scope="+r.Header.Get("X-Scope"))
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.server.Close)
	return c
}

func (c *recordingCollector) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

// exportOneSpan runs ParseEndpoint and InitTracer for the value, ends one
// span, flushes and shuts the provider down, so that no export is in flight
// when it returns.
func exportOneSpan(t *testing.T, value string) {
	t.Helper()
	oldPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		common_tracing.Uninstall()
		otel.SetTextMapPropagator(oldPropagator)
	})
	endpoint, err := ParseEndpoint(value)
	if err != nil {
		t.Fatalf("ParseEndpoint(%q): %v", value, err)
	}
	tp, err := InitTracer("endpoint-env-test", "test", endpoint)
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	_, span := common_tracing.GetTracer().Start(context.Background(), "probe")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("export through %q failed: %v", value, err)
	}
	if err := tp.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

// clearOTLPExporterEnv removes every OTEL_EXPORTER_OTLP_* variable the
// exporter reads, so that a test sets exactly the ones it is about. The
// exporter treats an empty value as unset.
func clearOTLPExporterEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"ENDPOINT", "CERTIFICATE", "CLIENT_CERTIFICATE", "CLIENT_KEY",
		"INSECURE", "HEADERS", "COMPRESSION", "PROTOCOL", "TIMEOUT",
	} {
		t.Setenv("OTEL_EXPORTER_OTLP_"+name, "")
		t.Setenv("OTEL_EXPORTER_OTLP_TRACES_"+name, "")
	}
}

// TestOTELExporterEndpointVariablesDoNotMoveTheExport pins the documented
// statement that OTEL_EXPORTER_OTLP_ENDPOINT and
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT do not change where spans go: host and
// path come from OTLP_ENDPOINT. Both variables name a second collector with
// a different path; it must receive nothing.
func TestOTELExporterEndpointVariablesDoNotMoveTheExport(t *testing.T) {
	for _, name := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		t.Run(name, func(t *testing.T) {
			configured := newRecordingCollector(t)
			other := newRecordingCollector(t)
			clearOTLPExporterEnv(t)
			t.Setenv(name, other.server.URL+"/from-environment")

			exportOneSpan(t, configured.server.URL+"/otlp/v1/traces")

			want := "POST /otlp/v1/traces authorization= x-scope="
			if got := configured.requests(); len(got) != 1 || got[0] != want {
				t.Errorf("configured collector received %v, want one %q", got, want)
			}
			if got := other.requests(); len(got) != 0 {
				t.Errorf("collector named by %s received %v, want nothing", name, got)
			}
		})
	}
}

// TestOTELExporterHeadersAreSentToTheCollector pins the documented statement
// that headers given in OTEL_EXPORTER_OTLP_HEADERS reach the collector with
// URL-decoded values, and that OTEL_EXPORTER_OTLP_TRACES_HEADERS replaces
// the whole list rather than merging with it.
func TestOTELExporterHeadersAreSentToTheCollector(t *testing.T) {
	for _, tc := range []struct {
		name, general, traces, want string
	}{
		{
			name:    "general",
			general: "Authorization=Bearer%20general,X-Scope=tenant-a",
			want:    "POST /v1/traces authorization=Bearer general x-scope=tenant-a",
		},
		{
			name:    "trace-specific replaces general",
			general: "Authorization=Bearer%20general,X-Scope=tenant-a",
			traces:  "Authorization=Bearer%20traces",
			want:    "POST /v1/traces authorization=Bearer traces x-scope=",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := newRecordingCollector(t)
			clearOTLPExporterEnv(t)
			t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", tc.general)
			t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", tc.traces)

			exportOneSpan(t, collector.server.URL)

			if got := collector.requests(); len(got) != 1 || got[0] != tc.want {
				t.Errorf("collector received %v, want one %q", got, tc.want)
			}
		})
	}
}
