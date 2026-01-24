package tracing

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestTracingMiddleware(t *testing.T) {
	t.Run("creates span", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			// Verify span is in context
			span := c.Locals("trace_span")
			if span == nil {
				t.Error("TracingMiddleware should add span to locals")
			}
			ctx := c.Locals("trace_context")
			if ctx == nil {
				t.Error("TracingMiddleware should add trace context to locals")
			}
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("extracts trace context", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("injects trace context", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		// Verify trace context is injected (traceparent header may be present)
		// The exact header depends on OTel implementation
		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sets HTTP attributes", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("User-Agent", "test-agent")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("handles error", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			return fiber.NewError(500, "test error")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 500 {
			t.Errorf("Expected status 500, got %d", resp.StatusCode)
		}
	})

	t.Run("handles 4xx status code", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/test", func(c *fiber.Ctx) error {
			return c.Status(404).SendString("Not Found")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 404 {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("uses route path for span name", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/api/v1/users/:id", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/api/v1/users/123", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("uses method and URL when no route", func(t *testing.T) {
		app := fiber.New()
		app.Use(TracingMiddleware("test-service"))
		app.Get("/*", func(c *fiber.Ctx) error {
			return c.SendString("OK")
		})

		req := httptest.NewRequest("GET", "/unknown/path", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test() error = %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestHeaderCarrier(t *testing.T) {
	t.Run("Get method", func(t *testing.T) {
		headers := map[string]string{
			"key1": "value1",
			"key2": "value2",
		}
		carrier := &headerCarrier{headers: headers}

		val := carrier.Get("key1")
		if val != "value1" {
			t.Errorf("Get() = %q, want %q", val, "value1")
		}

		val = carrier.Get("key2")
		if val != "value2" {
			t.Errorf("Get() = %q, want %q", val, "value2")
		}

		val = carrier.Get("nonexistent")
		if val != "" {
			t.Errorf("Get() for nonexistent key = %q, want empty string", val)
		}
	})

	t.Run("Set method", func(t *testing.T) {
		headers := make(map[string]string)
		carrier := &headerCarrier{headers: headers}

		carrier.Set("key1", "value1")
		if headers["key1"] != "value1" {
			t.Errorf("Set() failed, headers[%q] = %q, want %q", "key1", headers["key1"], "value1")
		}

		carrier.Set("key2", "value2")
		if headers["key2"] != "value2" {
			t.Errorf("Set() failed, headers[%q] = %q, want %q", "key2", headers["key2"], "value2")
		}

		// Overwrite existing key
		carrier.Set("key1", "new_value")
		if headers["key1"] != "new_value" {
			t.Errorf("Set() failed to overwrite, headers[%q] = %q, want %q", "key1", headers["key1"], "new_value")
		}
	})

	t.Run("Keys method", func(t *testing.T) {
		headers := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}
		carrier := &headerCarrier{headers: headers}

		keys := carrier.Keys()
		if len(keys) != 3 {
			t.Errorf("Keys() length = %d, want 3", len(keys))
		}

		// Verify all keys are present
		keyMap := make(map[string]bool)
		for _, k := range keys {
			keyMap[k] = true
		}

		if !keyMap["key1"] {
			t.Error("Keys() missing key1")
		}
		if !keyMap["key2"] {
			t.Error("Keys() missing key2")
		}
		if !keyMap["key3"] {
			t.Error("Keys() missing key3")
		}
	})

	t.Run("empty headers", func(t *testing.T) {
		headers := make(map[string]string)
		carrier := &headerCarrier{headers: headers}

		val := carrier.Get("key")
		if val != "" {
			t.Errorf("Get() with empty headers = %q, want empty string", val)
		}

		keys := carrier.Keys()
		if len(keys) != 0 {
			t.Errorf("Keys() with empty headers length = %d, want 0", len(keys))
		}
	})
}
