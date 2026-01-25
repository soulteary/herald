package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/router"
	rediskit "github.com/soulteary/redis-kit/client"
	"github.com/soulteary/tracing-kit"
)

func main() {
	// Initialize configuration
	if err := config.Initialize(); err != nil {
		logrus.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Set log level
	setLogLevel(config.LogLevel)

	// Initialize OpenTelemetry tracing if enabled
	if config.OTLPEnabled {
		_, err := tracing.InitTracer(
			config.ServiceName,
			config.Version,
			config.OTLPEndpoint,
		)
		if err != nil {
			logrus.Warnf("Failed to initialize OpenTelemetry tracing: %v", err)
		} else {
			logrus.Info("OpenTelemetry tracing initialized")
			// Setup graceful shutdown for tracer
			go func() {
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				<-sigChan
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					logrus.Errorf("Failed to shutdown tracer: %v", err)
				}
			}()
		}
	}

	// Initialize Redis client for router
	cfg := rediskit.DefaultConfig().
		WithAddr(config.RedisAddr).
		WithPassword(config.RedisPassword).
		WithDB(config.RedisDB)

	redisClient, err := rediskit.NewClient(cfg)
	if err != nil {
		logrus.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Create and start server
	routerWithHandlers := router.NewRouterWithClientAndHandlers(redisClient)
	app := routerWithHandlers.App
	port := config.GetPort()

	// Check if TLS is configured
	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		logrus.Infof("Herald service starting with TLS on port %s", port)

		// Load server certificate
		cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			logrus.Fatalf("Failed to load TLS certificate: %v", err)
		}

		// Configure TLS
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		// Configure mTLS if CA certificate is provided
		if config.TLSCACertFile != "" {
			logrus.Info("mTLS enabled: client certificate verification required")

			// Load CA certificate for client verification
			caCert, err := os.ReadFile(config.TLSCACertFile)
			if err != nil {
				logrus.Fatalf("Failed to read CA certificate: %v", err)
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				logrus.Fatalf("Failed to parse CA certificate")
			}

			tlsConfig.ClientCAs = caCertPool
			// Require client certificates (but allow fallback to HMAC/API Key if not provided)
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}

		// Start server with TLS
		ln, err := tls.Listen("tcp", port, tlsConfig)
		if err != nil {
			logrus.Fatalf("Failed to start TLS listener: %v", err)
		}

		// Setup graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		serverErr := make(chan error, 1)
		go func() {
			serverErr <- app.Listener(ln)
		}()

		// Wait for server error or shutdown signal
		select {
		case err := <-serverErr:
			if err != nil {
				logrus.Fatalf("Failed to start server: %v", err)
			}
		case sig := <-sigChan:
			logrus.Infof("Received signal: %v, shutting down gracefully...", sig)

			// Shutdown audit writer
			if routerWithHandlers.Handlers != nil {
				if err := routerWithHandlers.Handlers.StopAuditWriter(); err != nil {
					logrus.Errorf("Failed to shutdown audit writer: %v", err)
				}
			}

			// Shutdown tracer
			if config.OTLPEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					logrus.Errorf("Failed to shutdown tracer: %v", err)
				}
			}

			logrus.Info("Herald service stopped")
		}
	} else {
		logrus.Infof("Herald service starting on port %s", port)

		// Setup graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		serverErr := make(chan error, 1)
		go func() {
			serverErr <- app.Listen(port)
		}()

		// Wait for server error or shutdown signal
		select {
		case err := <-serverErr:
			if err != nil {
				logrus.Fatalf("Failed to start server: %v", err)
			}
		case sig := <-sigChan:
			logrus.Infof("Received signal: %v, shutting down gracefully...", sig)

			// Shutdown audit writer
			if routerWithHandlers.Handlers != nil {
				if err := routerWithHandlers.Handlers.StopAuditWriter(); err != nil {
					logrus.Errorf("Failed to shutdown audit writer: %v", err)
				}
			}

			// Shutdown tracer
			if config.OTLPEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					logrus.Errorf("Failed to shutdown tracer: %v", err)
				}
			}

			logrus.Info("Herald service stopped")
		}
	}
}

// setLogLevel sets the log level based on configuration
func setLogLevel(level string) {
	switch strings.ToLower(level) {
	case "trace":
		logrus.SetLevel(logrus.TraceLevel)
	case "debug":
		logrus.SetLevel(logrus.DebugLevel)
	case "info":
		logrus.SetLevel(logrus.InfoLevel)
	case "warn", "warning":
		logrus.SetLevel(logrus.WarnLevel)
	case "error":
		logrus.SetLevel(logrus.ErrorLevel)
	case "fatal":
		logrus.SetLevel(logrus.FatalLevel)
	case "panic":
		logrus.SetLevel(logrus.PanicLevel)
	default:
		logrus.SetLevel(logrus.InfoLevel)
	}
}
