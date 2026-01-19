package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald/internal/config"
)

func TestRequireAuth_NoAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := config.APIKey
	defer func() {
		config.APIKey = originalAPIKey
	}()

	// Set API key to empty (should skip auth)
	config.APIKey = ""

	app := fiber.New()
	app.Use(RequireAuth())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestRequireAuth_ValidAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := config.APIKey
	defer func() {
		config.APIKey = originalAPIKey
	}()

	// Set API key
	config.APIKey = "test-api-key"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

func TestRequireAuth_InvalidAPIKey(t *testing.T) {
	// Save original API key
	originalAPIKey := config.APIKey
	defer func() {
		config.APIKey = originalAPIKey
	}()

	// Set API key
	config.APIKey = "test-api-key"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "wrong-api-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestRequireAuth_ValidHMAC(t *testing.T) {
	// Save original config
	originalAPIKey := config.APIKey
	originalHMACSecret := config.HMACSecret
	defer func() {
		config.APIKey = originalAPIKey
		config.HMACSecret = originalHMACSecret
	}()

	// Set config
	config.APIKey = ""
	config.HMACSecret = "test-secret"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	body := `{"test": "data"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	service := "test-service"
	signature := computeHMAC(timestamp, service, body, config.HMACSecret)

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Service", service)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("RequireAuth() status = %d, want %d, body: %s", resp.StatusCode, fiber.StatusOK, string(bodyBytes))
	}
}

func TestRequireAuth_InvalidHMAC(t *testing.T) {
	// Save original config
	originalAPIKey := config.APIKey
	originalHMACSecret := config.HMACSecret
	defer func() {
		config.APIKey = originalAPIKey
		config.HMACSecret = originalHMACSecret
	}()

	// Set config - APIKey must be set to trigger HMAC check
	config.APIKey = "some-api-key"
	config.HMACSecret = "test-secret"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	body := `{"test": "data"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	service := "test-service"
	signature := "invalid-signature"

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Service", service)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestRequireAuth_ExpiredTimestamp(t *testing.T) {
	// Save original config
	originalAPIKey := config.APIKey
	originalHMACSecret := config.HMACSecret
	defer func() {
		config.APIKey = originalAPIKey
		config.HMACSecret = originalHMACSecret
	}()

	// Set config - APIKey must be set to trigger HMAC check
	config.APIKey = "some-api-key"
	config.HMACSecret = "test-secret"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	body := `{"test": "data"}`
	// Use timestamp 10 minutes ago (expired)
	timestamp := strconv.FormatInt(time.Now().Unix()-600, 10)
	service := "test-service"
	signature := computeHMAC(timestamp, service, body, config.HMACSecret)

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Service", service)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestRequireAuth_InvalidTimestamp(t *testing.T) {
	// Save original config
	originalAPIKey := config.APIKey
	originalHMACSecret := config.HMACSecret
	defer func() {
		config.APIKey = originalAPIKey
		config.HMACSecret = originalHMACSecret
	}()

	// Set config - APIKey must be set to trigger HMAC check
	config.APIKey = "some-api-key"
	config.HMACSecret = "test-secret"

	app := fiber.New()
	app.Use(RequireAuth())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	body := `{"test": "data"}`
	timestamp := "invalid-timestamp"
	service := "test-service"
	signature := computeHMAC("1234567890", service, body, config.HMACSecret)

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Service", service)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestRequireAuth_NoHMACSecret(t *testing.T) {
	// Save original config
	originalAPIKey := config.APIKey
	originalHMACSecret := config.HMACSecret
	defer func() {
		config.APIKey = originalAPIKey
		config.HMACSecret = originalHMACSecret
	}()

	// Set config - APIKey must be set to trigger HMAC check
	config.APIKey = "some-api-key"
	config.HMACSecret = ""

	app := fiber.New()
	app.Use(RequireAuth())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	body := `{"test": "data"}`
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	service := "test-service"
	signature := "some-signature"

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Signature", signature)
	req.Header.Set("X-Timestamp", timestamp)
	req.Header.Set("X-Service", service)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request failed: %v", err)
	}

	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("RequireAuth() status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestComputeHMAC(t *testing.T) {
	timestamp := "1234567890"
	service := "test-service"
	body := "test body"
	secret := "test-secret"

	signature := computeHMAC(timestamp, service, body, secret)

	// Verify signature is valid
	message := timestamp + ":" + service + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	if signature != expectedSig {
		t.Errorf("computeHMAC() = %v, want %v", signature, expectedSig)
	}

	// Verify different inputs produce different signatures
	signature2 := computeHMAC(timestamp+"1", service, body, secret)
	if signature == signature2 {
		t.Error("computeHMAC() should produce different signatures for different inputs")
	}
}

func TestAbs(t *testing.T) {
	tests := []struct {
		name string
		x    int64
		want int64
	}{
		{
			name: "positive number",
			x:    5,
			want: 5,
		},
		{
			name: "negative number",
			x:    -5,
			want: 5,
		},
		{
			name: "zero",
			x:    0,
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := abs(tt.x); got != tt.want {
				t.Errorf("abs() = %v, want %v", got, tt.want)
			}
		})
	}
}
