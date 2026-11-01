package auditlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
)

func TestJournalRejectsInvalidPathsAndAbandonedDirectories(t *testing.T) {
	for _, kind := range []string{"empty path", "zero capacity", "parent is file", "lock is directory", "temporary directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			limit := int64(1 << 20)
			switch kind {
			case "empty path":
				dir = ""
			case "zero capacity":
				limit = 0
			case "parent is file":
				dir = filepath.Join(dir, "file")
				if err := os.WriteFile(dir, nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "lock is directory":
				if err := os.Mkdir(filepath.Join(dir, ".lock"), 0700); err != nil {
					t.Fatal(err)
				}
			case "temporary directory":
				p := filepath.Join(dir, "abandoned.tmp")
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p, "data"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			j, err := newJournalStorage(audit.NewNoopStorage(), dir, limit, 1, 1, nil)
			if err == nil {
				_ = j.Close()
				t.Fatal("invalid journal accepted")
			}
		})
	}
}

func TestJournalWriteFailuresAndDefaultWorkers(t *testing.T) {
	for _, kind := range []string{"canceled", "unencodable record", "unavailable directory", "directory sync", "closed"} {
		t.Run(kind, func(t *testing.T) {
			j, err := newJournalStorage(audit.NewNoopStorage(), t.TempDir(), 1<<20, 0, 0, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			ctx := context.Background()
			rec := audit.NewRecord(audit.EventChallengeCreated, audit.ResultSuccess)
			switch kind {
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unencodable record":
				rec.Metadata = map[string]interface{}{"invalid": make(chan int)}
			case "unavailable directory":
				if err := os.Rename(j.dir, j.dir+"-offline"); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = os.RemoveAll(j.dir + "-offline") }()
			case "directory sync":
				_ = j.dirFile.Close()
			case "closed":
				if err := j.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if err := j.Write(ctx, rec); err == nil {
				t.Fatal("failed journal write acknowledged")
			}
		})
	}
}

type uncooperativeStorage struct{ started, release chan struct{} }

func (s *uncooperativeStorage) Write(context.Context, *audit.Record) error {
	close(s.started)
	<-s.release
	return errors.New("backend stopped")
}
func (*uncooperativeStorage) Query(context.Context, *audit.QueryFilter) ([]*audit.Record, error) {
	return nil, nil
}
func (*uncooperativeStorage) Close() error { return nil }

func TestJournalShutdownTimeoutRetainsPendingRecord(t *testing.T) {
	sink := &uncooperativeStorage{started: make(chan struct{}), release: make(chan struct{})}
	failures := make(chan error, 1)
	dir := t.TempDir()
	j, err := newJournalStorage(sink, dir, 1<<20, 1, 1, func(err error) { failures <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(sink.release)
		_ = j.Close()
		<-j.done
		select {
		case <-failures:
		default:
			t.Error("backend failure was not reported")
		}
	}()
	if err := j.Write(context.Background(), audit.NewRecord(audit.EventChallengeCreated, audit.ResultSuccess)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.started:
	case <-time.After(5 * time.Second):
		t.Fatal("backend not called")
	}
	if records, err := j.Query(context.Background(), nil); err != nil || len(records) != 0 {
		t.Fatalf("query=%v err=%v", records, err)
	}
	if err := j.Close(); err == nil || !strings.Contains(err.Error(), "pending records remain on disk") {
		t.Fatalf("shutdown did not report pending data: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("pending audit record lost: files=%v err=%v", files, err)
	}
}
