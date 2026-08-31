package router

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/redis/go-redis/v9"
	health "github.com/soulteary/health-kit/v2"
	logger "github.com/soulteary/logger-kit/v2"
	metricskit "github.com/soulteary/metrics-kit/v2"
	middlewarekit "github.com/soulteary/middleware-kit/v2"
	rediskit "github.com/soulteary/redis-kit/client"

	"github.com/soulteary/herald/internal/auth"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/handlers"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/tracing"
)

func jsonErrorHandler(c fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	if fiberErr := new(fiber.Error); errors.As(err, &fiberErr) {
		status = fiberErr.Code
	}
	reason := "internal_error"
	switch status {
	case fiber.StatusNotFound:
		reason = "not_found"
	case fiber.StatusMethodNotAllowed:
		reason = "method_not_allowed"
	default:
		if status < fiber.StatusInternalServerError {
			reason = "request_error"
		}
	}
	return c.Status(status).JSON(fiber.Map{"ok": false, "reason": reason})
}

// trustedForwardedClientIP walks a proxy-appended chain from right to left and
// returns the first address outside the configured trusted proxy boundary.
func trustedForwardedClientIP(raw string, trusted []string) (string, bool) {
	parts := strings.Split(raw, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			return "", false
		}
		if !isTrustedProxyIP(ip, trusted) {
			return ip.String(), true
		}
	}
	return "", false
}

func isTrustedProxyIP(ip net.IP, trusted []string) bool {
	for _, entry := range trusted {
		entry = strings.TrimSpace(entry)
		if candidate := net.ParseIP(entry); candidate != nil && candidate.Equal(ip) {
			return true
		}
		if _, network, err := net.ParseCIDR(entry); err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

func sanitizeTrustedForwardedIP(header string, trusted []string) fiber.Handler {
	return func(c fiber.Ctx) error {
		raw := c.Get(header)
		if raw == "" {
			return c.Next()
		}
		if clientIP, ok := trustedForwardedClientIP(raw, trusted); ok {
			c.Request().Header.Set(header, clientIP)
		} else {
			// A malformed or all-proxy chain is not client identity.
			c.Request().Header.Del(header)
		}
		return c.Next()
	}
}

// testAuthMiddleware guards the test-code endpoint with a constant-time
// comparison against HERALD_TEST_API_KEY. It accepts the key via X-Test-Api-Key
// or Authorization: Bearer <key>.
func testAuthMiddleware(testKey string) fiber.Handler {
	return func(c fiber.Ctx) error {
		provided := c.Get("X-Test-Api-Key")
		if provided == "" {
			if authz := c.Get("Authorization"); strings.HasPrefix(authz, "Bearer ") {
				provided = strings.TrimPrefix(authz, "Bearer ")
			}
		}
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(testKey)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"ok":     false,
				"reason": "unauthorized",
			})
		}
		return c.Next()
	}
}

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
	TestApp  *fiber.App
	Handlers *handlers.Handlers
}

// NewRouterWithClient creates and configures a new Fiber router with the provided Redis client
// This is useful for testing with mock Redis clients
func NewRouterWithClient(redisClient *redis.Client, log *logger.Logger) *fiber.App {
	return NewRouterWithClientAndHandlers(redisClient, log).App
}

// NewRouterWithClientAndHandlers creates a router with handlers for graceful shutdown.
// Initialization failures panic rather than leave a partially initialized app running.
func NewRouterWithClientAndHandlers(redisClient *redis.Client, log *logger.Logger) *RouterWithHandlers {
	router, err := NewRouterWithClientAndHandlersE(redisClient, log)
	if err != nil {
		panic(err)
	}
	return router
}

// NewRouterWithClientAndHandlersE creates a router and reports handler/provider
// initialization failures to the startup path.
func NewRouterWithClientAndHandlersE(redisClient *redis.Client, log *logger.Logger) (*RouterWithHandlers, error) {
	app := fiber.New(fiber.Config{
		// Server hardening: cap request body size and enforce timeouts so a slow
		// or oversized client cannot exhaust resources.
		BodyLimit:    config.MaxBodyBytes,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
		ErrorHandler: jsonErrorHandler,
		// Fiber only reads the proxy header when the immediate peer matches the
		// explicit proxy allowlist, preventing direct header spoofing.
		ProxyHeader:        config.TrustedProxyHeader,
		TrustProxy:         len(config.TrustedProxies) > 0,
		EnableIPValidation: true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: config.TrustedProxies,
		},
	})

	// Middleware
	app.Use(recover.New())
	if config.TrustedProxyHeader != "" && len(config.TrustedProxies) > 0 {
		app.Use(sanitizeTrustedForwardedIP(config.TrustedProxyHeader, config.TrustedProxies))
	}

	// Request logging using logger-kit
	app.Use(logger.FiberMiddleware(logger.MiddlewareConfig{
		Logger:           log,
		SkipPaths:        []string{"/healthz", "/metrics", "/livez", "/readyz"},
		IncludeRequestID: true,
		IncludeLatency:   true,
	}))

	// OpenTelemetry tracing middleware (if enabled)
	if config.OTLPEnabled {
		app.Use(tracing.TracingMiddleware(config.ServiceName))
		log.Info().Msg("OpenTelemetry tracing middleware enabled")
	}

	// CORS is disabled by default. Only enable it when an explicit origin
	// allowlist is configured; a wildcard is rejected by config.Validate() in
	// production so it can never be enabled there.
	if origins := strings.TrimSpace(config.CORSAllowOrigins); origins != "" {
		allowedOrigins := strings.Split(origins, ",")
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
		app.Use(cors.New(cors.Config{
			AllowOrigins: allowedOrigins,
			AllowMethods: []string{"GET", "POST", "OPTIONS"},
			AllowHeaders: []string{"Content-Type", "Authorization", "X-Service", "X-Signature", "X-Signature-Version", "X-Timestamp", "X-Nonce", "X-Key-Id", "X-API-Key", "Idempotency-Key", "traceparent", "tracestate"},
		}))
		log.Info().Str("origins", origins).Msg("CORS enabled with explicit allowlist")
	}

	// A configured provider that cannot be constructed must abort startup
	// rather than silently disabling that channel.
	h, err := handlers.NewHandlersWithError(redisClient, log)
	if err != nil {
		return nil, fmt.Errorf("initialize handlers: %w", err)
	}

	// Health check using health-kit
	healthConfig := health.DefaultConfig().WithServiceName(config.ServiceName)
	healthAggregator := health.NewAggregator(healthConfig)
	healthAggregator.AddChecker(health.NewRedisChecker(redisClient))
	app.Get("/healthz", health.FiberHandler(healthAggregator))

	// Liveness: the process is up and able to serve. Never touches dependencies
	// so a transient Redis outage cannot cause the orchestrator to kill the pod.
	app.Get("/livez", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true, "status": "live"})
	})

	// Readiness: only ready to receive traffic when the OTP backend (Redis) is
	// reachable. Fail closed (503) on backend errors so load balancers stop
	// routing to an instance that cannot serve OTP requests.
	app.Get("/readyz", func(c fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
		defer cancel()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"ok":     false,
				"status": "not_ready",
				"reason": "redis_unreachable",
			})
		}
		return c.JSON(fiber.Map{"ok": true, "status": "ready"})
	})

	// Prometheus metrics endpoint
	app.Get("/metrics", metricskit.FiberHandlerFor(metrics.Registry))

	// Test mode endpoint: build a separate app so the route can only be exposed
	// on the dedicated loopback/admin listener. It is never part of the public
	// application, even in the test environment.
	var testApp *fiber.App
	if config.TestCodeExposureEnabled() {
		if config.TestAPIKey == "" {
			log.Error().Msg("Test-code endpoint requested but HERALD_TEST_API_KEY is empty; refusing to mount it")
		} else {
			testApp = fiber.New(fiber.Config{BodyLimit: config.MaxBodyBytes, ErrorHandler: jsonErrorHandler})
			testApp.Use(recover.New())
			testApp.Get("/livez", func(c fiber.Ctx) error {
				return c.JSON(fiber.Map{"ok": true, "status": "live"})
			})
			testApp.Get("/v1/test/code/:challenge_id", testAuthMiddleware(config.TestAPIKey), h.GetTestCode)
			log.Warn().Str("addr", config.TestListenerAddr).Msg("Test-code endpoint enabled on dedicated listener")
		}
	}

	// API routes
	api := app.Group("/v1")

	// Request authentication policy (Phase 4). Herald owns the policy so that:
	//   - HMAC v2 is replay-resistant (canonical binding + single-use nonce),
	//   - there is no silent HMAC -> API-key downgrade,
	//   - client-cert (mTLS) handling is independent of request-body auth.
	zerologLogger := log.Zerolog()
	authMode := auth.ParseMode(config.RequestAuthMode)

	// Legacy v1 handler (middleware-kit) is only constructed when explicitly
	// enabled for a migration cycle.
	var v1Handler fiber.Handler
	if config.HMACV1Enabled {
		v1Handler = middlewarekit.HMACAuth(middlewarekit.HMACConfig{
			KeyProvider:  config.GetHMACSecret,
			MaxTimeDrift: config.HMACMaxDrift,
			Logger:       &zerologLogger,
		})
		log.Warn().Msg("HMAC v1 is enabled (deprecated); disable HMAC_V1_ENABLED after migration")
	}

	// Determine the effective default key id. With a single HMAC_SECRET (no
	// multi-key map) an explicit X-Key-Id is not required, so we supply a
	// stable implicit default. With a parsed key map, use the effective default
	// selected by config (empty => X-Key-Id mandatory for a multi-key map).
	defaultKeyID := config.GetHMACDefaultKeyID()
	if defaultKeyID == "" && !config.HasHMACKeys() && config.HMACSecret != "" {
		defaultKeyID = "default"
	}

	nonceStore := auth.NewNonceStore(redisClient, config.NonceStorePrefix, []byte(config.IdempotencySecret), config.HMACMaxDrift+30*time.Second)
	authHandler := auth.New(auth.Config{
		Mode:         authMode,
		KeyProvider:  config.GetHMACSecret,
		DefaultKeyID: defaultKeyID,
		APIKey:       config.APIKey,
		NonceStore:   nonceStore,
		MaxDrift:     config.HMACMaxDrift,
		V1Enabled:    config.HMACV1Enabled,
		V1Handler:    v1Handler,
		FailClosed:   config.IsProduction(),
		Logger:       &zerologLogger,
	})

	// OTP routes
	otp := api.Group("/otp")
	otp.Post("/challenges", authHandler, h.CreateChallenge)
	otp.Post("/verifications", authHandler, h.VerifyChallenge)
	otp.Post("/challenges/:id/revoke", authHandler, h.RevokeChallenge)

	// v2 OTP routes: context-bound verification. v1 is kept for backward
	// compatibility; new integrations should prefer v2.
	apiV2 := app.Group("/v2")
	otpV2 := apiV2.Group("/otp")
	otpV2.Post("/verifications", authHandler, h.VerifyChallengeV2)

	// TOTP proxy routes (forward to herald-totp when HERALD_TOTP_ENABLED and HERALD_TOTP_BASE_URL are set)
	totp := api.Group("/totp")
	totp.Get("/status", authHandler, h.TOTPStatus)
	totp.Post("/verify", authHandler, h.TOTPVerify)
	totp.Post("/enroll/start", authHandler, h.TOTPEnrollStart)
	totp.Post("/enroll/confirm", authHandler, h.TOTPEnrollConfirm)
	totp.Post("/revoke", authHandler, h.TOTPRevoke)

	return &RouterWithHandlers{
		App:      app,
		TestApp:  testApp,
		Handlers: h,
	}, nil
}
