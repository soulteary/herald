package tracing

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestHTTPSEndpointIsNotDowngradedByTheEnvironment configures an https://
// OTLP_ENDPOINT whose host serves plain HTTP only, together with each
// environment setting that the OTLP exporter reads as "use plain HTTP". The
// scheme of OTLP_ENDPOINT must decide: the export fails, and nothing is sent
// in plain text.
func TestHTTPSEndpointIsNotDowngradedByTheEnvironment(t *testing.T) {
	for name, env := range map[string][2]string{
		"OTEL_EXPORTER_OTLP_INSECURE":        {"OTEL_EXPORTER_OTLP_INSECURE", "true"},
		"OTEL_EXPORTER_OTLP_TRACES_INSECURE": {"OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true"},
		"OTEL_EXPORTER_OTLP_ENDPOINT":        {"OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.invalid:4318"},
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": {"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://collector.invalid:4318/v1/traces"},
	} {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.Method+" "+r.URL.Path)
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(plain.Close)

			clearOTLPCertificateEnv(t)
			for _, key := range []string{
				"OTEL_EXPORTER_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_TRACES_INSECURE",
				"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
			} {
				t.Setenv(key, "")
			}
			t.Setenv(env[0], env[1])

			endpointURL := "https://" + strings.TrimPrefix(plain.URL, "http://")
			if err := exportOneSpanOverHTTPS(t, endpointURL); err == nil {
				t.Fatal("the export to a plain-HTTP host through an https:// endpoint reported no error")
			}
			mu.Lock()
			defer mu.Unlock()
			if len(seen) != 0 {
				t.Fatalf("the plain-HTTP server received %v, want no request", seen)
			}
		})
	}
}
