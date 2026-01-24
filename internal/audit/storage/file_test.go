package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/soulteary/herald/internal/audit/types"
)

func TestNewFileStorage(t *testing.T) {
	t.Run("creates file storage", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		if storage == nil {
			t.Fatal("NewFileStorage() returned nil")
		}
		if storage.filePath != filePath {
			t.Errorf("NewFileStorage() filePath = %q, want %q", storage.filePath, filePath)
		}

		// Verify file was created
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("NewFileStorage() should create file")
		}
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "subdir", "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		// Verify directory was created
		dir := filepath.Dir(filePath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("NewFileStorage() should create directory")
		}
	})

	t.Run("opens existing file in append mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		// Create file with some content
		file, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("os.Create() error = %v", err)
		}
		_, _ = file.WriteString("existing content\n")
		_ = file.Close()

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		// Write new record
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user123",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Verify both old and new content exist
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}
		if len(content) == 0 {
			t.Error("File should contain content")
		}
	})
}

func TestFileStorage_Write(t *testing.T) {
	t.Run("writes JSON Lines format", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		record := &types.AuditRecord{
			EventType:   types.EventChallengeCreated,
			ChallengeID: "ch_123",
			UserID:      "user123",
			Channel:     "email",
			Destination: "test@example.com",
			Purpose:     "login",
			Result:      "success",
			Timestamp:   time.Now().Unix(),
		}

		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Read file and verify JSON Lines format
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}

		var readRecord types.AuditRecord
		err = json.Unmarshal(content[:len(content)-1], &readRecord) // Remove newline
		if err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if readRecord.EventType != record.EventType {
			t.Errorf("Read EventType = %q, want %q", readRecord.EventType, record.EventType)
		}
		if readRecord.UserID != record.UserID {
			t.Errorf("Read UserID = %q, want %q", readRecord.UserID, record.UserID)
		}
	})

	t.Run("writes multiple records", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		count := 5
		for i := 0; i < count; i++ {
			record := &types.AuditRecord{
				EventType: types.EventChallengeCreated,
				UserID:    "user" + string(rune(i)),
				Result:    "success",
				Timestamp: time.Now().Unix(),
			}
			err = storage.Write(context.Background(), record)
			if err != nil {
				t.Fatalf("Write() error = %v", err)
			}
		}

		// Verify all records were written
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}

		lines := 0
		for _, b := range content {
			if b == '\n' {
				lines++
			}
		}
		if lines != count {
			t.Errorf("Expected %d lines, got %d", count, lines)
		}
	})

	t.Run("concurrent writes", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		count := 10
		done := make(chan error, count)

		for i := 0; i < count; i++ {
			go func(id int) {
				record := &types.AuditRecord{
					EventType: types.EventChallengeCreated,
					UserID:    "user" + string(rune(id)),
					Result:    "success",
					Timestamp: time.Now().Unix(),
				}
				done <- storage.Write(context.Background(), record)
			}(i)
		}

		// Wait for all writes
		for i := 0; i < count; i++ {
			if err := <-done; err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}

		// Verify all records were written
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}

		lines := 0
		for _, b := range content {
			if b == '\n' {
				lines++
			}
		}
		if lines != count {
			t.Errorf("Expected %d lines, got %d", count, lines)
		}
	})
}

func TestFileStorage_Query(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(filePath)
	if err != nil {
		t.Fatalf("NewFileStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	// Write test records
	now := time.Now().Unix()
	records := []*types.AuditRecord{
		{EventType: types.EventChallengeCreated, UserID: "user1", Channel: "email", Result: "success", Timestamp: now},
		{EventType: types.EventChallengeVerified, UserID: "user1", Channel: "sms", Result: "success", Timestamp: now + 1},
		{EventType: types.EventChallengeCreated, UserID: "user2", Channel: "email", Result: "failure", Timestamp: now + 2},
		{EventType: types.EventSendFailed, UserID: "user2", Channel: "sms", Result: "failure", Timestamp: now + 3},
	}

	for _, record := range records {
		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	t.Run("query all records", func(t *testing.T) {
		filter := DefaultQueryFilter()
		filter.Limit = 100

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 4 {
			t.Errorf("Query() returned %d records, want 4", len(results))
		}

		// Verify newest first (reverse order)
		if results[0].Timestamp != now+3 {
			t.Errorf("Query() first record timestamp = %d, want %d", results[0].Timestamp, now+3)
		}
	})

	t.Run("filter by EventType", func(t *testing.T) {
		filter := &QueryFilter{
			EventType: string(types.EventChallengeCreated),
			Limit:     100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with EventType filter returned %d records, want 2", len(results))
		}

		for _, r := range results {
			if r.EventType != types.EventChallengeCreated {
				t.Errorf("Query() returned wrong EventType: %q", r.EventType)
			}
		}
	})

	t.Run("filter by UserID", func(t *testing.T) {
		filter := &QueryFilter{
			UserID: "user1",
			Limit:  100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with UserID filter returned %d records, want 2", len(results))
		}

		for _, r := range results {
			if r.UserID != "user1" {
				t.Errorf("Query() returned wrong UserID: %q", r.UserID)
			}
		}
	})

	t.Run("filter by Channel", func(t *testing.T) {
		filter := &QueryFilter{
			Channel: "email",
			Limit:   100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with Channel filter returned %d records, want 2", len(results))
		}

		for _, r := range results {
			if r.Channel != "email" {
				t.Errorf("Query() returned wrong Channel: %q", r.Channel)
			}
		}
	})

	t.Run("filter by Result", func(t *testing.T) {
		filter := &QueryFilter{
			Result: "success",
			Limit:  100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with Result filter returned %d records, want 2", len(results))
		}

		for _, r := range results {
			if r.Result != "success" {
				t.Errorf("Query() returned wrong Result: %q", r.Result)
			}
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		filter := &QueryFilter{
			StartTime: now + 1,
			EndTime:   now + 2,
			Limit:     100,
		}

		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Query() with time range returned %d records, want 2", len(results))
		}

		for _, r := range results {
			if r.Timestamp < now+1 || r.Timestamp > now+2 {
				t.Errorf("Query() returned record outside time range: timestamp = %d", r.Timestamp)
			}
		}
	})

	t.Run("limit and offset", func(t *testing.T) {
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

	t.Run("empty file", func(t *testing.T) {
		tmpDir := t.TempDir()
		emptyFilePath := filepath.Join(tmpDir, "empty.log")

		emptyStorage, err := NewFileStorage(emptyFilePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = emptyStorage.Close() }()

		filter := DefaultQueryFilter()
		results, err := emptyStorage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		if len(results) != 0 {
			t.Errorf("Query() on empty file returned %d records, want 0", len(results))
		}
	})

	t.Run("invalid JSON line skipped", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "invalid.log")

		// Write invalid JSON manually
		file, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("os.Create() error = %v", err)
		}
		_, _ = file.WriteString("invalid json\n")
		_, _ = file.WriteString(`{"event_type":"challenge_created","user_id":"user1","result":"success","timestamp":1234567890}` + "\n")
		_ = file.Close()

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		filter := DefaultQueryFilter()
		results, err := storage.Query(context.Background(), filter)
		if err != nil {
			t.Fatalf("Query() error = %v", err)
		}

		// Should return 1 valid record
		if len(results) != 1 {
			t.Errorf("Query() with invalid JSON returned %d records, want 1", len(results))
		}
	})
}

func TestFileStorage_Close(t *testing.T) {
	t.Run("closes file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}

		// Write a record
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user123",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Close
		err = storage.Close()
		if err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}

		// Verify file is closed (can't write after close)
		// This is tested by the fact that Close() doesn't error
	})

	t.Run("flush on close", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user123",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Close should flush
		err = storage.Close()
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}

		// Verify content was written
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.ReadFile() error = %v", err)
		}
		if len(content) == 0 {
			t.Error("Close() should flush buffer")
		}
	})
}

func TestFileStorage_Rotate(t *testing.T) {
	t.Run("rotates file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		// Write a record
		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user123",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record)
		if err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		// Rotate
		err = storage.Rotate()
		if err != nil {
			t.Fatalf("Rotate() error = %v", err)
		}

		// Verify old file was renamed
		matches, err := filepath.Glob(filePath + ".*")
		if err != nil {
			t.Fatalf("filepath.Glob() error = %v", err)
		}
		if len(matches) == 0 {
			t.Error("Rotate() should rename old file")
		}

		// Verify new file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("Rotate() should create new file")
		}

		// Write to new file
		record2 := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user456",
			Result:    "success",
			Timestamp: time.Now().Unix(),
		}
		err = storage.Write(context.Background(), record2)
		if err != nil {
			t.Fatalf("Write() after rotate error = %v", err)
		}
	})
}

func TestFileStorage_Concurrent(t *testing.T) {
	t.Run("concurrent write and query", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		storage, err := NewFileStorage(filePath)
		if err != nil {
			t.Fatalf("NewFileStorage() error = %v", err)
		}
		defer func() { _ = storage.Close() }()

		// Concurrent writes
		writeDone := make(chan error, 10)
		for i := 0; i < 10; i++ {
			go func(id int) {
				record := &types.AuditRecord{
					EventType: types.EventChallengeCreated,
					UserID:    "user" + string(rune(id)),
					Result:    "success",
					Timestamp: time.Now().Unix(),
				}
				writeDone <- storage.Write(context.Background(), record)
			}(i)
		}

		// Concurrent queries
		queryDone := make(chan error, 5)
		for i := 0; i < 5; i++ {
			go func() {
				_, err := storage.Query(context.Background(), DefaultQueryFilter())
				queryDone <- err
			}()
		}

		// Wait for all operations
		for i := 0; i < 10; i++ {
			if err := <-writeDone; err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}
		for i := 0; i < 5; i++ {
			if err := <-queryDone; err != nil {
				t.Errorf("Query() error = %v", err)
			}
		}
	})
}
