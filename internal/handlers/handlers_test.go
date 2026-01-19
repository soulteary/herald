package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/soulteary/herald/internal/challenge"
	"github.com/soulteary/herald/internal/config"
)

// testRedisClient returns a Redis client for testing
// If Redis is not available, tests will be skipped
func testRedisClient(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
		DB:   15, // Use DB 15 for testing
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("Skipping test: Redis not available: %v", err)
	}

	// Clean up test database
	client.FlushDB(ctx)

	return client
}

func TestNewHandlers(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	if handlers == nil {
		t.Fatal("NewHandlers() returned nil")
	}

	if handlers.challengeManager == nil {
		t.Error("NewHandlers() challengeManager is nil")
	}

	if handlers.rateLimitManager == nil {
		t.Error("NewHandlers() rateLimitManager is nil")
	}

	if handlers.providerRegistry == nil {
		t.Error("NewHandlers() providerRegistry is nil")
	}
}

func TestHandlers_HealthCheck(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	app := fiber.New()
	app.Get("/health", handlers.HealthCheck)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("HealthCheck() status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("HealthCheck() status = %v, want 'ok'", result["status"])
	}
}

func TestHandlers_CreateChallenge_Success(t *testing.T) {
	// Save original config
	originalRateLimitPerUser := config.RateLimitPerUser
	originalRateLimitPerIP := config.RateLimitPerIP
	originalRateLimitPerDestination := config.RateLimitPerDestination
	originalResendCooldown := config.ResendCooldown
	originalChallengeExpiry := config.ChallengeExpiry
	defer func() {
		config.RateLimitPerUser = originalRateLimitPerUser
		config.RateLimitPerIP = originalRateLimitPerIP
		config.RateLimitPerDestination = originalRateLimitPerDestination
		config.ResendCooldown = originalResendCooldown
		config.ChallengeExpiry = originalChallengeExpiry
	}()

	// Set config for testing
	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	reqBody := CreateChallengeRequest{
		UserID:      "user123",
		Channel:     "email",
		Destination: "test@example.com",
		Purpose:     "login",
		Locale:      "en",
		ClientIP:    "127.0.0.1",
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("CreateChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["challenge_id"] == nil {
		t.Error("CreateChallenge() response missing challenge_id")
	}
}

func TestHandlers_CreateChallenge_InvalidRequest(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	tests := []struct {
		name     string
		reqBody  interface{}
		wantCode int
	}{
		{
			name:     "missing user_id",
			reqBody:  map[string]string{"channel": "email", "destination": "test@example.com"},
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "invalid channel",
			reqBody:  map[string]string{"user_id": "user123", "channel": "invalid", "destination": "test@example.com"},
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "missing destination",
			reqBody:  map[string]string{"user_id": "user123", "channel": "email"},
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "invalid JSON",
			reqBody:  "invalid json",
			wantCode: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			if str, ok := tt.reqBody.(string); ok {
				bodyBytes = []byte(str)
			} else {
				bodyBytes, _ = json.Marshal(tt.reqBody)
			}

			req := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test request failed: %v", err)
			}

			if resp.StatusCode != tt.wantCode {
				t.Errorf("CreateChallenge() status = %d, want %d", resp.StatusCode, tt.wantCode)
			}
		})
	}
}

func TestHandlers_VerifyChallenge_Success(t *testing.T) {
	// Save original config
	originalCodeLength := config.CodeLength
	originalMaxAttempts := config.MaxAttempts
	originalLockoutDuration := config.LockoutDuration
	originalChallengeExpiry := config.ChallengeExpiry
	defer func() {
		config.CodeLength = originalCodeLength
		config.MaxAttempts = originalMaxAttempts
		config.LockoutDuration = originalLockoutDuration
		config.ChallengeExpiry = originalChallengeExpiry
	}()

	config.CodeLength = 6
	config.MaxAttempts = 5
	config.LockoutDuration = 10 * time.Minute
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	// Create a challenge first
	challengeMgr := challenge.NewManager(
		redisClient,
		config.ChallengeExpiry,
		config.MaxAttempts,
		config.LockoutDuration,
		config.CodeLength,
	)

	ctx := context.Background()
	ch, code, err := challengeMgr.CreateChallenge(ctx, "user123", "email", "test@example.com", "login", "127.0.0.1")
	if err != nil {
		t.Fatalf("Failed to create challenge: %v", err)
	}

	app := fiber.New()
	app.Post("/verify", handlers.VerifyChallenge)

	reqBody := VerifyChallengeRequest{
		ChallengeID: ch.ID,
		Code:        code,
		ClientIP:    "127.0.0.1",
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/verify", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("VerifyChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["ok"] != true {
		t.Error("VerifyChallenge() ok should be true")
	}
}

func TestHandlers_VerifyChallenge_InvalidRequest(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	app := fiber.New()
	app.Post("/verify", handlers.VerifyChallenge)

	tests := []struct {
		name     string
		reqBody  interface{}
		wantCode int
	}{
		{
			name:     "missing challenge_id",
			reqBody:  map[string]string{"code": "123456"},
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "missing code",
			reqBody:  map[string]string{"challenge_id": "ch_123"},
			wantCode: fiber.StatusBadRequest,
		},
		{
			name:     "invalid code format",
			reqBody:  map[string]string{"challenge_id": "ch_123", "code": "abc"},
			wantCode: fiber.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.reqBody)
			req := httptest.NewRequest("POST", "/verify", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test request failed: %v", err)
			}

			if resp.StatusCode != tt.wantCode {
				t.Errorf("VerifyChallenge() status = %d, want %d", resp.StatusCode, tt.wantCode)
			}
		})
	}
}

func TestHandlers_RevokeChallenge(t *testing.T) {
	// Save original config
	originalChallengeExpiry := config.ChallengeExpiry
	originalMaxAttempts := config.MaxAttempts
	originalLockoutDuration := config.LockoutDuration
	originalCodeLength := config.CodeLength
	defer func() {
		config.ChallengeExpiry = originalChallengeExpiry
		config.MaxAttempts = originalMaxAttempts
		config.LockoutDuration = originalLockoutDuration
		config.CodeLength = originalCodeLength
	}()

	config.ChallengeExpiry = 5 * time.Minute
	config.MaxAttempts = 5
	config.LockoutDuration = 10 * time.Minute
	config.CodeLength = 6

	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	// Create a challenge first
	challengeMgr := challenge.NewManager(
		redisClient,
		config.ChallengeExpiry,
		config.MaxAttempts,
		config.LockoutDuration,
		config.CodeLength,
	)

	ctx := context.Background()
	ch, _, err := challengeMgr.CreateChallenge(ctx, "user123", "email", "test@example.com", "login", "127.0.0.1")
	if err != nil {
		t.Fatalf("Failed to create challenge: %v", err)
	}

	app := fiber.New()
	app.Delete("/challenge/:id", handlers.RevokeChallenge)

	req := httptest.NewRequest("DELETE", "/challenge/"+ch.ID, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("RevokeChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["ok"] != true {
		t.Error("RevokeChallenge() ok should be true")
	}
}

func TestHandlers_RevokeChallenge_MissingID(t *testing.T) {
	redisClient := testRedisClient(t)
	defer redisClient.Close()

	handlers := NewHandlers(redisClient)

	app := fiber.New()
	app.Delete("/challenge/:id", handlers.RevokeChallenge)

	// Test with /challenge/ (missing ID)
	// Fiber's route /challenge/:id requires :id to have at least one character,
	// so /challenge/ doesn't match the route and returns 404
	// This is acceptable framework behavior
	req := httptest.NewRequest("DELETE", "/challenge/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	// Accept 404 as valid since Fiber's routing requires :id to have content
	// The handler's empty check would only be reached if the route matched
	if resp.StatusCode != fiber.StatusNotFound {
		t.Logf("Note: Route matched (unexpected), handler returned %d", resp.StatusCode)
		// If route somehow matches, handler should return 400 for empty param
		if resp.StatusCode != fiber.StatusBadRequest {
			t.Errorf("RevokeChallenge() status = %d, expected %d (route not matched) or %d (empty param)",
				resp.StatusCode, fiber.StatusNotFound, fiber.StatusBadRequest)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{
			name:   "contains substring",
			s:      "hello world",
			substr: "world",
			want:   true,
		},
		{
			name:   "does not contain substring",
			s:      "hello world",
			substr: "foo",
			want:   false,
		},
		{
			name:   "empty string",
			s:      "",
			substr: "test",
			want:   false,
		},
		{
			name:   "empty substring",
			s:      "hello",
			substr: "",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains() = %v, want %v", got, tt.want)
			}
		})
	}
}
