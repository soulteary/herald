package auditlog

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
	logger "github.com/soulteary/logger-kit/v3"

	"github.com/soulteary/herald/internal/metrics"
)

// Unreadable entries are retried a few times before quarantine. The tests do
// not need a second between attempts.
func init() {
	unreadableRetryDelay = 20 * time.Millisecond
}

func quarantinedCount(t *testing.T) float64 {
	t.Helper()
	families, err := metrics.Registry.Gatherer().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "herald_audit_quarantined_total" {
			return family.GetMetric()[0].GetCounter().GetValue()
		}
	}
	return 0
}

func TestJournalFailureReporterSeparatesQuarantineFromRetry(t *testing.T) {
	var logs bytes.Buffer
	report := journalFailureReporter(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON, Output: &logs}))

	before := quarantinedCount(t)
	report(errors.New("backend down"))
	if got := quarantinedCount(t); got != before {
		t.Fatalf("a delivery failure counted as a quarantine: %v -> %v", before, got)
	}
	if !strings.Contains(logs.String(), "undelivered records remain in spool") {
		t.Fatalf("delivery failure log changed: %s", logs.String())
	}

	logs.Reset()
	report(&QuarantinedError{Name: "abc.json.quarantined", Err: errors.New("unexpected end of JSON input")})
	if got := quarantinedCount(t); got != before+1 {
		t.Fatalf("quarantine counter = %v, want %v", got, before+1)
	}
	for _, want := range []string{"was quarantined", `"file":"abc.json.quarantined"`, "unexpected end of JSON input"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("quarantine log does not contain %q: %s", want, logs.String())
		}
	}

	// A nil logger must not panic.
	journalFailureReporter(nil)(&QuarantinedError{Name: "x", Err: errors.New("y")})
	journalFailureReporter(nil)(errors.New("z"))
}

func TestQuarantinedErrorUnwrapsItsCause(t *testing.T) {
	cause := errors.New("decode failure")
	err := error(&QuarantinedError{Name: "abc.json.quarantined", Err: cause})
	if !errors.Is(err, cause) || !strings.Contains(err.Error(), "abc.json.quarantined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJournalCountsQuarantinedFilesAtStart(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.json.quarantined", "b.json.quarantined"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	j, err := newJournalStorage(audit.NewNoopStorage(), dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	if got := j.QuarantinedAtStart(); got != 2 {
		t.Fatalf("QuarantinedAtStart() = %d, want 2", got)
	}
	if j.bytes != 0 || len(j.entries) != 0 {
		t.Fatalf("quarantined files were taken into the retry set: bytes=%d entries=%d", j.bytes, len(j.entries))
	}
}

// When the rename into quarantine fails the entry stays where it is, is
// reported, and is not tried again for a long time.
func TestJournalKeepsEntryWhenQuarantineRenameFails(t *testing.T) {
	dir := t.TempDir()
	const name = "ffffffffffffffffffffffffffffffff.json"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory at the quarantine name makes the rename fail.
	blocker := filepath.Join(dir, name+quarantineSuffix)
	if err := os.MkdirAll(filepath.Join(blocker, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	reports := make(chan error, 16)
	j, err := newJournalStorage(audit.NewNoopStorage(), dir, 1<<20, 1, 1, func(err error) { reports <- err })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()

	deadline := time.After(10 * time.Second)
	for {
		select {
		case err := <-reports:
			if !strings.Contains(err.Error(), "could not be quarantined") {
				continue
			}
			var quarantined *QuarantinedError
			if errors.As(err, &quarantined) {
				t.Fatalf("a failed rename was reported as a quarantine: %v", err)
			}
			if _, statErr := os.Stat(filepath.Join(dir, name)); statErr != nil {
				t.Fatalf("entry lost although it was not quarantined: %v", statErr)
			}
			j.mu.Lock()
			entry := j.entries[name]
			j.mu.Unlock()
			if entry == nil || time.Until(entry.next) < time.Minute {
				t.Fatalf("entry is not backed off: %+v", entry)
			}
			return
		case <-deadline:
			t.Fatal("failed quarantine was not reported")
		}
	}
}
