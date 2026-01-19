package router

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/handlers"
	"github.com/soulteary/herald/internal/middleware"
)

// NewRouter creates and configures a new Fiber router
func NewRouter() *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Content-Type,Authorization,X-Service,X-Signature,X-Timestamp,X-API-Key",
	}))

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:     config.RedisAddr,
		Password: config.RedisPassword,
		DB:       config.RedisDB,
	})

	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logrus.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Initialize handlers
	h := handlers.NewHandlers(redisClient)

	// Health check
	app.Get("/health", h.HealthCheck)

	// API routes
	api := app.Group("/v1")

	// OTP routes
	otp := api.Group("/otp")
	otp.Post("/challenges", middleware.RequireAuth(), h.CreateChallenge)
	otp.Post("/verifications", middleware.RequireAuth(), h.VerifyChallenge)
	otp.Post("/challenges/:id/revoke", middleware.RequireAuth(), h.RevokeChallenge)

	return app
}
