package audit

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/soulteary/herald/internal/audit/storage"
	"github.com/soulteary/herald/internal/audit/types"
)

// Writer handles asynchronous writing of audit records to persistent storage
type Writer struct {
	storage storage.Storage
	queue   chan *types.AuditRecord
	workers int
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewWriter creates a new asynchronous audit writer
func NewWriter(s storage.Storage, queueSize int, workers int) *Writer {
	ctx, cancel := context.WithCancel(context.Background())

	if queueSize <= 0 {
		queueSize = 1000 // Default queue size
	}
	if workers <= 0 {
		workers = 2 // Default number of workers
	}

	return &Writer{
		storage: s,
		queue:   make(chan *types.AuditRecord, queueSize),
		workers: workers,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts the writer workers
func (w *Writer) Start() {
	for i := 0; i < w.workers; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}
	logrus.Infof("Started %d audit log writer workers", w.workers)
}

// Stop stops the writer workers gracefully
func (w *Writer) Stop() error {
	logrus.Info("Stopping audit log writer...")

	// Cancel context to signal workers to stop
	w.cancel()

	// Close queue to prevent new writes
	close(w.queue)

	// Wait for all workers to finish processing remaining items
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	// Wait with timeout
	select {
	case <-done:
		logrus.Info("All audit log writer workers stopped")
	case <-time.After(10 * time.Second):
		logrus.Warn("Timeout waiting for audit log writer workers to stop")
	}

	// Close storage
	if w.storage != nil {
		return w.storage.Close()
	}

	return nil
}

// Enqueue enqueues an audit record for asynchronous writing
// Returns false if queue is full (non-blocking)
func (w *Writer) Enqueue(record *types.AuditRecord) bool {
	select {
	case w.queue <- record:
		return true
	default:
		// Queue is full, log warning but don't block
		logrus.Warnf("Audit log queue is full, dropping record: event_type=%s, user_id=%s", record.EventType, record.UserID)
		return false
	}
}

// worker is the worker goroutine that processes audit records from the queue
func (w *Writer) worker(id int) {
	defer w.wg.Done()

	logrus.Debugf("Audit log writer worker %d started", id)

	for {
		select {
		case <-w.ctx.Done():
			logrus.Debugf("Audit log writer worker %d stopping", id)
			return
		case record, ok := <-w.queue:
			if !ok {
				// Queue closed
				logrus.Debugf("Audit log writer worker %d: queue closed", id)
				return
			}

			// Write record to storage
			if err := w.storage.Write(w.ctx, record); err != nil {
				logrus.Errorf("Audit log writer worker %d failed to write record: %v", id, err)
				// Continue processing other records
			} else {
				logrus.Debugf("Audit log writer worker %d wrote record: event_type=%s", id, record.EventType)
			}
		}
	}
}

// Stats returns writer statistics
type Stats struct {
	QueueLength int
	Workers     int
}

// GetStats returns current writer statistics
func (w *Writer) GetStats() Stats {
	return Stats{
		QueueLength: len(w.queue),
		Workers:     w.workers,
	}
}
