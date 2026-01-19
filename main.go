package main

import (
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
	
	logrus.Infof("Herald service starting on port %s", port)
	if err := app.Listen(port); err != nil {
		logrus.Fatalf("Failed to start server: %v", err)
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
