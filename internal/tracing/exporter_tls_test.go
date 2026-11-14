package tracing

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	common_tracing "github.com/soulteary/tracing-kit/v2"
	"go.opentelemetry.io/otel"
)

// tlsCollector is an OTLP/HTTP collector that serves TLS only and records
// each request it receives together with whether it arrived over TLS.
type tlsCollector struct {
	server *httptest.Server
	mu     sync.Mutex
	seen   []string
}

func newTLSCollector(t *testing.T) *tlsCollector {
	t.Helper()
	c := &tlsCollector{}
	c.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		transport := "plain"
		if r.TLS != nil && r.TLS.HandshakeComplete {
			transport = "tls"
		}
		c.mu.Lock()
		c.seen = append(c.seen, transport+" "+r.Method+" "+r.URL.Path)
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	// A refused handshake is expected in the untrusted case; keep the
	// server's report of it out of the test output.
	c.server.Config.ErrorLog = log.New(io.Discard, "", 0)
	c.server.StartTLS()
	t.Cleanup(c.server.Close)
	return c
}

func (c *tlsCollector) requests() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.seen...)
}

// writeCertificatePEM stores the collector's certificate as a PEM file and
// returns its path.
func (c *tlsCollector) writeCertificatePEM(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collector-ca.pem")
	block := &pem.Block{Type: "CERTIFICATE", Bytes: c.server.Certificate().Raw}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write certificate: %v", err)
	}
	return path
}

// exportOneSpanOverHTTPS runs the startup path for an https:// OTLP_ENDPOINT
// (ParseEndpoint, then InitTracer), ends one span and forces a flush. It
// returns the flush error.
func exportOneSpanOverHTTPS(t *testing.T, endpointURL string) error {
	t.Helper()
	oldPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		common_tracing.Uninstall()
		otel.SetTextMapPropagator(oldPropagator)
	})

	endpoint, err := ParseEndpoint(endpointURL)
	if err != nil {
		t.Fatalf("ParseEndpoint(%q): %v", endpointURL, err)
	}
	if endpoint.Insecure {
		t.Fatalf("ParseEndpoint(%q) = %+v, want a TLS endpoint", endpointURL, endpoint)
	}
	tp, err := InitTracer("endpoint-tls-test", "test", endpoint)
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	_, span := common_tracing.GetTracer().Start(context.Background(), "probe")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	flushErr := tp.ForceFlush(ctx)
	_ = tp.Shutdown(ctx)
	return flushErr
}

// clearOTLPCertificateEnv makes sure the exporter verifies the collector
// against the system roots only, whatever the environment the tests run in.
func clearOTLPCertificateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "")
}

// TestInitTracerExportsOverHTTPS sends one span through the https:// form of
// OTLP_ENDPOINT to a collector that serves TLS only.
//
// The collector's certificate is self-signed, so it has to be made trusted
// for the exporter. Production code is left as it is: the certificate is
// named by OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE, which the OTLP/HTTP
// exporter reads itself and uses as its root pool. Unlike SSL_CERT_FILE this
// does not depend on the platform's verifier, so the test runs on macOS and
// Linux alike. Verification stays on: the next test shows that the same
// export fails without the variable.
func TestInitTracerExportsOverHTTPS(t *testing.T) {
	collector := newTLSCollector(t)
	if !strings.HasPrefix(collector.server.URL, "https://") {
		t.Fatalf("collector URL %q is not https", collector.server.URL)
	}
	clearOTLPCertificateEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", collector.writeCertificatePEM(t))

	if err := exportOneSpanOverHTTPS(t, collector.server.URL); err != nil {
		t.Fatalf("export through %q failed: %v", collector.server.URL, err)
	}
	got := collector.requests()
	if len(got) != 1 || got[0] != "tls POST /v1/traces" {
		t.Fatalf("collector received %v, want one %q", got, "tls POST /v1/traces")
	}
}

// TestInitTracerDoesNotExportToUntrustedHTTPSCollector proves the https://
// form verifies the collector's certificate: with a certificate no root
// vouches for, the flush reports a certificate verification failure, the
// collector receives no request, and nothing panics.
func TestInitTracerDoesNotExportToUntrustedHTTPSCollector(t *testing.T) {
	collector := newTLSCollector(t)
	clearOTLPCertificateEnv(t)

	err := exportOneSpanOverHTTPS(t, collector.server.URL)
	if err == nil {
		t.Fatal("export to a collector with an untrusted certificate reported no error")
	}
	var verification *tls.CertificateVerificationError
	if !errors.As(err, &verification) {
		t.Fatalf("flush error is not a certificate verification failure: %T %v", err, err)
	}
	if got := collector.requests(); len(got) != 0 {
		t.Fatalf("collector received %v, want no request", got)
	}
}
