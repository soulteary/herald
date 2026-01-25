package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	metricskit "github.com/soulteary/metrics-kit"
	rediskit "github.com/soulteary/redis-kit/client"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/handlers"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/middleware"
	"github.com/soulteary/herald/internal/session"
	"github.com/soulteary/herald/internal/tracing"
)

// NewRouter creates and configures a new Fiber router
// It initializes Redis client from config
// Deprecated: Use NewRouterWithClientAndHandlers for graceful shutdown support
func NewRouter() *fiber.App {
	// Initialize Redis client using redis-kit
	cfg := rediskit.DefaultConfig().
		WithAddr(config.RedisAddr).
		WithPassword(config.RedisPassword).
		WithDB(config.RedisDB)

	redisClient, err := rediskit.NewClient(cfg)
	if err != nil {
		logrus.Fatalf("Failed to connect to Redis: %v", err)
	}

	return NewRouterWithClient(redisClient)
}

// RouterWithHandlers wraps the router and handlers for graceful shutdown
type RouterWithHandlers struct {
	App      *fiber.App
	Handlers *handlers.Handlers
}

// NewRouterWithClient creates and configures a new Fiber router with the provided Redis client
// This is useful for testing with mock Redis clients
func NewRouterWithClient(redisClient *redis.Client) *fiber.App {
	return NewRouterWithClientAndHandlers(redisClient).App
}

// NewRouterWithClientAndHandlers creates a router with handlers for graceful shutdown
func NewRouterWithClientAndHandlers(redisClient *redis.Client) *RouterWithHandlers {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())

	// OpenTelemetry tracing middleware (if enabled)
	if config.OTLPEnabled {
		app.Use(tracing.TracingMiddleware(config.ServiceName))
		logrus.Info("OpenTelemetry tracing middleware enabled")
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
		)
		logrus.Info("Session storage manager initialized")
	}

	// Initialize handlers
	h := handlers.NewHandlers(redisClient, sessionManager)

	// Health check
	app.Get("/healthz", h.HealthCheck)

	// Prometheus metrics endpoint
	app.Get("/metrics", metricskit.FiberHandlerFor(metrics.Registry))

	// Test mode endpoint (only available when HERALD_TEST_MODE=true)
	if config.TestMode {
		app.Get("/v1/test/code/:challenge_id", h.GetTestCode)
	}

	// API routes
	api := app.Group("/v1")

	// OTP routes
	otp := api.Group("/otp")
	otp.Post("/challenges", middleware.RequireAuth(), h.CreateChallenge)
	otp.Post("/verifications", middleware.RequireAuth(), h.VerifyChallenge)
	otp.Post("/challenges/:id/revoke", middleware.RequireAuth(), h.RevokeChallenge)

	return &RouterWithHandlers{
		App:      app,
		Handlers: h,
	}
}
