package auditlog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
)

func writeSpoolRecord(t *testing.T, dir, id, challengeID string) string {
	t.Helper()
	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID(challengeID)
	record.EventID = id
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestJournalRetriesUnreadableEntryWithoutQuarantine starts a journal on a
// spool whose one pending record is valid but cannot be opened. A read error
// says nothing about the content, and the cause may pass, so the record must
// stay in the spool however often the read fails, be reported only on some
// attempts, and be delivered once the file can be read again. It used to be
// quarantined after five attempts and was then never delivered.
func TestJournalRetriesUnreadableEntryWithoutQuarantine(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions do not stop root from reading")
	}
	oldMax := readRetryMaxDelay
	readRetryMaxDelay = 80 * time.Millisecond
	t.Cleanup(func() { readRetryMaxDelay = oldMax })

	dir := t.TempDir()
	const id = "0123456789abcdef0123456789abcdef"
	path := writeSpoolRecord(t, dir, id, "unreadable-for-a-while")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var reported []error
	sink := &controlledStorage{records: map[string]bool{}}
	j, err := newJournalStorage(sink, dir, 1<<20, 1, 1, func(err error) {
		mu.Lock()
		reported = append(reported, err)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	// Well past the five attempts that used to end in quarantine.
	const attempts = 12
	waitFor(t, func() bool {
		j.mu.Lock()
		defer j.mu.Unlock()
		entry := j.entries[id+".json"]
		return entry != nil && entry.readFailures >= attempts
	})
	if _, err := os.Stat(path + quarantineSuffix); !os.IsNotExist(err) {
		t.Fatalf("an unreadable entry was quarantined (stat error: %v)", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the entry left the spool: %v", err)
	}
	j.mu.Lock()
	failures, counted := j.entries[id+".json"].readFailures, j.bytes
	j.mu.Unlock()
	if counted == 0 {
		t.Fatal("the pending entry no longer counts towards the spool size limit")
	}

	// Reported on the 1st, 2nd, 4th, 8th ... failure only.
	mu.Lock()
	count := len(reported)
	for _, err := range reported {
		var quarantined *QuarantinedError
		if errors.As(err, &quarantined) {
			t.Fatalf("a read failure was reported as a quarantine: %v", err)
		}
		if !strings.Contains(err.Error(), "could not be read") || !errors.Is(err, os.ErrPermission) {
			t.Fatalf("unexpected report: %v", err)
		}
	}
	mu.Unlock()
	if count < 4 || count >= failures {
		t.Fatalf("%d reports for %d failed reads; want one for the 1st, 2nd, 4th, 8th ... only", count, failures)
	}

	// The fault clears and the record is delivered.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return sink.records["unreadable-for-a-while"]
	})
	waitFor(t, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	})
}

// TestJournalDecodeFailuresDoNotAddUpAcrossSuccessfulDecodes checks that the
// count leading to quarantine covers consecutive decode failures only.
func TestJournalDecodeFailuresDoNotAddUpAcrossSuccessfulDecodes(t *testing.T) {
	dir := t.TempDir()
	const id = "fedcba9876543210fedcba9876543210"
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(`{"event_type":`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The backend refuses writes, so a decoded record stays pending.
	sink := &controlledStorage{records: map[string]bool{}, fail: true, failed: make(chan struct{}, 1)}
	oldDelay := unreadableRetryDelay
	unreadableRetryDelay = 150 * time.Millisecond
	t.Cleanup(func() { unreadableRetryDelay = oldDelay })
	j, err := newJournalStorage(sink, dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	undecodable := func() int {
		j.mu.Lock()
		defer j.mu.Unlock()
		if entry := j.entries[id+".json"]; entry != nil {
			return entry.undecodable
		}
		return -1
	}
	waitFor(t, func() bool { return undecodable() >= 2 })

	// The same file now holds a valid record.
	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID("now-valid")
	payload, _ := json.Marshal(record)
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sink.failed:
	case <-time.After(5 * time.Second):
		t.Fatal("the decoded record was not handed to the backend")
	}
	waitFor(t, func() bool { return undecodable() == 0 })
	if _, err := os.Stat(path + quarantineSuffix); !os.IsNotExist(err) {
		t.Fatalf("entry quarantined although it decodes (stat error: %v)", err)
	}
}

func TestReadRetryDelayDoublesUpToTheLimit(t *testing.T) {
	oldBase, oldMax := unreadableRetryDelay, readRetryMaxDelay
	unreadableRetryDelay, readRetryMaxDelay = time.Second, 5*time.Minute
	t.Cleanup(func() { unreadableRetryDelay, readRetryMaxDelay = oldBase, oldMax })

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		32 * time.Second, 64 * time.Second, 128 * time.Second, 256 * time.Second, 5 * time.Minute, 5 * time.Minute}
	for i, delay := range want {
		if got := readRetryDelay(i + 1); got != delay {
			t.Errorf("readRetryDelay(%d) = %s, want %s", i+1, got, delay)
		}
	}
	if got := readRetryDelay(1 << 20); got != 5*time.Minute {
		t.Errorf("delay after many failures = %s, want the limit", got)
	}
	reportedAt := []int{}
	for n := 1; n <= 20; n++ {
		if reportsReadFailure(n) {
			reportedAt = append(reportedAt, n)
		}
	}
	if len(reportedAt) != 5 || reportedAt[4] != 16 {
		t.Errorf("failures reported at %v, want 1, 2, 4, 8, 16", reportedAt)
	}
}
