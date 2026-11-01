package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logger "github.com/soulteary/logger-kit/v3"
)

func serverCertificateFiles(t *testing.T) (string, string) {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestTLSListenerPolicyAndFailures(t *testing.T) {
	cert, key := serverCertificateFiles(t)
	for _, mode := range []string{"off", "optional", "require"} {
		cfg := Config{Addr: "127.0.0.1:0", TLSCertFile: cert, TLSKeyFile: key, ClientCertMode: mode}
		if mode != "off" {
			cfg.TLSClientCAFile = cert
		}
		s, err := New(newApp(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		ln, err := s.listener()
		if err != nil {
			t.Fatal(err)
		}
		accepted := make(chan error, 1)
		go func() {
			conn, err := ln.Accept()
			if err == nil {
				err = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}
			accepted <- err
		}()
		identity, err := tls.LoadX509KeyPair(cert, key)
		if err != nil {
			t.Fatal(err)
		}
		// This client trusts the local test fixture by explicit test-only bypass.
		conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{InsecureSkipVerify: true, Certificates: []tls.Certificate{identity}, MinVersion: tls.VersionTLS12})
		if err != nil {
			t.Fatal(err)
		}
		_ = conn.Close()
		if err := <-accepted; err != nil {
			t.Fatal(err)
		}
		_ = ln.Close()
	}
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = busy.Close() }()
	badCA := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(badCA, []byte("bad CA"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Config{
		{Addr: "127.0.0.1:0", TLSCertFile: "missing", TLSKeyFile: "missing"},
		{Addr: "127.0.0.1:0", TLSCertFile: cert, TLSKeyFile: key, TLSClientCAFile: "missing", ClientCertMode: "require"},
		{Addr: "127.0.0.1:0", TLSCertFile: cert, TLSKeyFile: key, TLSClientCAFile: badCA, ClientCertMode: "require"},
		{Addr: busy.Addr().String(), TLSCertFile: cert, TLSKeyFile: key},
	} {
		s, err := New(newApp(), cfg)
		if err != nil {
			t.Fatal(err)
		}
		if ln, err := s.listener(); err == nil {
			_ = ln.Close()
			t.Fatal("invalid TLS listener accepted")
		}
	}
	if _, err := New(nil, Config{Addr: ":0"}); err == nil {
		t.Fatal("nil app accepted")
	}
	if _, err := New(newApp(), Config{Addr: ":0", TLSCertFile: cert, TLSKeyFile: key, TLSClientCAFile: cert}); err == nil {
		t.Fatal("CA accepted with client auth disabled")
	}
}

func TestRunGroupStartupFailureAndShutdownHookErrors(t *testing.T) {
	if err := RunAll(context.Background(), nil); err == nil {
		t.Fatal("nil group member accepted")
	}
	bad, err := New(newApp(), Config{Addr: "not a listen address"})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunAll(context.Background(), bad, bad); err == nil || !strings.Contains(err.Error(), "listen") {
		t.Fatalf("startup error lost: %v", err)
	}
	s, err := New(newApp(), Config{Addr: "127.0.0.1:0", Logger: logger.New(logger.Config{Level: logger.ErrorLevel})})
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("audit flush failed")
	order := []int{}
	s.OnShutdown(func(context.Context) error { order = append(order, 1); return want })
	s.OnShutdown(func(context.Context) error { order = append(order, 2); return errors.New("tracer failed") })
	stopped := make(chan error, 1)
	stopped <- nil
	if err := s.shutdown(stopped); !errors.Is(err, want) {
		t.Fatalf("first hook error lost: %v", err)
	}
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("hook ordering: %v", order)
	}
}
