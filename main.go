package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/router"
)

func main() {
	// Initialize configuration
	if err := config.Initialize(); err != nil {
		logrus.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Set log level
	setLogLevel(config.LogLevel)

	// Create and start server
	app := router.NewRouter()
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

		if err := app.Listener(ln); err != nil {
			logrus.Fatalf("Failed to start server: %v", err)
		}
	} else {
		logrus.Infof("Herald service starting on port %s", port)
		if err := app.Listen(port); err != nil {
			logrus.Fatalf("Failed to start server: %v", err)
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
