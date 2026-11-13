package auditlog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
)

// TestJournalQuarantinesUndecodableEntry starts a journal on a spool that
// holds one record file that is not valid JSON. It can never be delivered.
// The journal must stop retrying it, keep its bytes on disk under a
// quarantine name, report it, release its share of the size limit, and go on
// delivering good records. It used to retry the file every second forever
// and report a failure each time.
func TestJournalQuarantinesUndecodableEntry(t *testing.T) {
	dir := t.TempDir()
	const name = "0123456789abcdef0123456789abcdef.json"
	corrupt := []byte(`{"event_type":"challenge_created","challenge_id":`)
	if err := os.WriteFile(filepath.Join(dir, name), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var reported []string
	sink := &controlledStorage{records: map[string]bool{}}
	// The limit leaves room for one small record only while the corrupt
	// file's bytes are counted.
	limit := int64(len(corrupt)) + 600
	j, err := newJournalStorage(sink, dir, limit, 1, 1, func(err error) {
		mu.Lock()
		reported = append(reported, err.Error())
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	quarantined := filepath.Join(dir, name+".quarantined")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(quarantined); err == nil {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			defer mu.Unlock()
			t.Fatalf("undecodable entry was not quarantined; %d failures reported, last: %v", len(reported), reported)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The original bytes are kept, under the quarantine name only.
	kept, err := os.ReadFile(quarantined)
	if err != nil || string(kept) != string(corrupt) {
		t.Fatalf("quarantined content = %q, %v", kept, err)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("entry still present under its retry name: %v", err)
	}

	// It was reported, the last report names the quarantined file, and the
	// reports stop. The rename happens just before its report is delivered,
	// so wait for that report rather than reading the list at once.
	var count int
	var last string
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		count = len(reported)
		if count == 0 {
			return false
		}
		last = reported[count-1]
		return strings.Contains(last, name+".quarantined")
	})
	if count > 5 {
		t.Fatalf("want at most 5 reports before quarantine, got %d", count)
	}
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	after := len(reported)
	mu.Unlock()
	if after != count {
		t.Fatalf("entry is still being retried: %d reports became %d", count, after)
	}

	// Its bytes no longer count towards the limit, and good records flow.
	for i := 0; i < 3; i++ {
		record := audit.NewRecord(audit.EventChallengeCreated, audit.ResultSuccess).WithChallengeID("after-quarantine")
		if err := j.Write(context.Background(), record); err != nil {
			t.Fatalf("write %d after quarantine: %v", i, err)
		}
		waitFor(t, func() bool {
			pending, _ := filepath.Glob(filepath.Join(dir, "*.json"))
			return len(pending) == 0
		})
	}
	sink.mu.Lock()
	delivered := sink.records["after-quarantine"]
	sink.mu.Unlock()
	if !delivered {
		t.Fatal("record written after the quarantine was not delivered")
	}

	// A restart leaves the quarantined file alone and counts it.
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := newJournalStorage(sink, dir, limit, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restarted.Close() }()
	if _, err := os.Stat(quarantined); err != nil {
		t.Fatalf("quarantined file removed by restart: %v", err)
	}
}
