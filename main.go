package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	logger "github.com/soulteary/logger-kit/v2"
	version "github.com/soulteary/version-kit/v2"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/router"
	"github.com/soulteary/herald/internal/server"
	rediskit "github.com/soulteary/redis-kit/client"
	"github.com/soulteary/tracing-kit"
)

// log is the global logger instance
var log *logger.Logger

// showBanner displays the startup banner with version
func showBanner() {
	pterm.DefaultBox.Println(
		putils.CenterText(
			"Herald\n" +
				"OTP and Verification Code Service\n" +
				"Version: " + version.Version,
		),
	)
	time.Sleep(time.Millisecond) // Don't ask why, but this fixes the docker-compose log
}

func main() {
	// -healthcheck performs a lightweight self-probe against the local /livez
	// endpoint and exits 0/1. This lets the container healthcheck use the
	// shipped binary instead of adding curl to the runtime image.
	healthcheck := flag.Bool("healthcheck", false, "probe local /livez and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	if err := run(); err != nil {
		if log != nil {
			log.Fatal().Err(err).Msg("Herald exited with error")
		}
		os.Exit(1)
	}
}

// runHealthcheck probes the local liveness endpoint and returns a process exit
// code (0 healthy, 1 unhealthy). It derives the port from the same config the
// server uses so it works regardless of PORT overrides.
func runHealthcheck() int {
	port := config.GetPort()
	// GetPort returns forms like ":8082"; build a loopback URL.
	host := "127.0.0.1" + port
	if _, p, err := net.SplitHostPort(port); err == nil && p != "" {
		host = "127.0.0.1:" + p
	}
	scheme, client, err := newHealthcheckClient()
	if err != nil {
		return 1
	}
	resp, err := client.Get(fmt.Sprintf("%s://%s/livez", scheme, host))
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// newHealthcheckClient mirrors the public listener's TLS mode. Certificate
// verification remains enabled; operators can provide a private CA and the DNS
// name present in the server certificate. mTLS deployments can also provide a
// dedicated client certificate for the loopback probe.
func newHealthcheckClient() (string, *http.Client, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	if config.TLSCertFile == "" || config.TLSKeyFile == "" {
		return "http", client, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: config.HealthcheckTLSServerName,
	}
	if config.HealthcheckTLSCAFile != "" {
		pem, err := os.ReadFile(config.HealthcheckTLSCAFile)
		if err != nil {
			return "", nil, fmt.Errorf("read healthcheck TLS CA: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return "", nil, fmt.Errorf("parse healthcheck TLS CA")
		}
		tlsConfig.RootCAs = roots
	}
	if config.HealthcheckTLSClientCertFile != "" {
		cert, err := tls.LoadX509KeyPair(config.HealthcheckTLSClientCertFile, config.HealthcheckTLSClientKeyFile)
		if err != nil {
			return "", nil, fmt.Errorf("load healthcheck client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	client.Transport = &http.Transport{TLSClientConfig: tlsConfig}
	return "https", client, nil
}

// run wires up dependencies and blocks until shutdown. Returning an error here
// (instead of calling log.Fatal deep in the stack) keeps startup failures
// testable and shutdown ordering in one place.
func run() error {
	// Display startup banner
	showBanner()

	// Initialize logger using logger-kit
	log = logger.New(logger.Config{
		Level:          logger.ParseLevelFromEnv("LOG_LEVEL", logger.InfoLevel),
		Format:         logger.FormatJSON,
		ServiceName:    config.ServiceName,
		ServiceVersion: version.Version,
	})

	// Initialize configuration (fails closed on invalid production config)
	if err := config.Initialize(log); err != nil {
		return err
	}

	// Initialize OpenTelemetry tracing if enabled
	if config.OTLPEnabled {
		if _, err := tracing.InitTracer(config.ServiceName, version.Version, config.OTLPEndpoint); err != nil {
			log.Warn().Err(err).Msg("Failed to initialize OpenTelemetry tracing")
		} else {
			log.Info().Msg("OpenTelemetry tracing initialized")
		}
	}

	// Initialize Redis client for router
	cfg := rediskit.DefaultConfig().
		WithAddr(config.RedisAddr).
		WithPassword(config.RedisPassword).
		WithDB(config.RedisDB)

	redisClient, err := rediskit.NewClient(cfg)
	if err != nil {
		return err
	}

	routerWithHandlers := router.NewRouterWithClientAndHandlers(redisClient, log)
	port := config.GetPort()

	srv, err := server.New(routerWithHandlers.App, server.Config{
		Addr:            port,
		TLSCertFile:     config.TLSCertFile,
		TLSKeyFile:      config.TLSKeyFile,
		TLSClientCAFile: config.TLSCACertFile,
		ClientCertMode:  config.ClientCertMode,
		Logger:          log,
	})
	if err != nil {
		return err
	}

	// Shutdown ordering: flush audit writer, then shut down the tracer.
	if routerWithHandlers.Handlers != nil {
		srv.OnShutdown(func(context.Context) error {
			return routerWithHandlers.Handlers.StopAuditWriter()
		})
	}
	if config.OTLPEnabled {
		srv.OnShutdown(func(ctx context.Context) error {
			return tracing.Shutdown(ctx)
		})
	}

	if srv.TLSEnabled() {
		log.Info().Str("port", port).Msg("Herald service starting with TLS")
	} else {
		log.Info().Str("port", port).Msg("Herald service starting")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if routerWithHandlers.TestApp != nil {
		testSrv, err := server.New(routerWithHandlers.TestApp, server.Config{
			Addr:         config.TestListenerAddr,
			LoopbackOnly: true,
			Logger:       log,
		})
		if err != nil {
			return fmt.Errorf("test listener: %w", err)
		}
		log.Warn().Str("addr", config.TestListenerAddr).Msg("Test-only listener starting")
		return server.RunAll(ctx, srv, testSrv)
	}

	return srv.Run(ctx)
}
