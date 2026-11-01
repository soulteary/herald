package auditlog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
	"github.com/soulteary/herald/internal/config"
)

type controlledStorage struct {
	mu      sync.Mutex
	records map[string]bool
	started chan struct{}
	release chan struct{}
	fail    bool
	failed  chan struct{}
}

func TestAuditDelivery_MasksBeforePersistence(t *testing.T) {
	dir := t.TempDir()
	s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 1), fail: true}
	j, err := newJournalStorage(s, dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	oldLogger, oldStorage := auditLogger, auditStorage
	oldEnabled, oldMask := config.AuditEnabled, config.AuditMaskDestination
	t.Cleanup(func() {
		_ = j.Close()
		auditLogger, auditStorage = oldLogger, oldStorage
		config.AuditEnabled, config.AuditMaskDestination = oldEnabled, oldMask
	})
	auditStorage = j
	auditLogger = audit.NewLogger(j, audit.DefaultConfig())
	config.AuditEnabled, config.AuditMaskDestination = true, true
	if err = LogSendSuccess(context.Background(), "mask", "u", "email", "private@example.com", "login", "smtp", "message", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	payload, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var record audit.Record
	if err = json.Unmarshal(payload, &record); err != nil {
		t.Fatal(err)
	}
	if record.Destination == "private@example.com" || record.Destination == "" || record.EventID == "" {
		t.Fatalf("unmasked or unidentified audit record: %+v", record)
	}
}

func (s *controlledStorage) Write(ctx context.Context, r *audit.Record) error {
	select {
	case s.started <- struct{}{}:
	default:
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		if s.failed != nil {
			select {
			case s.failed <- struct{}{}:
			default:
			}
		}
		return errors.New("injected audit storage failure")
	}
	s.records[r.ChallengeID] = true
	return nil
}
func (*controlledStorage) Query(context.Context, *audit.QueryFilter) ([]*audit.Record, error) {
	return nil, nil
}
func (*controlledStorage) Close() error { return nil }

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !condition() {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatal("audit delivery timed out")
		}
	}
}

// Fill one active write and the single queue slot before logging the third
// record, so queue pressure is deterministic rather than timing-dependent.
func TestAuditDelivery_QueueFullPreservesAllRecords(t *testing.T) {
	s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 4), release: make(chan struct{})}
	cfg := audit.DefaultConfig()
	cfg.Writer = &audit.WriterConfig{QueueSize: 1, Workers: 1}
	j, err := newJournalStorage(s, t.TempDir(), 1<<20, cfg.Writer.QueueSize, cfg.Writer.Workers, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := audit.NewLogger(j, cfg)
	t.Cleanup(func() { _ = l.Stop() })
	l.LogChallenge(context.Background(), audit.EventChallengeCreated, "one", "u", audit.ResultSuccess)
	select {
	case <-s.started:
	case <-time.After(5 * time.Second):
		t.Fatal("write did not start")
	}
	l.LogChallenge(context.Background(), audit.EventChallengeCreated, "two", "u", audit.ResultSuccess)
	l.LogChallenge(context.Background(), audit.EventChallengeCreated, "three", "u", audit.ResultSuccess)
	close(s.release)
	waitFor(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return len(s.records) == 3 })
}

func TestAuditDelivery_StorageFailureIsRetried(t *testing.T) {
	s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 4), fail: true, failed: make(chan struct{}, 1)}
	j, err := newJournalStorage(s, t.TempDir(), 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := audit.NewLogger(j, audit.DefaultConfig())
	t.Cleanup(func() { _ = l.Stop() })
	l.LogChallenge(context.Background(), audit.EventChallengeCreated, "retry", "u", audit.ResultSuccess)
	select {
	case <-s.failed:
	case <-time.After(5 * time.Second):
		t.Fatal("failing write did not run")
	}
	// Wait for the failing write to release the lock before recovery.
	s.mu.Lock()
	s.fail = false
	s.mu.Unlock()
	waitFor(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.records["retry"] })
}

func TestAuditDelivery_RestartReplaysPendingRecords(t *testing.T) {
	dir := t.TempDir()
	s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 1), failed: make(chan struct{}, 1), fail: true}
	j, err := newJournalStorage(s, dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Write(context.Background(), audit.NewRecord(audit.EventChallengeCreated, audit.ResultSuccess).WithChallengeID("restart")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.failed:
	case <-time.After(5 * time.Second):
		t.Fatal("failure was not exercised")
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.fail = false
	s.mu.Unlock()
	restarted, err := newJournalStorage(s, dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	waitFor(t, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.records["restart"] })
	waitFor(t, func() bool { restarted.mu.Lock(); defer restarted.mu.Unlock(); return len(restarted.entries) == 0 })
}

func TestAuditDelivery_CapacityAndWriteErrorsAreReturned(t *testing.T) {
	for _, kind := range []string{"capacity", "directory_missing"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 1)}
			limit := int64(1 << 20)
			if kind == "capacity" {
				limit = 1
			}
			j, err := newJournalStorage(s, dir, limit, 1, 1, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = j.Close() })
			if kind == "directory_missing" {
				if err = os.Rename(dir, dir+"-moved"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Rename(dir+"-moved", dir) })
			}
			if err = j.Write(context.Background(), audit.NewRecord(audit.EventSendSuccess, audit.ResultSuccess)); err == nil {
				t.Fatal("expected durable acceptance error")
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if len(s.records) != 0 {
				t.Fatal("unpersisted record was delivered")
			}
		})
	}
}

func TestAuditDelivery_SpoolIsExclusiveAndPrivate(t *testing.T) {
	dir := t.TempDir()
	s := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 1), fail: true}
	j, err := newJournalStorage(s, dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	other, err := newJournalStorage(s, dir, 1<<20, 1, 1, nil)
	if err == nil {
		_ = other.Close()
		t.Fatal("two journal owners accepted")
	}
	if err = j.Write(context.Background(), audit.NewRecord(audit.EventSendSuccess, audit.ResultSuccess)); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	info, err := os.Stat(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("record mode=%v", info.Mode())
	}
}
