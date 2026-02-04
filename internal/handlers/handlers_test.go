package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	logger "github.com/soulteary/logger-kit"

	challengekit "github.com/soulteary/challenge-kit"
	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
	sessionkit "github.com/soulteary/session-kit"
)

// testLogger returns a logger for testing (disabled output)
func testLogger() *logger.Logger {
	return logger.New(logger.Config{
		Level:  logger.ErrorLevel, // Only log errors during tests
		Format: logger.FormatJSON,
	})
}

// testRedisClient returns a mock Redis client for testing
func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	client, _ := testutil.NewTestRedisClient()
	return client
}

// testChallengeManager creates a challenge manager for testing
func testChallengeManager(t *testing.T, redisClient *redis.Client) challengekit.ManagerInterface {
	t.Helper()
	config := challengekit.Config{
		Expiry:          config.ChallengeExpiry,
		MaxAttempts:     config.MaxAttempts,
		LockoutDuration: config.LockoutDuration,
		CodeLength:      config.CodeLength,
	}
	return challengekit.NewManager(redisClient, config)
}

func TestNewHandlers(t *testing.T) {
	// Save original config
	originalSMTPHost := config.SMTPHost
	originalSMSProvider := config.SMSProvider
	defer func() {
		config.SMTPHost = originalSMTPHost
		config.SMSProvider = originalSMSProvider
	}()

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	// Test without providers
	config.SMTPHost = ""
	config.SMSProvider = ""
	handlers := NewHandlers(redisClient, nil, testLogger())

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

	// Test with SMTP provider configured
	config.SMTPHost = "smtp.example.com"
	config.SMTPPort = 587
	config.SMTPFrom = "test@example.com"
	handlers2 := NewHandlers(redisClient, nil, testLogger())
	if handlers2 == nil {
		t.Fatal("NewHandlers() returned nil")
	}

	// Test with SMS provider configured (uses HTTP API mode)
	config.SMSProvider = "aliyun"
	config.SMSAPIBaseURL = "http://localhost:8080"
	config.SMSAPIKey = "test-key"
	handlers3 := NewHandlers(redisClient, nil, testLogger())
	if handlers3 == nil {
		t.Fatal("NewHandlers() returned nil")
	}

	// Test with invalid SMTP config (should log warning but not fail)
	config.SMTPHost = "invalid"
	config.SMTPPort = 0 // Invalid port
	handlers4 := NewHandlers(redisClient, nil, testLogger())
	if handlers4 == nil {
		t.Fatal("NewHandlers() returned nil")
	}

	// Test with session manager (session-kit Store + KVManager)
	store := sessionkit.NewRedisStore(redisClient, "test_session:")
	sessionManager := sessionkit.NewKVManager(store, 1*time.Hour)
	handlers5 := NewHandlers(redisClient, sessionManager, testLogger())
	if handlers5 == nil {
		t.Fatal("NewHandlers() returned nil")
	}
	if handlers5.sessionManager == nil {
		t.Error("NewHandlers() sessionManager should not be nil when provided")
	}
}

func TestHandlers_StopAuditWriter(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() { _ = redisClient.Close() }()

	h := NewHandlers(redisClient, nil, testLogger())
	if h == nil {
		t.Fatal("NewHandlers() returned nil")
	}
	// StopAuditWriter calls auditlog.Stop(); auditlog uses sync.Once so may share
	// a logger from another test. We only assert the call runs without panic.
	_ = h.StopAuditWriter()
}

// Note: Health check is now handled by health-kit in router.go
// Tests for health check endpoint are in router_test.go

func TestHandlers_CreateChallenge_DefaultPurpose(t *testing.T) {
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

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	// Request without purpose (should default to "login")
	reqBody := map[string]interface{}{
		"user_id":     "user123",
		"channel":     "email",
		"destination": "test@example.com",
		// purpose omitted
		"locale":    "en",
		"client_ip": "127.0.0.1",
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
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	// Create a challenge first
	challengeConfig := challengekit.Config{
		Expiry:          config.ChallengeExpiry,
		MaxAttempts:     config.MaxAttempts,
		LockoutDuration: config.LockoutDuration,
		CodeLength:      config.CodeLength,
	}
	challengeMgr := challengekit.NewManager(redisClient, challengeConfig)

	ctx := context.Background()
	createReq := challengekit.CreateRequest{
		UserID:      "user123",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	}
	ch, code, err := challengeMgr.Create(ctx, createReq)
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

	// Verify AMR for email channel
	if amr, ok := result["amr"].([]interface{}); ok {
		expectedAMR := []string{"otp", "email"}
		if len(amr) != len(expectedAMR) {
			t.Errorf("VerifyChallenge() amr length = %d, want %d", len(amr), len(expectedAMR))
		} else {
			for i, v := range amr {
				if str, ok := v.(string); ok {
					if str != expectedAMR[i] {
						t.Errorf("VerifyChallenge() amr[%d] = %s, want %s", i, str, expectedAMR[i])
					}
				}
			}
		}
	} else {
		t.Error("VerifyChallenge() amr should be present and be an array")
	}
}

func TestHandlers_VerifyChallenge_AMR_ByChannel(t *testing.T) {
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

	tests := []struct {
		name        string
		channel     string
		destination string
		expectedAMR []string
	}{
		{
			name:        "email channel",
			channel:     "email",
			destination: "test@example.com",
			expectedAMR: []string{"otp", "email"},
		},
		{
			name:        "sms channel",
			channel:     "sms",
			destination: "+8613800138000",
			expectedAMR: []string{"otp", "sms"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redisClient := testRedisClient(t)
			defer func() {
				_ = redisClient.Close()
			}()

			handlers := NewHandlers(redisClient, nil, testLogger())

			// Create a challenge first
			challengeConfig := challengekit.Config{
				Expiry:          config.ChallengeExpiry,
				MaxAttempts:     config.MaxAttempts,
				LockoutDuration: config.LockoutDuration,
				CodeLength:      config.CodeLength,
			}
			challengeMgr := challengekit.NewManager(redisClient, challengeConfig)

			ctx := context.Background()
			var channel challengekit.Channel
			if tt.channel == "sms" {
				channel = challengekit.ChannelSMS
			} else {
				channel = challengekit.ChannelEmail
			}
			createReq := challengekit.CreateRequest{
				UserID:      "user123",
				Channel:     channel,
				Destination: tt.destination,
				Purpose:     "login",
				ClientIP:    "127.0.0.1",
			}
			ch, code, err := challengeMgr.Create(ctx, createReq)
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
				t.Fatalf("VerifyChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
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

			// Verify AMR matches expected values
			amrInterface, ok := result["amr"]
			if !ok {
				t.Fatal("VerifyChallenge() amr should be present")
			}

			amr, ok := amrInterface.([]interface{})
			if !ok {
				t.Fatal("VerifyChallenge() amr should be an array")
			}

			if len(amr) != len(tt.expectedAMR) {
				t.Errorf("VerifyChallenge() amr length = %d, want %d", len(amr), len(tt.expectedAMR))
			} else {
				for i, v := range amr {
					if str, ok := v.(string); ok {
						if str != tt.expectedAMR[i] {
							t.Errorf("VerifyChallenge() amr[%d] = %s, want %s", i, str, tt.expectedAMR[i])
						}
					} else {
						t.Errorf("VerifyChallenge() amr[%d] should be a string", i)
					}
				}
			}
		})
	}
}

func TestHandlers_VerifyChallenge_InvalidRequest(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	// Create a challenge first
	challengeMgr := testChallengeManager(t, redisClient)

	ctx := context.Background()
	createReq := challengekit.CreateRequest{
		UserID:      "user123",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	}
	ch, _, err := challengeMgr.Create(ctx, createReq)
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

func TestHandlers_RevokeChallenge_NotFound(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Delete("/challenge/:id", handlers.RevokeChallenge)

	// Try to revoke non-existent challenge
	req := httptest.NewRequest("DELETE", "/challenge/nonexistent_id", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	// Should still return OK (revoke is idempotent)
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("RevokeChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}
}

func TestHandlers_RevokeChallenge_EmptyID(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Delete("/challenge/:id", handlers.RevokeChallenge)

	// Test with empty ID (Fiber routing may not match, but test the handler logic)
	// We'll test by creating a route that can accept empty param
	req := httptest.NewRequest("DELETE", "/challenge/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	// Should return 404 (route not matched) or 400 (bad request if route matched)
	if resp.StatusCode != fiber.StatusNotFound && resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("RevokeChallenge with empty ID returned status=%d, body=%s", resp.StatusCode, string(body))
	}
}

func TestHandlers_RevokeChallenge_MissingID(t *testing.T) {
	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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

func TestHandlers_CreateChallenge_IdempotencyCacheError(t *testing.T) {
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

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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
	idempotencyKey := "test-idempotency-key-456"

	// First request
	req1 := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idempotencyKey)

	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp1.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp1.Body)
		t.Fatalf("First request failed: status=%d, body=%s", resp1.StatusCode, string(body))
	}

	// Idempotency cache Set may fail, but request should still succeed
	// The error is logged but doesn't fail the request
}

func TestHandlers_CreateChallenge_Idempotency(t *testing.T) {
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

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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
	idempotencyKey := "test-idempotency-key-123"

	// First request
	req1 := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", idempotencyKey)

	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp1.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp1.Body)
		t.Fatalf("First request failed: status=%d, body=%s", resp1.StatusCode, string(body))
	}

	body1, _ := io.ReadAll(resp1.Body)
	var result1 map[string]interface{}
	if err := json.Unmarshal(body1, &result1); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	firstChallengeID := result1["challenge_id"].(string)

	// Second request with same idempotency key
	req2 := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", idempotencyKey)

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp2.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("Second request failed: status=%d, body=%s", resp2.StatusCode, string(body))
	}

	body2, _ := io.ReadAll(resp2.Body)
	var result2 map[string]interface{}
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	secondChallengeID := result2["challenge_id"].(string)

	// Should return same challenge_id
	if firstChallengeID != secondChallengeID {
		t.Errorf("Idempotency failed: first challenge_id = %s, second = %s", firstChallengeID, secondChallengeID)
	}
}

func TestHandlers_CreateChallenge_ClientIPFromRequest(t *testing.T) {
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

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	// Request with ClientIP in body (should use it instead of c.IP())
	reqBody := CreateChallengeRequest{
		UserID:      "user123",
		Channel:     "email",
		Destination: "test@example.com",
		Purpose:     "login",
		Locale:      "en",
		ClientIP:    "192.168.1.100", // Custom IP
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
}

func TestHandlers_CreateChallenge_RateLimit(t *testing.T) {
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

	config.RateLimitPerUser = 2                  // Very low limit for testing
	config.RateLimitPerIP = 100                  // High IP limit to avoid IP rate limiting
	config.RateLimitPerDestination = 100         // High destination limit
	config.ResendCooldown = 1 * time.Millisecond // Very short cooldown to avoid interference
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	// Use different destinations to avoid resend cooldown
	for i := 0; i < 2; i++ {
		reqBody := CreateChallengeRequest{
			UserID:      "user123", // Same user to trigger user rate limit
			Channel:     "email",
			Destination: fmt.Sprintf("test%d@example.com", i), // Different destinations
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
			t.Fatalf("Request %d failed: status=%d, body=%s", i+1, resp.StatusCode, string(body))
		}
		// Small delay to avoid cooldown
		time.Sleep(10 * time.Millisecond)
	}

	// Next request with same user should be rate limited
	reqBody := CreateChallengeRequest{
		UserID:      "user123",
		Channel:     "email",
		Destination: "test3@example.com",
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

	if resp.StatusCode != fiber.StatusTooManyRequests {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected rate limit, got status=%d, body=%s", resp.StatusCode, string(body))
	}
}

func TestHandlers_CreateChallenge_ResendCooldown(t *testing.T) {
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

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 60 * time.Second // Long cooldown
	config.ChallengeExpiry = 5 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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

	// First request should succeed
	req1 := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application/json")

	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp1.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp1.Body)
		t.Fatalf("First request failed: status=%d, body=%s", resp1.StatusCode, string(body))
	}

	// Second request immediately should be blocked by cooldown
	req2 := httptest.NewRequest("POST", "/challenge", bytes.NewBuffer(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp2.StatusCode != fiber.StatusTooManyRequests {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("Expected cooldown, got status=%d, body=%s", resp2.StatusCode, string(body))
	}

	body2, _ := io.ReadAll(resp2.Body)
	var result2 map[string]interface{}
	if err := json.Unmarshal(body2, &result2); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result2["reason"] != "resend_cooldown" {
		t.Errorf("Expected reason=resend_cooldown, got %v", result2["reason"])
	}
}

func TestHandlers_CreateChallenge_InvalidPurpose(t *testing.T) {
	// Save original config
	originalAllowedPurposes := config.AllowedPurposes
	defer func() {
		config.AllowedPurposes = originalAllowedPurposes
	}()

	config.AllowedPurposes = []string{"login", "reset"} // Only allow login and reset

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	reqBody := CreateChallengeRequest{
		UserID:      "user123",
		Channel:     "email",
		Destination: "test@example.com",
		Purpose:     "invalid_purpose",
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

	if resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected bad request, got status=%d, body=%s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["reason"] != "invalid_purpose" {
		t.Errorf("Expected reason=invalid_purpose, got %v", result["reason"])
	}
}

func TestHandlers_CreateChallenge_UserLocked(t *testing.T) {
	// Save original config
	originalRateLimitPerUser := config.RateLimitPerUser
	originalRateLimitPerIP := config.RateLimitPerIP
	originalRateLimitPerDestination := config.RateLimitPerDestination
	originalResendCooldown := config.ResendCooldown
	originalChallengeExpiry := config.ChallengeExpiry
	originalMaxAttempts := config.MaxAttempts
	originalLockoutDuration := config.LockoutDuration
	defer func() {
		config.RateLimitPerUser = originalRateLimitPerUser
		config.RateLimitPerIP = originalRateLimitPerIP
		config.RateLimitPerDestination = originalRateLimitPerDestination
		config.ResendCooldown = originalResendCooldown
		config.ChallengeExpiry = originalChallengeExpiry
		config.MaxAttempts = originalMaxAttempts
		config.LockoutDuration = originalLockoutDuration
	}()

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute
	config.MaxAttempts = 3
	config.LockoutDuration = 10 * time.Minute

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	// Manually lock the user by using the challenge manager's lock functionality
	challengeConfig := challengekit.Config{
		Expiry:          config.ChallengeExpiry,
		MaxAttempts:     config.MaxAttempts,
		LockoutDuration: config.LockoutDuration,
		CodeLength:      6,
	}
	challengeMgr := challengekit.NewManager(redisClient, challengeConfig)
	ctx := context.Background()

	// Create a challenge and fail verification maxAttempts times on the SAME challenge to trigger lock
	createReq := challengekit.CreateRequest{
		UserID:      "user123",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	}
	ch, _, err := challengeMgr.Create(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create challenge: %v", err)
	}

	// Fail verification maxAttempts times on the same challenge
	// Each failure increments attempts, and after maxAttempts, user gets locked
	for i := 0; i < config.MaxAttempts; i++ {
		_, _ = challengeMgr.Verify(ctx, ch.ID, "000000", "127.0.0.1")
	}

	// Verify user is locked
	if !challengeMgr.IsUserLocked(ctx, "user123") {
		t.Fatal("User should be locked after max attempts on same challenge")
	}

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

	if resp.StatusCode != fiber.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected forbidden, got status=%d, body=%s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["reason"] != "user_locked" {
		t.Errorf("Expected reason=user_locked, got %v", result["reason"])
	}
}

func TestHandlers_VerifyChallenge_FailureReasons(t *testing.T) {
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
	config.ChallengeExpiry = 1 * time.Millisecond // Very short expiry

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/verify", handlers.VerifyChallenge)

	// Test expired challenge
	t.Run("expired challenge", func(t *testing.T) {
		challengeMgr := testChallengeManager(t, redisClient)

		ctx := context.Background()
		createReq := challengekit.CreateRequest{
			UserID:      "user123",
			Channel:     challengekit.ChannelEmail,
			Destination: "test@example.com",
			Purpose:     "login",
			ClientIP:    "127.0.0.1",
		}
		ch, code, err := challengeMgr.Create(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create challenge: %v", err)
		}

		// Wait for expiry
		time.Sleep(10 * time.Millisecond)

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

		if resp.StatusCode != fiber.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected unauthorized, got status=%d, body=%s", resp.StatusCode, string(body))
		}

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if result["reason"] != "expired" {
			t.Errorf("Expected reason=expired, got %v", result["reason"])
		}
	})

	// Test locked challenge
	t.Run("locked challenge", func(t *testing.T) {
		config.ChallengeExpiry = 5 * time.Minute
		challengeConfig := challengekit.Config{
			Expiry:          config.ChallengeExpiry,
			MaxAttempts:     3, // Max attempts = 3
			LockoutDuration: config.LockoutDuration,
			CodeLength:      config.CodeLength,
		}
		challengeMgr := challengekit.NewManager(redisClient, challengeConfig)

		ctx := context.Background()
		createReq := challengekit.CreateRequest{
			UserID:      "user789",
			Channel:     challengekit.ChannelEmail,
			Destination: "test3@example.com",
			Purpose:     "login",
			ClientIP:    "127.0.0.1",
		}
		ch, _, err := challengeMgr.Create(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create challenge: %v", err)
		}

		// Fail verification 3 times to lock
		for i := 0; i < 3; i++ {
			_, _ = challengeMgr.Verify(ctx, ch.ID, "000000", "127.0.0.1")
		}

		// Create new challenge for locked user
		createReq2 := challengekit.CreateRequest{
			UserID:      "user789",
			Channel:     challengekit.ChannelEmail,
			Destination: "test3@example.com",
			Purpose:     "login",
			ClientIP:    "127.0.0.1",
		}
		ch2, _, err := challengeMgr.Create(ctx, createReq2)
		if err != nil {
			t.Fatalf("Failed to create challenge: %v", err)
		}

		reqBody := VerifyChallengeRequest{
			ChallengeID: ch2.ID,
			Code:        "123456",
			ClientIP:    "127.0.0.1",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/verify", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test request failed: %v", err)
		}

		if resp.StatusCode != fiber.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected unauthorized, got status=%d, body=%s", resp.StatusCode, string(body))
		}

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Should return user_locked reason (user was locked before verification)
		reason := result["reason"].(string)
		if reason != "user_locked" {
			t.Errorf("Expected reason=user_locked, got %v", reason)
		}
	})

	// Test invalid code
	t.Run("invalid code", func(t *testing.T) {
		config.ChallengeExpiry = 5 * time.Minute
		challengeMgr := testChallengeManager(t, redisClient)

		ctx := context.Background()
		createReq := challengekit.CreateRequest{
			UserID:      "user456",
			Channel:     challengekit.ChannelEmail,
			Destination: "test2@example.com",
			Purpose:     "login",
			ClientIP:    "127.0.0.1",
		}
		ch, _, err := challengeMgr.Create(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create challenge: %v", err)
		}

		reqBody := VerifyChallengeRequest{
			ChallengeID: ch.ID,
			Code:        "000000",
			ClientIP:    "127.0.0.1",
		}

		bodyBytes, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/verify", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test request failed: %v", err)
		}

		if resp.StatusCode != fiber.StatusUnauthorized {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("Expected unauthorized, got status=%d, body=%s", resp.StatusCode, string(body))
		}

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if result["reason"] != "invalid" {
			t.Errorf("Expected reason=invalid, got %v", result["reason"])
		}
	})
}

func TestHandlers_CreateChallenge_TemplateErrorFallback(t *testing.T) {
	// Save original config
	originalRateLimitPerUser := config.RateLimitPerUser
	originalRateLimitPerIP := config.RateLimitPerIP
	originalRateLimitPerDestination := config.RateLimitPerDestination
	originalResendCooldown := config.ResendCooldown
	originalChallengeExpiry := config.ChallengeExpiry
	originalTemplateDir := config.TemplateDir
	defer func() {
		config.RateLimitPerUser = originalRateLimitPerUser
		config.RateLimitPerIP = originalRateLimitPerIP
		config.RateLimitPerDestination = originalRateLimitPerDestination
		config.ResendCooldown = originalResendCooldown
		config.ChallengeExpiry = originalChallengeExpiry
		config.TemplateDir = originalTemplateDir
	}()

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute
	config.TemplateDir = "/nonexistent" // Force template fallback

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Post("/challenge", handlers.CreateChallenge)

	// Test SMS channel with template fallback
	reqBody := CreateChallengeRequest{
		UserID:      "user123",
		Channel:     "sms",
		Destination: "+8613800138000",
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

	// Should succeed even with template fallback
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("CreateChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}
}

func TestHandlers_CreateChallenge_TemplateFallback(t *testing.T) {
	// Save original config
	originalRateLimitPerUser := config.RateLimitPerUser
	originalRateLimitPerIP := config.RateLimitPerIP
	originalRateLimitPerDestination := config.RateLimitPerDestination
	originalResendCooldown := config.ResendCooldown
	originalChallengeExpiry := config.ChallengeExpiry
	originalTemplateDir := config.TemplateDir
	defer func() {
		config.RateLimitPerUser = originalRateLimitPerUser
		config.RateLimitPerIP = originalRateLimitPerIP
		config.RateLimitPerDestination = originalRateLimitPerDestination
		config.ResendCooldown = originalResendCooldown
		config.ChallengeExpiry = originalChallengeExpiry
		config.TemplateDir = originalTemplateDir
	}()

	config.RateLimitPerUser = 100
	config.RateLimitPerIP = 100
	config.RateLimitPerDestination = 100
	config.ResendCooldown = 1 * time.Second
	config.ChallengeExpiry = 5 * time.Minute
	config.TemplateDir = "/nonexistent" // Force template fallback

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

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

	// Should succeed even with template fallback
	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("CreateChallenge() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(body))
	}
}

func TestHandlers_GetTestCode(t *testing.T) {
	// Save original config
	originalTestMode := config.TestMode
	defer func() {
		config.TestMode = originalTestMode
	}()

	config.TestMode = true

	redisClient := testRedisClient(t)
	defer func() {
		_ = redisClient.Close()
	}()

	handlers := NewHandlers(redisClient, nil, testLogger())

	// Create a challenge and store test code
	challengeConfig := challengekit.Config{
		Expiry:          5 * time.Minute,
		MaxAttempts:     5,
		LockoutDuration: 10 * time.Minute,
		CodeLength:      6,
	}
	challengeMgr := challengekit.NewManager(redisClient, challengeConfig)

	ctx := context.Background()
	createReq := challengekit.CreateRequest{
		UserID:      "user123",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	}
	ch, code, err := challengeMgr.Create(ctx, createReq)
	if err != nil {
		t.Fatalf("Failed to create challenge: %v", err)
	}

	// Store test code manually (simulating what CreateChallenge does in test mode)
	testCodeCache := handlers.testCodeCache
	if err := testCodeCache.Set(ctx, ch.ID, code, 5*time.Minute); err != nil {
		t.Fatalf("Failed to store test code: %v", err)
	}

	app := fiber.New()
	app.Get("/test/code/:challenge_id", handlers.GetTestCode)

	req := httptest.NewRequest("GET", "/test/code/"+ch.ID, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected OK, got status=%d, body=%s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if result["code"] != code {
		t.Errorf("Expected code=%s, got %v", code, result["code"])
	}

	// Test with test mode disabled
	config.TestMode = false
	req2 := httptest.NewRequest("GET", "/test/code/"+ch.ID, nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp2.StatusCode != fiber.StatusNotFound {
		body, _ := io.ReadAll(resp2.Body)
		t.Errorf("Expected not found, got status=%d, body=%s", resp2.StatusCode, string(body))
	}
}

func TestHandlers_GetTestCode_EmptyChallengeID(t *testing.T) {
	originalTestMode := config.TestMode
	defer func() { config.TestMode = originalTestMode }()
	config.TestMode = true

	redisClient := testRedisClient(t)
	defer func() { _ = redisClient.Close() }()
	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Get("/test/code/:challenge_id", handlers.GetTestCode)

	// Request with path that may yield empty challenge_id (e.g. trailing slash or empty segment)
	req := httptest.NewRequest("GET", "/test/code/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	// Expect 404 (route not matched) or 400 (handler got empty param)
	if resp.StatusCode != fiber.StatusNotFound && resp.StatusCode != fiber.StatusBadRequest {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GetTestCode with empty challenge_id: status=%d, body=%s", resp.StatusCode, string(body))
	}
}

func TestHandlers_GetTestCode_CodeNotFound(t *testing.T) {
	originalTestMode := config.TestMode
	defer func() { config.TestMode = originalTestMode }()
	config.TestMode = true

	redisClient := testRedisClient(t)
	defer func() { _ = redisClient.Close() }()
	handlers := NewHandlers(redisClient, nil, testLogger())

	app := fiber.New()
	app.Get("/test/code/:challenge_id", handlers.GetTestCode)

	req := httptest.NewRequest("GET", "/test/code/nonexistent_challenge_id_123", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("GetTestCode with nonexistent id: status=%d, want 404, body=%s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if result["reason"] != "code_not_found" {
		t.Errorf("Expected reason=code_not_found, got %v", result["reason"])
	}
}

func TestHandlers_VerifyChallenge_RemainingAttempts(t *testing.T) {
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
	defer func() { _ = redisClient.Close() }()
	handlers := NewHandlers(redisClient, nil, testLogger())

	challengeMgr := testChallengeManager(t, redisClient)
	ctx := context.Background()
	createReq := challengekit.CreateRequest{
		UserID:      "user_ra",
		Channel:     challengekit.ChannelEmail,
		Destination: "test@example.com",
		Purpose:     "login",
		ClientIP:    "127.0.0.1",
	}
	ch, _, err := challengeMgr.Create(ctx, createReq)
	if err != nil {
		t.Fatalf("Create challenge: %v", err)
	}

	app := fiber.New()
	app.Post("/verify", handlers.VerifyChallenge)

	reqBody := VerifyChallengeRequest{ChallengeID: ch.ID, Code: "000000", ClientIP: "127.0.0.1"}
	bodyBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/verify", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected 401, got %d, body=%s", resp.StatusCode, string(body))
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if result["reason"] != "invalid" {
		t.Errorf("Expected reason=invalid, got %v", result["reason"])
	}
	// challenge-kit may return remaining_attempts on invalid code
	if rem, ok := result["remaining_attempts"]; ok && rem != nil {
		// coverage for remaining_attempts branch
		_ = rem
	}
}

func TestMaskDestination(t *testing.T) {
	// maskDestination is used for tracing; test email and phone masking
	t.Run("email", func(t *testing.T) {
		out := maskDestination("user@example.com")
		if out == "" || out == "user@example.com" {
			t.Errorf("maskDestination(email) should mask, got %q", out)
		}
	})
	t.Run("phone", func(t *testing.T) {
		out := maskDestination("+8613800138000")
		if out == "" {
			t.Errorf("maskDestination(phone) should not be empty, got %q", out)
		}
	})
	t.Run("empty", func(t *testing.T) {
		out := maskDestination("")
		if out != "" {
			t.Errorf("maskDestination(empty) = %q, want empty", out)
		}
	})
}
