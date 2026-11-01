package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/config"
)

func TestHealthcheckProbeResultsAndTrustFailures(t *testing.T) {
	oldPort, oldCert, oldKey := config.Port, config.TLSCertFile, config.TLSKeyFile
	oldCA, oldName := config.HealthcheckTLSCAFile, config.HealthcheckTLSServerName
	oldClientCert, oldClientKey := config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile
	t.Cleanup(func() {
		config.Port, config.TLSCertFile, config.TLSKeyFile = oldPort, oldCert, oldKey
		config.HealthcheckTLSCAFile, config.HealthcheckTLSServerName = oldCA, oldName
		config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile = oldClientCert, oldClientKey
	})
	config.TLSCertFile, config.TLSKeyFile = "", ""
	config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile = "", ""
	for _, status := range []int{200, 503} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/livez" {
				t.Errorf("probe path = %s", r.URL.Path)
			}
			w.WriteHeader(status)
		}))
		config.Port = strings.TrimPrefix(s.URL, "http://127.0.0.1:")
		want := 0
		if status != 200 {
			want = 1
		}
		if got := runHealthcheck(); got != want {
			t.Fatalf("status %d: exit %d", status, got)
		}
		s.Close()
		if got := runHealthcheck(); got != 1 {
			t.Fatal("closed listener reported healthy")
		}
	}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer s.Close()
	config.Port = strings.TrimPrefix(s.URL, "https://127.0.0.1:")
	config.TLSCertFile, config.TLSKeyFile = "configured.crt", "configured.key"
	dir := t.TempDir()
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	config.HealthcheckTLSCAFile, config.HealthcheckTLSServerName = ca, "example.com"
	if got := runHealthcheck(); got != 0 {
		t.Fatal("trusted TLS probe failed")
	}
	for _, bad := range []string{filepath.Join(dir, "absent.pem"), filepath.Join(dir, "bad.pem")} {
		if strings.HasSuffix(bad, "bad.pem") {
			if err := os.WriteFile(bad, []byte("invalid CA"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		config.HealthcheckTLSCAFile = bad
		if got := runHealthcheck(); got != 1 {
			t.Fatal("invalid trust material accepted")
		}
	}
	config.HealthcheckTLSCAFile = ca
	config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile = "missing.crt", "missing.key"
	if _, _, err := newHealthcheckClient(); err == nil {
		t.Fatal("invalid client identity accepted")
	}
}
