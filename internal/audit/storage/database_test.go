package storage

import (
	"context"
	"testing"
	"time"

	"github.com/soulteary/herald/internal/audit/types"
)

func TestNewDatabaseStorage(t *testing.T) {
	t.Run("PostgreSQL URL format", func(t *testing.T) {
		// This test requires a real PostgreSQL database
		// Skip if not available
		databaseURL := "postgres://user:password@localhost:5432/herald?sslmode=disable"
		storage, err := NewDatabaseStorage(databaseURL)

		if err != nil {
			// Expected if database is not available
			t.Logf("NewDatabaseStorage() with PostgreSQL URL error = %v (expected if DB not available)", err)
		} else {
			defer func() { _ = storage.Close() }()
			if storage == nil {
				t.Fatal("NewDatabaseStorage() returned nil")
			}
			if storage.dbType != "postgres" {
				t.Errorf("NewDatabaseStorage() dbType = %q, want %q", storage.dbType, "postgres")
			}
		}
	})

	t.Run("MySQL URL format", func(t *testing.T) {
		// This test requires a real MySQL database
		// Skip if not available
		databaseURL := "mysql://user:password@tcp(localhost:3306)/herald"
		storage, err := NewDatabaseStorage(databaseURL)

		if err != nil {
			// Expected if database is not available
			t.Logf("NewDatabaseStorage() with MySQL URL error = %v (expected if DB not available)", err)
		} else {
			defer func() { _ = storage.Close() }()
			if storage == nil {
				t.Fatal("NewDatabaseStorage() returned nil")
			}
			if storage.dbType != "mysql" {
				t.Errorf("NewDatabaseStorage() dbType = %q, want %q", storage.dbType, "mysql")
			}
		}
	})

	t.Run("invalid URL format", func(t *testing.T) {
		invalidURLs := []string{
			"invalid://url",
			"http://localhost:5432/db",
			"sqlite://memory",
			"",
		}

		for _, url := range invalidURLs {
			t.Run(url, func(t *testing.T) {
				storage, err := NewDatabaseStorage(url)
				if err == nil {
					t.Errorf("NewDatabaseStorage() with invalid URL %q should return error", url)
				}
				if storage != nil {
					t.Errorf("NewDatabaseStorage() with invalid URL %q should return nil storage", url)
					_ = storage.Close()
				}
			})
		}
	})

	t.Run("connection failure", func(t *testing.T) {
		// Use invalid connection string
		databaseURL := "postgres://invalid:invalid@invalid:5432/invalid"
		storage, err := NewDatabaseStorage(databaseURL)

		if err == nil {
			t.Error("NewDatabaseStorage() with invalid connection should return error")
			if storage != nil {
				_ = storage.Close()
			}
		}
	})
}

// setupTestDatabase creates a test database storage
// Returns nil if database is not available (test will be skipped)
func setupTestDatabase(t *testing.T, dbType string) *DatabaseStorage {
	var databaseURL string
	switch dbType {
	case "postgres":
		databaseURL = "postgres://postgres:postgres@localhost:5432/herald_test?sslmode=disable"
	case "mysql":
		databaseURL = "mysql://root:password@tcp(localhost:3306)/herald_test"
	default:
		t.Fatalf("Unknown dbType: %s", dbType)
	}

	storage, err := NewDatabaseStorage(databaseURL)
	if err != nil {
		t.Skipf("Skipping test: Database not available: %v", err)
		return nil
	}

	return storage
}

func TestDatabaseStorage_Write(t *testing.T) {
	t.Run("PostgreSQL write", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType:         types.EventChallengeCreated,
			ChallengeID:       "ch_123",
			UserID:            "user123",
			Channel:           "email",
			Destination:       "test@example.com",
			Purpose:           "login",
			Result:            "success",
			Reason:            "",
			Provider:          "smtp",
			ProviderMessageID: "msg_123",
			IP:                "127.0.0.1",
			Timestamp:         time.Now().Unix(),
		}

		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	})

	t.Run("MySQL write", func(t *testing.T) {
		storage := setupTestDatabase(t, "mysql")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType:         types.EventChallengeCreated,
			ChallengeID:       "ch_123",
			UserID:            "user123",
			Channel:           "email",
			Destination:       "test@example.com",
			Purpose:           "login",
			Result:            "success",
			Reason:            "",
			Provider:          "smtp",
			ProviderMessageID: "msg_123",
			IP:                "127.0.0.1",
			Timestamp:         time.Now().Unix(),
		}

		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	})

	t.Run("write with NULL values", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user123",
			Result:    "success",
			Timestamp: time.Now().Unix(),
			// Other fields are empty (NULL)
		}

		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() with NULL values error = %v", err)
		}
	})
}

func TestDatabaseStorage_Query(t *testing.T) {
	t.Run("PostgreSQL query all", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		// Write test records
		now := time.Now().Unix()
		records := []*types.AuditRecord{
			{EventType: types.EventChallengeCreated, UserID: "user1", Channel: "email", Result: "success", Timestamp: now},
			{EventType: types.EventChallengeVerified, UserID: "user1", Channel: "sms", Result: "success", Timestamp: now + 1},
			{EventType: types.EventChallengeCreated, UserID: "user2", Channel: "email", Result: "failure", Timestamp: now + 2},
		}

		for _, record := range records {
			err := storage.Write(context.Background(), record)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}

		// Query all
		filter := DefaultQueryFilter()
		filter.Limit = 100

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) < 3 {
			t.Errorf("Query() returned %d records, want at least 3", len(results))
		}
	})

	t.Run("filter by EventType", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		// Write test record
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Query by EventType
		filter := &QueryFilter{
			EventType: string(types.EventChallengeCreated),
			Limit:     100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Query() with EventType filter returned no results")
		}

		for _, r := range results {
			if r.EventType != types.EventChallengeCreated {
				t.Errorf("Query() returned wrong EventType: %q", r.EventType)
			}
		}
	})

	t.Run("filter by UserID", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user_filter_test",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		filter := &QueryFilter{
			UserID: "user_filter_test",
			Limit:  100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Query() with UserID filter returned no results")
		}

		for _, r := range results {
			if r.UserID != "user_filter_test" {
				t.Errorf("Query() returned wrong UserID: %q", r.UserID)
			}
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		now := time.Now().Unix()
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: now,
		}
		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		filter := &QueryFilter{
			StartTime: now - 100,
			EndTime:   now + 100,
			Limit:     100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Query() with time range returned no results")
		}

		for _, r := range results {
			if r.Timestamp < now-100 || r.Timestamp > now+100 {
				t.Errorf("Query() returned record outside time range: timestamp = %d", r.Timestamp)
			}
		}
	})

	t.Run("limit and offset", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		// Write multiple records
		for i := 0; i < 5; i++ {
			record := &types.AuditRecord{
				EventType: types.EventChallengeCreated,
				UserID:    "user" + string(rune(i)),
				Result:    "success",
				Timestamp: time.Now().Unix() + int64(i),
			}
			err := storage.Write(context.Background(), record)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}

		filter := &QueryFilter{
			Limit:  2,
			Offset: 1,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with limit/offset returned %d records, want 2", len(results))
		}
	})

	t.Run("MySQL query", func(t *testing.T) {
		storage := setupTestDatabase(t, "mysql")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		filter := DefaultQueryFilter()
		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) == 0 {
			t.Error("Query() returned no results")
		}
	})
}

func TestDatabaseStorage_Close(t *testing.T) {
	t.Run("closes connection", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}

		err := storage.Close()
		if err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}

		// Verify connection is closed (subsequent operations should fail)
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record)
		if err == nil {
			t.Error("Write() after Close() should return error")
		}
	})
}

func TestDatabaseStorage_PostgreSQL_Specific(t *testing.T) {
	t.Run("PostgreSQL placeholder format", func(t *testing.T) {
		storage := setupTestDatabase(t, "postgres")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		// PostgreSQL uses $1, $2, etc. for placeholders
		// This is tested implicitly in Write and Query
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}

		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() with PostgreSQL error = %v", err)
		}
	})
}

func TestDatabaseStorage_MySQL_Specific(t *testing.T) {
	t.Run("MySQL placeholder format", func(t *testing.T) {
		storage := setupTestDatabase(t, "mysql")
		if storage == nil {
			return
		}
		defer func() { _ = storage.Close() }()

		// MySQL uses ? for placeholders
		// This is tested implicitly in Write and Query
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}

		err := storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() with MySQL error = %v", err)
		}
	})
}
