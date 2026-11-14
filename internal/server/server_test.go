package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func newApp() *fiber.App {
	app := fiber.New()
	app.Get("/ping", func(c fiber.Ctx) error { return c.SendString("pong") })
	return app
}

func TestNew_RejectsHalfConfiguredTLS(t *testing.T) {
	_, err := New(newApp(), Config{Addr: ":0", TLSCertFile: "cert.pem"})
	if err == nil {
		t.Fatal("expected error for cert without key")
	}
	_, err = New(newApp(), Config{Addr: ":0", TLSKeyFile: "key.pem"})
	if err == nil {
		t.Fatal("expected error for key without cert")
	}
}

func TestNew_RejectsClientCAWithoutServerCert(t *testing.T) {
	_, err := New(newApp(), Config{Addr: ":0", TLSClientCAFile: "ca.pem", ClientCertMode: "require"})
	if err == nil {
		t.Fatal("expected error for client CA without server cert/key")
	}
}

func TestNew_ValidatesClientCertMode(t *testing.T) {
	tests := []Config{
		{Addr: ":0", ClientCertMode: "unknown"},
		{Addr: ":0", ClientCertMode: "optional"},
		{Addr: ":0", ClientCertMode: "require"},
		{Addr: ":0", ClientCertMode: "off", TLSClientCAFile: "ca.pem"},
	}
	for _, cfg := range tests {
		if _, err := New(newApp(), cfg); err == nil {
			t.Errorf("New(%+v) should reject invalid client certificate configuration", cfg)
		}
	}
}

func TestClientAuthType(t *testing.T) {
	if got := clientAuthType("off"); got != tls.NoClientCert {
		t.Errorf("off = %v, want NoClientCert", got)
	}
	if got := clientAuthType("optional"); got != tls.VerifyClientCertIfGiven {
		t.Errorf("optional = %v, want VerifyClientCertIfGiven", got)
	}
	if got := clientAuthType("require"); got != tls.RequireAndVerifyClientCert {
		t.Errorf("require = %v, want RequireAndVerifyClientCert", got)
	}
}

func TestRunAll_RejectsEmptyGroup(t *testing.T) {
	if err := RunAll(context.Background()); err == nil {
		t.Fatal("expected error for empty server group")
	}
}

func TestNew_RejectsEmptyAddr(t *testing.T) {
	if _, err := New(newApp(), Config{}); err == nil {
		t.Fatal("expected error for empty addr")
	}
}

func TestNew_EnforcesLoopbackOnly(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:8080", ":8080", "192.0.2.10:8080"} {
		if _, err := New(newApp(), Config{Addr: addr, LoopbackOnly: true}); err == nil {
			t.Errorf("expected %q to be rejected", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0", "localhost:0"} {
		if _, err := New(newApp(), Config{Addr: addr, LoopbackOnly: true}); err != nil {
			t.Errorf("expected %q to be accepted: %v", addr, err)
		}
	}
}

func TestRun_GracefulShutdownRunsHooksInOrder(t *testing.T) {
	srv, err := New(newApp(), Config{Addr: "127.0.0.1:0", ShutdownTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var mu sync.Mutex
	var order []string
	srv.OnShutdown(func(context.Context) error {
		mu.Lock()
		order = append(order, "audit")
		mu.Unlock()
		return nil
	})
	srv.OnShutdown(func(context.Context) error {
		mu.Lock()
		order = append(order, "tracer")
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Give the server a moment to start, then trigger shutdown.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 2 || order[0] != "audit" || order[1] != "tracer" {
		t.Fatalf("hooks ran out of order: %v", order)
	}
}

func TestRun_ListenerErrorReturned(t *testing.T) {
	// An invalid address should make listener() fail and Run return the error
	// instead of aborting the process.
	srv, err := New(newApp(), Config{Addr: "invalid-host-no-port"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Run(context.Background()); err == nil {
		t.Fatal("expected Run to return a listener error")
	}
}

func TestRun_ServesRequests(t *testing.T) {
	// A port the system reports as free, not a fixed number: a fixed port can
	// be in use on the machine that runs the tests, and the request then
	// reaches whatever listens there.
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free port: %v", err)
	}
	addr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("release the probed port: %v", err)
	}

	srv, err := New(newApp(), Config{Addr: addr, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Run(ctx) }()

	// Poll until this server answers. The allowance only bounds a server that
	// never starts; a loaded machine can need more than a moment.
	var body []byte
	var lastErr error
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, getErr := http.Get("http://" + addr + "/ping")
		if getErr == nil {
			body, lastErr = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if lastErr == nil && string(body) == "pong" {
				break
			}
		} else {
			lastErr = getErr
		}
		time.Sleep(20 * time.Millisecond)
	}
	if string(body) != "pong" {
		cancel()
		t.Fatalf("the server did not answer /ping with pong: body %q, last error: %v", body, lastErr)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not stop")
	}
}
