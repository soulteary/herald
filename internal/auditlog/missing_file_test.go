package auditlog

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
	logger "github.com/soulteary/logger-kit/v3"

	"github.com/soulteary/herald/internal/metrics"
)

func auditCounter(t *testing.T, name string) float64 {
	t.Helper()
	families, err := metrics.Registry.Gatherer().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == name {
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}

type reportLog struct {
	mu      sync.Mutex
	reports []error
}

func (r *reportLog) add(err error) {
	r.mu.Lock()
	r.reports = append(r.reports, err)
	r.mu.Unlock()
}

func (r *reportLog) missing() []*MissingError {
	r.mu.Lock()
	defer r.mu.Unlock()
	var found []*MissingError
	for _, err := range r.reports {
		var missing *MissingError
		if errors.As(err, &missing) {
			found = append(found, missing)
		}
	}
	return found
}

func pendingState(j *journalStorage) (int, int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.entries), j.bytes
}

// TestJournalDropsEntryWhoseFileWasDeleted accepts a record the backend
// refuses, so it stays pending, and then deletes its file from the spool.
// The entry used to be retried without end as an unreadable file and kept
// counting towards the size limit until the next restart. It must leave the
// retry set, release its bytes and be reported exactly once.
func TestJournalDropsEntryWhoseFileWasDeleted(t *testing.T) {
	dir := t.TempDir()
	sink := &controlledStorage{records: map[string]bool{}, fail: true, failed: make(chan struct{}, 1)}
	reports := &reportLog{}
	j, err := newJournalStorage(sink, dir, 1<<20, 1, 1, reports.add)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID("deleted-from-spool")
	if err := j.Write(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	<-sink.failed
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	// The spool is exactly full, so the pending entry blocks the next record.
	j.mu.Lock()
	j.limit = j.bytes
	j.mu.Unlock()
	if err := j.Write(context.Background(), record); err == nil || !strings.Contains(err.Error(), "capacity exceeded") {
		t.Fatalf("write into a full spool: %v", err)
	}

	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		entries, _ := pendingState(j)
		return entries == 0
	})
	if _, pending := pendingState(j); pending != 0 {
		t.Fatalf("%d bytes still count towards the spool limit after the entry was dropped", pending)
	}
	if err := j.Write(context.Background(), record); err != nil {
		t.Fatalf("the released bytes cannot be used: %v", err)
	}

	// Long enough for further attempts on the dropped entry, if there were any.
	time.Sleep(300 * time.Millisecond)
	missing := reports.missing()
	if len(missing) != 1 {
		t.Fatalf("%d reports of a missing file, want exactly 1: %v", len(missing), reports.reports)
	}
	if missing[0].Name != filepath.Base(files[0]) || !errors.Is(missing[0], fs.ErrNotExist) {
		t.Fatalf("unexpected report: %v", missing[0])
	}
}

// TestJournalKeepsEntriesWhenSpoolDirectoryIsReplaced moves the spool
// directory away and puts an empty one in its place, as an unmounted volume
// would. Every file then appears to be missing although the records still
// exist, so the entry must keep the read-failure retry and its bytes.
func TestJournalKeepsEntriesWhenSpoolDirectoryIsReplaced(t *testing.T) {
	oldMax := readRetryMaxDelay
	readRetryMaxDelay = 40 * time.Millisecond
	t.Cleanup(func() { readRetryMaxDelay = oldMax })

	base := t.TempDir()
	dir := filepath.Join(base, "spool")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	const id = "00112233445566778899aabbccddeeff"
	writeSpoolRecord(t, dir, id, "directory-replaced")
	if err := os.Chmod(filepath.Join(dir, id+".json"), 0); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root from reading")
	}
	reports := &reportLog{}
	j, err := newJournalStorage(&controlledStorage{records: map[string]bool{}}, dir, 1<<20, 1, 1, reports.add)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	if err := os.Rename(dir, filepath.Join(base, "detached")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	readFailures := func() int {
		j.mu.Lock()
		defer j.mu.Unlock()
		if entry := j.entries[id+".json"]; entry != nil {
			return entry.readFailures
		}
		return -1
	}
	start := readFailures()
	waitFor(t, func() bool { return readFailures() >= start+3 || readFailures() < 0 })
	if entries, pending := pendingState(j); entries != 1 || pending == 0 {
		t.Fatalf("entry dropped although the spool directory was replaced: entries=%d bytes=%d", entries, pending)
	}
	if missing := reports.missing(); len(missing) != 0 {
		t.Fatalf("reported as missing: %v", missing)
	}
}

// TestJournalDeliveredRecordWhoseFileVanishedIsNotReported deletes the file
// while the backend write is in flight. The record is delivered, so the
// failed cleanup of a file that is already gone is neither a delivery failure
// nor a lost record.
func TestJournalDeliveredRecordWhoseFileVanishedIsNotReported(t *testing.T) {
	dir := t.TempDir()
	sink := &controlledStorage{records: map[string]bool{}, started: make(chan struct{}, 1), release: make(chan struct{})}
	reports := &reportLog{}
	j, err := newJournalStorage(sink, dir, 1<<20, 1, 1, reports.add)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID("vanished-during-delivery")
	if err := j.Write(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	<-sink.started
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if err := os.Remove(files[0]); err != nil {
		t.Fatal(err)
	}
	close(sink.release)
	waitFor(t, func() bool {
		entries, _ := pendingState(j)
		return entries == 0
	})
	if _, pending := pendingState(j); pending != 0 {
		t.Fatalf("%d bytes still pending", pending)
	}
	sink.mu.Lock()
	delivered := sink.records["vanished-during-delivery"]
	sink.mu.Unlock()
	reports.mu.Lock()
	defer reports.mu.Unlock()
	if !delivered || len(reports.reports) != 0 {
		t.Fatalf("delivered=%v reports=%v, want a delivered record and no report", delivered, reports.reports)
	}
}

func TestJournalFailureReporterCountsMissingFilesSeparately(t *testing.T) {
	var logs bytes.Buffer
	report := journalFailureReporter(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON, Output: &logs}))

	const missingMetric, retryMetric = "herald_audit_spool_missing_total", "herald_audit_write_failure_total"
	missingBefore, retryBefore, quarantinedBefore := auditCounter(t, missingMetric), auditCounter(t, retryMetric), quarantinedCount(t)
	report(&MissingError{Name: "abc.json", Err: fs.ErrNotExist})
	if got := auditCounter(t, missingMetric); got != missingBefore+1 {
		t.Fatalf("%s = %v, want %v", missingMetric, got, missingBefore+1)
	}
	if auditCounter(t, retryMetric) != retryBefore || quarantinedCount(t) != quarantinedBefore {
		t.Fatal("a missing file was also counted as a retry or a quarantine")
	}
	for _, want := range []string{"no longer exists on disk", `"file":"abc.json"`, "not delivered"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log does not contain %q: %s", want, logs.String())
		}
	}

	// A nil logger must not panic.
	journalFailureReporter(nil)(&MissingError{Name: "x", Err: fs.ErrNotExist})
}
