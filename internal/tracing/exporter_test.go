package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	common_tracing "github.com/soulteary/tracing-kit/v2"
	"go.opentelemetry.io/otel"
)

func TestParseEndpoint(t *testing.T) {
	accepted := []struct {
		raw  string
		want Endpoint
	}{
		{"localhost:4318", Endpoint{Host: "localhost:4318", Path: "/v1/traces", Insecure: true}},
		{" collector.internal:4318 ", Endpoint{Host: "collector.internal:4318", Path: "/v1/traces", Insecure: true}},
		{"collector", Endpoint{Host: "collector", Path: "/v1/traces", Insecure: true}},
		{"[::1]:4318", Endpoint{Host: "[::1]:4318", Path: "/v1/traces", Insecure: true}},
		{"http://localhost:4318", Endpoint{Host: "localhost:4318", Path: "/v1/traces", Insecure: true}},
		{"http://localhost:4318/", Endpoint{Host: "localhost:4318", Path: "/v1/traces", Insecure: true}},
		{"http://localhost:4318/v1/traces", Endpoint{Host: "localhost:4318", Path: "/v1/traces", Insecure: true}},
		{"https://otel.example.com", Endpoint{Host: "otel.example.com", Path: "/v1/traces", Insecure: false}},
		{"https://otel.example.com:443/otlp/v1/traces", Endpoint{Host: "otel.example.com:443", Path: "/otlp/v1/traces", Insecure: false}},
	}
	for _, tc := range accepted {
		got, err := ParseEndpoint(tc.raw)
		if err != nil || got != tc.want {
			t.Errorf("ParseEndpoint(%q) = %+v, %v; want %+v", tc.raw, got, err, tc.want)
		}
	}
	if got := (Endpoint{Host: "otel.example.com:443", Path: "/otlp/v1/traces"}).String(); got != "https://otel.example.com:443/otlp/v1/traces" {
		t.Errorf("String() = %q", got)
	}

	refused := []string{
		"", "   ", "grpc://localhost:4317", "ftp://host/x", "http://", "http:///v1/traces",
		"http://user:secret-password@collector:4318", "http://collector:4318?x=1", "http://collector:4318/#frag",
		"localhost:4318/v1/traces", "localhost:", "localhost:port", "localhost:70000", ":4318", "::1", "a b:4318",
		"http://collector:notaport",
	}
	for _, raw := range refused {
		got, err := ParseEndpoint(raw)
		if err == nil {
			t.Errorf("ParseEndpoint(%q) = %+v, want an error", raw, got)
			continue
		}
		if strings.Contains(err.Error(), "secret-password") {
			t.Errorf("error for %q repeats the value: %v", raw, err)
		}
	}
}

// TestInitTracerExportsForEveryEndpointForm sends one span through each form
// OTLP_ENDPOINT may take and checks where the collector received it. The URL
// form is the one the documentation has always shown; passed on unchanged it
// produced the request URL "http://http:%2F%2F...", and nothing was exported.
func TestInitTracerExportsForEveryEndpointForm(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	hostPort := strings.TrimPrefix(collector.URL, "http://")

	oldPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		common_tracing.Uninstall()
		otel.SetTextMapPropagator(oldPropagator)
	})

	for _, tc := range []struct{ form, value, wantPath string }{
		{"host:port", hostPort, "POST /v1/traces"},
		{"URL", collector.URL, "POST /v1/traces"},
		{"URL with trailing slash", collector.URL + "/", "POST /v1/traces"},
		{"URL with path", collector.URL + "/otlp/v1/traces", "POST /otlp/v1/traces"},
	} {
		t.Run(tc.form, func(t *testing.T) {
			mu.Lock()
			paths = nil
			mu.Unlock()

			endpoint, err := ParseEndpoint(tc.value)
			if err != nil {
				t.Fatalf("ParseEndpoint(%q): %v", tc.value, err)
			}
			tp, err := InitTracer("endpoint-test", "test", endpoint)
			if err != nil {
				t.Fatalf("InitTracer: %v", err)
			}
			_, span := common_tracing.GetTracer().Start(context.Background(), "probe")
			span.End()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := tp.ForceFlush(ctx); err != nil {
				t.Fatalf("export through %q failed: %v", tc.value, err)
			}
			_ = tp.Shutdown(ctx)

			mu.Lock()
			defer mu.Unlock()
			if len(paths) != 1 || paths[0] != tc.wantPath {
				t.Fatalf("collector received %v, want one %q", paths, tc.wantPath)
			}
		})
	}

	if _, err := InitTracer("endpoint-test", "test", Endpoint{}); err == nil {
		t.Fatal("InitTracer accepted an endpoint without a host")
	}
}
