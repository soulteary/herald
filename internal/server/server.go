// Package server owns Herald's HTTP lifecycle: it builds a TLS or plaintext
// listener from config, starts serving, and performs an ordered graceful
// shutdown (stop accepting -> drain in-flight -> flush audit writer -> shutdown
// tracer). Startup errors are returned to the caller instead of calling
// log.Fatal deep in the stack, so main can decide how to exit and tests can
// assert on failures.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	logger "github.com/soulteary/logger-kit"
)

// ShutdownHook runs during graceful shutdown. Hooks run in registration order
// after the HTTP server has stopped accepting new connections.
type ShutdownHook func(ctx context.Context) error

// Config describes how to start the server.
type Config struct {
	// Addr is the listen address (e.g. ":8080").
	Addr string

	// TLSCertFile / TLSKeyFile enable TLS when both are set.
	TLSCertFile string
	TLSKeyFile  string
	// TLSClientCAFile enables mTLS (client certificate verification) when set.
	// It requires TLSCertFile/TLSKeyFile.
	TLSClientCAFile string

	// ShutdownTimeout bounds the graceful drain window.
	ShutdownTimeout time.Duration

	Logger *logger.Logger
}

// Server wraps a Fiber app plus its lifecycle configuration.
type Server struct {
	app   *fiber.App
	cfg   Config
	hooks []ShutdownHook
}

// New validates the configuration and returns a Server. It returns an error
// (rather than aborting) for any misconfiguration so the caller controls exit.
func New(app *fiber.App, cfg Config) (*Server, error) {
	if app == nil {
		return nil, errors.New("server: nil fiber app")
	}
	if cfg.Addr == "" {
		return nil, errors.New("server: empty listen address")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return nil, errors.New("server: half-configured TLS (cert without key or vice versa)")
	}
	if cfg.TLSClientCAFile != "" && (cfg.TLSCertFile == "" || cfg.TLSKeyFile == "") {
		return nil, errors.New("server: client CA configured without server certificate/key")
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}
	return &Server{app: app, cfg: cfg}, nil
}

// OnShutdown registers a hook to run during graceful shutdown, after the HTTP
// server stops accepting connections.
func (s *Server) OnShutdown(h ShutdownHook) {
	if h != nil {
		s.hooks = append(s.hooks, h)
	}
}

// TLSEnabled reports whether the server will terminate TLS.
func (s *Server) TLSEnabled() bool {
	return s.cfg.TLSCertFile != "" && s.cfg.TLSKeyFile != ""
}

// listener builds the appropriate net.Listener (TLS or plaintext). It is split
// out so startup errors (bad cert, unusable port) are returned to the caller.
func (s *Server) listener() (net.Listener, error) {
	if !s.TLSEnabled() {
		ln, err := net.Listen("tcp", s.cfg.Addr)
		if err != nil {
			return nil, fmt.Errorf("server: listen %s: %w", s.cfg.Addr, err)
		}
		return ln, nil
	}

	cert, err := tls.LoadX509KeyPair(s.cfg.TLSCertFile, s.cfg.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("server: load TLS keypair: %w", err)
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}
	if s.cfg.TLSClientCAFile != "" {
		caCert, err := os.ReadFile(s.cfg.TLSClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("server: read client CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("server: failed to parse client CA certificate")
		}
		tlsCfg.ClientCAs = pool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}
	ln, err := tls.Listen("tcp", s.cfg.Addr, tlsCfg)
	if err != nil {
		return nil, fmt.Errorf("server: TLS listen %s: %w", s.cfg.Addr, err)
	}
	return ln, nil
}

// Run starts the server and blocks until ctx is cancelled or the server exits
// with an error. On ctx cancellation it performs an ordered graceful shutdown.
// It returns nil on a clean shutdown.
func (s *Server) Run(ctx context.Context) error {
	ln, err := s.listener()
	if err != nil {
		return err
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.app.Listener(ln)
	}()

	select {
	case err := <-serveErr:
		// Server stopped on its own; a nil error means a normal Shutdown.
		if err != nil {
			return fmt.Errorf("server: serve: %w", err)
		}
		return nil
	case <-ctx.Done():
		return s.shutdown(serveErr)
	}
}

// shutdown stops accepting connections, drains in-flight requests, and runs
// shutdown hooks in order within the configured timeout.
func (s *Server) shutdown(serveErr <-chan error) error {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Info().Msg("Shutting down gracefully...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownTimeout)
	defer cancel()

	// Stop accepting new connections and drain in-flight requests first.
	if err := s.app.ShutdownWithContext(shutdownCtx); err != nil && s.cfg.Logger != nil {
		s.cfg.Logger.Error().Err(err).Msg("HTTP graceful shutdown error")
	}

	// Wait for the serve goroutine to return so we don't run hooks while the
	// server is still touching shared resources.
	<-serveErr

	// Run shutdown hooks (audit writer flush, tracer shutdown, ...) in order.
	var firstErr error
	for _, h := range s.hooks {
		if err := h(shutdownCtx); err != nil {
			if s.cfg.Logger != nil {
				s.cfg.Logger.Error().Err(err).Msg("shutdown hook error")
			}
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if s.cfg.Logger != nil {
		s.cfg.Logger.Info().Msg("Herald service stopped")
	}
	return firstErr
}
