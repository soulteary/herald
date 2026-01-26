package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"os/signal"
	"syscall"
	"time"

	logger "github.com/soulteary/logger-kit"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/router"
	rediskit "github.com/soulteary/redis-kit/client"
	"github.com/soulteary/tracing-kit"
)

// log is the global logger instance
var log *logger.Logger

func main() {
	// Initialize logger using logger-kit
	log = logger.New(logger.Config{
		Level:          logger.ParseLevelFromEnv("LOG_LEVEL", logger.InfoLevel),
		Format:         logger.FormatJSON,
		ServiceName:    config.ServiceName,
		ServiceVersion: config.Version,
	})

	// Initialize configuration
	if err := config.Initialize(log); err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize configuration")
	}

	// Initialize OpenTelemetry tracing if enabled
	if config.OTLPEnabled {
		_, err := tracing.InitTracer(
			config.ServiceName,
			config.Version,
			config.OTLPEndpoint,
		)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to initialize OpenTelemetry tracing")
		} else {
			log.Info().Msg("OpenTelemetry tracing initialized")
			// Setup graceful shutdown for tracer
			go func() {
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				<-sigChan
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown tracer")
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
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	// Create and start server
	routerWithHandlers := router.NewRouterWithClientAndHandlers(redisClient, log)
	app := routerWithHandlers.App
	port := config.GetPort()

	// Check if TLS is configured
	if config.TLSCertFile != "" && config.TLSKeyFile != "" {
		log.Info().Str("port", port).Msg("Herald service starting with TLS")

		// Load server certificate
		cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load TLS certificate")
		}

		// Configure TLS
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}

		// Configure mTLS if CA certificate is provided
		if config.TLSCACertFile != "" {
			log.Info().Msg("mTLS enabled: client certificate verification required")

			// Load CA certificate for client verification
			caCert, err := os.ReadFile(config.TLSCACertFile)
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to read CA certificate")
			}

			caCertPool := x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caCert) {
				log.Fatal().Msg("Failed to parse CA certificate")
			}

			tlsConfig.ClientCAs = caCertPool
			// Require client certificates (but allow fallback to HMAC/API Key if not provided)
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		}

		// Start server with TLS
		ln, err := tls.Listen("tcp", port, tlsConfig)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to start TLS listener")
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
				log.Fatal().Err(err).Msg("Failed to start server")
			}
		case sig := <-sigChan:
			log.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

			// Shutdown audit writer
			if routerWithHandlers.Handlers != nil {
				if err := routerWithHandlers.Handlers.StopAuditWriter(); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown audit writer")
				}
			}

			// Shutdown tracer
			if config.OTLPEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown tracer")
				}
			}

			log.Info().Msg("Herald service stopped")
		}
	} else {
		log.Info().Str("port", port).Msg("Herald service starting")

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
				log.Fatal().Err(err).Msg("Failed to start server")
			}
		case sig := <-sigChan:
			log.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

			// Shutdown audit writer
			if routerWithHandlers.Handlers != nil {
				if err := routerWithHandlers.Handlers.StopAuditWriter(); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown audit writer")
				}
			}

			// Shutdown tracer
			if config.OTLPEnabled {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := tracing.Shutdown(ctx); err != nil {
					log.Error().Err(err).Msg("Failed to shutdown tracer")
				}
			}

			log.Info().Msg("Herald service stopped")
		}
	}
}
