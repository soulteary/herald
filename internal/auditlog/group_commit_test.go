package auditlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
)

// TestGroupSyncCoversOnlyCallersThatArrivedBeforeItStarted pins the rule the
// durability guarantee rests on: a caller that arrives while a sync is
// running is not answered by that sync, because the sync may have started
// before the caller's rename. It waits for the next one.
func TestGroupSyncCoversOnlyCallersThatArrivedBeforeItStarted(t *testing.T) {
	var g groupSync
	var calls atomic.Int64
	firstStarted, releaseFirst := make(chan struct{}), make(chan struct{})
	syncFn := func() error {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-releaseFirst
		}
		return nil
	}

	first := make(chan error, 1)
	go func() { first <- g.do(syncFn) }()
	<-firstStarted

	// Three callers arrive during the first sync.
	late := make(chan error, 3)
	for i := 0; i < 3; i++ {
		go func() { late <- g.do(syncFn) }()
	}
	waitFor(t, func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.pending != nil
	})
	select {
	case err := <-late:
		t.Fatalf("a caller that arrived during a sync returned before a later sync ran: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseFirst)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := <-late; err != nil {
			t.Fatal(err)
		}
	}
	// The late callers were not covered by the first sync, and they needed
	// at most one sync each. They share one unless a caller had not yet
	// joined when the second sync started.
	if got := calls.Load(); got < 2 || got > 4 {
		t.Fatalf("%d syncs for one early and three late callers", got)
	}
}

// Every member of a group gets the result of the sync that covered it.
func TestGroupSyncReportsTheErrorToEveryCallerOfTheGroup(t *testing.T) {
	var g groupSync
	failure := errors.New("directory sync failed")
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	syncFn := func() error {
		switch calls.Add(1) {
		case 1:
			close(started)
			<-release
			return nil
		case 2:
			return failure
		}
		return nil
	}
	first := make(chan error, 1)
	go func() { first <- g.do(syncFn) }()
	<-started
	const members = 8
	results := make(chan error, members)
	var joined atomic.Int64
	for i := 0; i < members; i++ {
		go func() {
			joined.Add(1)
			results <- g.do(syncFn)
		}()
	}
	waitFor(t, func() bool { return joined.Load() == members })
	time.Sleep(50 * time.Millisecond) // let them reach the wait
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("the first caller got another group's error: %v", err)
	}
	failed := 0
	for i := 0; i < members; i++ {
		if err := <-results; errors.Is(err, failure) {
			failed++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	// All callers that formed the second group share its failure. A caller
	// that joined after that sync had started is in a third group.
	if failed == 0 || int(calls.Load()) > 2+members-failed {
		t.Fatalf("%d of %d callers got the failure after %d syncs", failed, members, calls.Load())
	}
}

// TestJournalConcurrentWritesAreDurableBeforeTheyReturn runs many concurrent
// writers against a directory sync that records which record files existed
// when it started. Each Write must return only after a sync that began with
// its file in place, and the writers must share syncs.
func TestJournalConcurrentWritesAreDurableBeforeTheyReturn(t *testing.T) {
	dir := t.TempDir()
	// The backend refuses writes so that the files stay in place.
	sink := &controlledStorage{records: map[string]bool{}, fail: true}
	j, err := newJournalStorage(sink, dir, 1<<30, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	var mu sync.Mutex
	covered := map[string]bool{} // challenge IDs whose file a completed sync started with
	var syncs atomic.Int64
	realSync := j.dirSync
	j.dirSync = func() error {
		syncs.Add(1)
		present := map[string]bool{}
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return err
		}
		for _, file := range files {
			payload, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			var record audit.Record
			if json.Unmarshal(payload, &record) == nil {
				present[record.ChallengeID] = true
			}
		}
		// Slow enough for other writers to queue up behind this sync.
		time.Sleep(5 * time.Millisecond)
		if err := realSync(); err != nil {
			return err
		}
		mu.Lock()
		for id := range present {
			covered[id] = true
		}
		mu.Unlock()
		return nil
	}

	const writers, perWriter = 16, 8
	var wg sync.WaitGroup
	failures := make(chan string, writers*perWriter)
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				id := fmt.Sprintf("writer-%d-record-%d", w, i)
				record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID(id)
				if err := j.Write(context.Background(), record); err != nil {
					failures <- id + ": " + err.Error()
					continue
				}
				mu.Lock()
				durable := covered[id]
				mu.Unlock()
				if !durable {
					failures <- id + ": Write returned before a directory sync that started after its rename had completed"
				}
			}
		}(w)
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}

	const total = writers * perWriter
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(files) != total {
		t.Fatalf("%d record files, want %d (err=%v)", len(files), total, err)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
	entries, pending := pendingEntries(j)
	var onDisk int64
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		onDisk += info.Size()
	}
	if entries != total || pending != onDisk {
		t.Fatalf("accounting: %d entries and %d bytes, want %d entries and %d bytes", entries, pending, total, onDisk)
	}
	if got := syncs.Load(); got >= total {
		t.Fatalf("%d directory syncs for %d concurrent writes: the writers did not share any", got, total)
	}
	t.Logf("%d writes shared %d directory syncs", total, syncs.Load())
}

func pendingEntries(j *journalStorage) (int, int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.entries), j.bytes
}

// The size limit holds when writers race for the last bytes: reservations
// are made under the lock before any file is written.
func TestJournalConcurrentWritesRespectTheSizeLimit(t *testing.T) {
	dir := t.TempDir()
	sink := &controlledStorage{records: map[string]bool{}, fail: true}
	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID("limit")
	probe := record.Copy()
	probe.EventID = strings.Repeat("0", 32)
	payload, err := json.Marshal(probe)
	if err != nil {
		t.Fatal(err)
	}
	const admitted = 5
	j, err := newJournalStorage(sink, dir, int64(len(payload))*admitted+1, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	var accepted, refused atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch err := j.Write(context.Background(), record); {
			case err == nil:
				accepted.Add(1)
			case strings.Contains(err.Error(), "capacity exceeded"):
				refused.Add(1)
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != admitted || refused.Load() != 40-admitted {
		t.Fatalf("accepted=%d refused=%d, want %d and %d", accepted.Load(), refused.Load(), admitted, 40-admitted)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*.json")); len(files) != admitted {
		t.Fatalf("%d files on disk, want %d", len(files), admitted)
	}
}

// A failed directory sync is returned to every writer that depended on it,
// and the bytes of a write whose file could not be created are released.
func TestJournalDirectorySyncFailureReachesEveryWaitingWriter(t *testing.T) {
	sink := &controlledStorage{records: map[string]bool{}, fail: true}
	j, err := newJournalStorage(sink, t.TempDir(), 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	failure := errors.New("injected directory sync failure")
	j.dirSync = func() error {
		time.Sleep(5 * time.Millisecond)
		return failure
	}
	record := audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess).WithChallengeID("sync-failure")
	var wg sync.WaitGroup
	var acknowledged atomic.Int64
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := j.Write(context.Background(), record); !errors.Is(err, failure) {
				acknowledged.Add(1)
			}
		}()
	}
	wg.Wait()
	if acknowledged.Load() != 0 {
		t.Fatalf("%d writes were acknowledged although the directory sync failed", acknowledged.Load())
	}
}

// Close lets a Write that was already accepted finish with its directory
// sync instead of closing the directory under it.
func TestJournalCloseWaitsForAcceptedWrites(t *testing.T) {
	dir := t.TempDir()
	j, err := newJournalStorage(audit.NewNoopStorage(), dir, 1<<20, 1, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	syncing, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	realSync := j.dirSync
	j.dirSync = func() error {
		once.Do(func() {
			close(syncing)
			<-release
		})
		return realSync()
	}
	written := make(chan error, 1)
	go func() {
		written <- j.Write(context.Background(), audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess))
	}()
	<-syncing
	closed := make(chan error, 1)
	go func() { closed <- j.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned while a write was syncing: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := j.Write(context.Background(), audit.NewRecord(audit.EventChallengeVerified, audit.ResultSuccess)); err == nil {
		t.Fatal("a write was accepted after Close began")
	}
	close(release)
	if err := <-written; err != nil {
		t.Fatalf("the accepted write failed during Close: %v", err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
}
