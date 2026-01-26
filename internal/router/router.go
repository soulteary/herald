package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	health "github.com/soulteary/health-kit"
	logger "github.com/soulteary/logger-kit"
	metricskit "github.com/soulteary/metrics-kit"
	middlewarekit "github.com/soulteary/middleware-kit"
	rediskit "github.com/soulteary/redis-kit/client"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/handlers"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/session"
	"github.com/soulteary/herald/internal/tracing"
)

// NewRouter creates and configures a new Fiber router
// It initializes Redis client from config
// Deprecated: Use NewRouterWithClientAndHandlers for graceful shutdown support
func NewRouter(log *logger.Logger) *fiber.App {
	// Initialize Redis client using redis-kit
	cfg := rediskit.DefaultConfig().
		WithAddr(config.RedisAddr).
		WithPassword(config.RedisPassword).
		WithDB(config.RedisDB)

	redisClient, err := rediskit.NewClient(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}

	return NewRouterWithClient(redisClient, log)
}

// RouterWithHandlers wraps the router and handlers for graceful shutdown
type RouterWithHandlers struct {
	App      *fiber.App
	Handlers *handlers.Handlers
}

// NewRouterWithClient creates and configures a new Fiber router with the provided Redis client
// This is useful for testing with mock Redis clients
func NewRouterWithClient(redisClient *redis.Client, log *logger.Logger) *fiber.App {
	return NewRouterWithClientAndHandlers(redisClient, log).App
}

// NewRouterWithClientAndHandlers creates a router with handlers for graceful shutdown
func NewRouterWithClientAndHandlers(redisClient *redis.Client, log *logger.Logger) *RouterWithHandlers {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Middleware
	app.Use(recover.New())

	// Request logging using logger-kit
	app.Use(logger.FiberMiddleware(logger.MiddlewareConfig{
		Logger:           log,
		SkipPaths:        []string{"/healthz", "/metrics"},
		IncludeRequestID: true,
		IncludeLatency:   true,
	}))

	// OpenTelemetry tracing middleware (if enabled)
	if config.OTLPEnabled {
		app.Use(tracing.TracingMiddleware(config.ServiceName))
		log.Info().Msg("OpenTelemetry tracing middleware enabled")
	}

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,Authorization,X-Service,X-Signature,X-Timestamp,X-API-Key,traceparent,tracestate",
	}))

	// Initialize session manager if enabled
	var sessionManager *session.Manager
	if config.SessionStorageEnabled {
		sessionManager = session.NewManager(
			redisClient,
			config.SessionKeyPrefix,
			config.SessionDefaultTTL,
			log,
		)
		log.Info().Msg("Session storage manager initialized")
	}

	// Initialize handlers
	h := handlers.NewHandlers(redisClient, sessionManager, log)

	// Health check using health-kit
	healthConfig := health.DefaultConfig().WithServiceName(config.ServiceName)
	healthAggregator := health.NewAggregator(healthConfig)
	healthAggregator.AddChecker(health.NewRedisChecker(redisClient))
	app.Get("/healthz", health.FiberHandler(healthAggregator))

	// Prometheus metrics endpoint
	app.Get("/metrics", metricskit.FiberHandlerFor(metrics.Registry))

	// Test mode endpoint (only available when HERALD_TEST_MODE=true)
	if config.TestMode {
		app.Get("/v1/test/code/:challenge_id", h.GetTestCode)
	}

	// API routes
	api := app.Group("/v1")

	// Create authentication middleware using middleware-kit
	zerologLogger := log.Zerolog()
	authHandler := middlewarekit.CombinedAuth(middlewarekit.AuthConfig{
		HMACConfig: &middlewarekit.HMACConfig{
			KeyProvider: config.GetHMACSecret,
		},
		APIKeyConfig: &middlewarekit.APIKeyConfig{
			APIKey: config.APIKey,
		},
		AllowNoAuth: config.APIKey == "" && config.HMACSecret == "" && !config.HasHMACKeys(),
		Logger:      &zerologLogger,
	})

	// OTP routes
	otp := api.Group("/otp")
	otp.Post("/challenges", authHandler, h.CreateChallenge)
	otp.Post("/verifications", authHandler, h.VerifyChallenge)
	otp.Post("/challenges/:id/revoke", authHandler, h.RevokeChallenge)

	return &RouterWithHandlers{
		App:      app,
		Handlers: h,
	}
}
