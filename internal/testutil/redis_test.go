package testutil

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestNewTestRedisClient(t *testing.T) {
	t.Run("returns non-nil client", func(t *testing.T) {
		client, mock := NewTestRedisClient()
		defer func() {
			_ = client.Close()
		}()

		if client == nil {
			t.Fatal("NewTestRedisClient() returned nil client")
		}

		if mock == nil {
			t.Fatal("NewTestRedisClient() returned nil mock")
		}
	})

	t.Run("client is usable", func(t *testing.T) {
		client, _ := NewTestRedisClient()
		defer func() {
			_ = client.Close()
		}()

		ctx := context.Background()

		// Test basic operations
		err := client.Set(ctx, "test:key", "test:value", 0).Err()
		if err != nil {
			t.Errorf("Set() error = %v, want nil", err)
		}

		val, err := client.Get(ctx, "test:key").Result()
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}
		if val != "test:value" {
			t.Errorf("Get() = %q, want %q", val, "test:value")
		}
	})

	t.Run("mock can be controlled", func(t *testing.T) {
		client, mock := NewTestRedisClient()
		defer func() {
			_ = client.Close()
		}()

		ctx := context.Background()

		// Normal operation
		err := client.Set(ctx, "test:key", "value", 0).Err()
		if err != nil {
			t.Errorf("Set() error = %v, want nil", err)
		}

		// Set should fail
		mock.SetShouldFail(true)
		err = client.Set(ctx, "test:key2", "value2", 0).Err()
		if err == nil {
			t.Error("Set() should fail when mock.SetShouldFail(true)")
		}

		// Reset
		mock.SetShouldFail(false)
		err = client.Set(ctx, "test:key3", "value3", 0).Err()
		if err != nil {
			t.Errorf("Set() error = %v, want nil after reset", err)
		}
	})

	t.Run("multiple clients are independent", func(t *testing.T) {
		client1, _ := NewTestRedisClient()
		defer func() {
			_ = client1.Close()
		}()

		client2, _ := NewTestRedisClient()
		defer func() {
			_ = client2.Close()
		}()

		ctx := context.Background()

		// Set different values in each client
		err := client1.Set(ctx, "key1", "value1", 0).Err()
		if err != nil {
			t.Errorf("client1.Set() error = %v", err)
		}

		err = client2.Set(ctx, "key2", "value2", 0).Err()
		if err != nil {
			t.Errorf("client2.Set() error = %v", err)
		}

		// Verify they are independent
		val1, err := client1.Get(ctx, "key1").Result()
		if err != nil {
			t.Errorf("client1.Get() error = %v", err)
		}
		if val1 != "value1" {
			t.Errorf("client1.Get() = %q, want %q", val1, "value1")
		}

		// key2 should not exist in client1
		_, err = client1.Get(ctx, "key2").Result()
		if err != redis.Nil {
			t.Errorf("client1.Get(key2) error = %v, want redis.Nil", err)
		}
	})
}
