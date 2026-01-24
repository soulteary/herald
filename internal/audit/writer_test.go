package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/soulteary/herald/internal/audit/storage"
	"github.com/soulteary/herald/internal/audit/types"
)

// mockStorage is a thread-safe mock storage for testing Writer
type mockStorage struct {
	mu          sync.Mutex
	records     []*types.AuditRecord
	shouldError bool
}

func (m *mockStorage) Write(ctx context.Context, record *types.AuditRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldError {
		return errors.New("write error")
	}
	m.records = append(m.records, record)
	return nil
}

func (m *mockStorage) Query(ctx context.Context, filter *storage.QueryFilter) ([]*types.AuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records, nil
}

func (m *mockStorage) Close() error {
	return nil
}

func TestWriter_Worker(t *testing.T) {
	t.Run("processes_records", func(t *testing.T) {
		store := &mockStorage{
			records: make([]*types.AuditRecord, 0),
		}
		writer := NewWriter(store, 10, 1)
		writer.Start()

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
		}
		writer.Enqueue(record)

		// Wait a bit for processing
		time.Sleep(100 * time.Millisecond)

		if err := writer.Stop(); err != nil {
			t.Errorf("writer.Stop() failed: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.records) != 1 {
			t.Errorf("Expected 1 record, got %d", len(store.records))
		}
	})

	t.Run("handles_write_errors", func(t *testing.T) {
		store := &mockStorage{
			records:     make([]*types.AuditRecord, 0),
			shouldError: true,
		}
		writer := NewWriter(store, 10, 1)
		writer.Start()

		record := &types.AuditRecord{
			EventType: types.EventChallengeCreated,
			UserID:    "user1",
		}
		writer.Enqueue(record)

		// Wait a bit for processing
		time.Sleep(100 * time.Millisecond)

		if err := writer.Stop(); err != nil {
			t.Errorf("writer.Stop() failed: %v", err)
		}

		store.mu.Lock()
		defer store.mu.Unlock()
		if len(store.records) != 0 {
			t.Errorf("Expected 0 records, got %d", len(store.records))
		}
	})

	t.Run("stops_on_context_cancel", func(t *testing.T) {
		store := &mockStorage{
			records: make([]*types.AuditRecord, 0),
		}
		writer := NewWriter(store, 10, 1)
		writer.Start()

		if err := writer.Stop(); err != nil {
			t.Errorf("writer.Stop() failed: %v", err)
		}

		// Should not panic or hang
	})

	t.Run("stops_on_queue_close", func(t *testing.T) {
		store := &mockStorage{
			records: make([]*types.AuditRecord, 0),
		}
		writer := NewWriter(store, 10, 1)
		writer.Start()

		close(writer.queue)
		// Wait for workers to detect closed queue
		time.Sleep(50 * time.Millisecond)

		// Manually wait on wg to ensure workers exited
		done := make(chan struct{})
		go func() {
			writer.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Success
		case <-time.After(1 * time.Second):
			t.Error("Timeout waiting for workers to stop after queue close")
		}
	})
}

func TestWriter_Enqueue(t *testing.T) {
	store := &mockStorage{
		records: make([]*types.AuditRecord, 0),
	}
	// Small queue size to test full queue
	writer := NewWriter(store, 1, 1)

	// Don't start workers so queue fills up

	record1 := &types.AuditRecord{EventType: "event1"}
	if !writer.Enqueue(record1) {
		t.Error("Enqueue failed for empty queue")
	}

	record2 := &types.AuditRecord{EventType: "event2"}
	if writer.Enqueue(record2) {
		t.Error("Enqueue succeeded for full queue")
	}
}

func TestWriter_GetStats(t *testing.T) {
	store := &mockStorage{
		records: make([]*types.AuditRecord, 0),
	}
	writer := NewWriter(store, 10, 2)

	stats := writer.GetStats()
	if stats.Workers != 2 {
		t.Errorf("Expected 2 workers, got %d", stats.Workers)
	}
	if stats.QueueLength != 0 {
		t.Errorf("Expected queue length 0, got %d", stats.QueueLength)
	}
}
