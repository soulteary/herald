package auditlog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	audit "github.com/soulteary/audit-kit/v2"
)

type journalEntry struct {
	size int64
	busy bool
	next time.Time
	// undecodable counts consecutive attempts that read the file and could
	// not decode it. A successful decode resets it. Read errors and backend
	// delivery failures do not count: both are retried for as long as they
	// last.
	undecodable int
	// readFailures counts consecutive attempts that could not read the
	// file. It sets the retry delay and is reset by a successful read.
	readFailures int
}

// quarantineSuffix is appended to the name of a spool file that was read and
// could not be decoded. A quarantined file is kept for inspection, is not retried,
// is not picked up again after a restart and does not count towards the spool
// size limit. Renaming it back to its ".json" name makes the next start retry
// it.
const quarantineSuffix = ".quarantined"

// maxUnreadableAttempts is how often an undecodable spool file is tried
// before it is quarantined.
const maxUnreadableAttempts = 5

// unreadableRetryDelay, readRetryMaxDelay and quarantineRetryDelay are
// variables so tests need not wait for them. The first is the delay between
// attempts on an undecodable file and the first delay after a read error; the
// delay after further read errors doubles up to the second. The third applies
// when the rename into quarantine itself fails and the entry has to stay in
// place.
var (
	unreadableRetryDelay = time.Second
	readRetryMaxDelay    = 5 * time.Minute
	quarantineRetryDelay = time.Hour
)

// readRetryDelay is the wait before the next attempt on a file that could not
// be read `failures` times in a row: the base delay, doubled per failure, at
// most readRetryMaxDelay.
func readRetryDelay(failures int) time.Duration {
	delay := unreadableRetryDelay
	for i := 1; i < failures && delay < readRetryMaxDelay; i++ {
		delay *= 2
	}
	if delay > readRetryMaxDelay {
		delay = readRetryMaxDelay
	}
	return delay
}

// reportsReadFailure says whether the nth consecutive read failure of an
// entry is reported: the 1st, 2nd, 4th, 8th and so on. A file that stays
// unreadable is retried indefinitely, and reporting every attempt would log
// an error and count a failure each time.
func reportsReadFailure(failures int) bool {
	return failures > 0 && failures&(failures-1) == 0
}

// QuarantinedError is passed to the failure callback when a spool file is set
// aside. Name is the quarantined file's base name.
type QuarantinedError struct {
	Name string
	Err  error
}

func (e *QuarantinedError) Error() string {
	return fmt.Sprintf("audit spool entry quarantined as %s after %d failed attempts to decode it: %v", e.Name, maxUnreadableAttempts, e.Err)
}

func (e *QuarantinedError) Unwrap() error { return e.Err }

// journalStorage acknowledges a record only after its file and directory entry
// have been synced. The channel contains wake-up hints, never the only copy of
// a record. Backend failures and shutdown leave the files available for replay.
type journalStorage struct {
	mu                sync.Mutex
	dir               string
	dirFile, lockFile *os.File
	backend           audit.Storage
	entries           map[string]*journalEntry
	bytes, limit      int64
	wake              chan struct{}
	ctx               context.Context
	cancel            context.CancelFunc
	done              chan struct{}
	closed            bool
	onFailure         func(error)
	// quarantinedAtStart is the number of quarantined files found when the
	// spool was opened.
	quarantinedAtStart int
}

func newJournalStorage(backend audit.Storage, dir string, limit int64, queue, workers int, onFailure func(error)) (*journalStorage, error) {
	if dir == "" || limit <= 0 {
		return nil, errors.New("audit spool directory and positive size limit are required")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create audit spool: %w", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("audit spool already in use: %w", err)
	}
	df, err := os.Open(dir)
	if err != nil {
		_ = lock.Close()
		return nil, err
	}
	if queue <= 0 {
		queue = 1000
	}
	if workers <= 0 {
		workers = 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	j := &journalStorage{dir: dir, dirFile: df, lockFile: lock, backend: backend, entries: make(map[string]*journalEntry), limit: limit, wake: make(chan struct{}, queue), ctx: ctx, cancel: cancel, done: make(chan struct{}), onFailure: onFailure}
	err = j.recoverPendingEntries()
	if err == nil {
		err = probeJournalDirectory(dir, df)
	}
	if err != nil {
		cancel()
		_ = df.Close()
		_ = lock.Close()
		return nil, err
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); j.run() }()
	}
	go func() { wg.Wait(); _ = backend.Close(); _ = df.Close(); _ = lock.Close(); close(j.done) }()
	return j, nil
}

func (j *journalStorage) Write(ctx context.Context, record *audit.Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cp := record.Copy()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	cp.EventID = hex.EncodeToString(id)
	payload, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return errors.New("audit spool is closed")
	}
	if int64(len(payload)) > j.limit-j.bytes {
		return errors.New("audit spool capacity exceeded")
	}
	name := cp.EventID + ".json"
	tmp := filepath.Join(j.dir, cp.EventID+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()
	_, err = f.Write(payload)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(j.dir, name)); err != nil {
		return err
	}
	j.entries[name] = &journalEntry{size: int64(len(payload))}
	j.bytes += int64(len(payload))
	if err = j.dirFile.Sync(); err != nil {
		return err
	}
	select {
	case j.wake <- struct{}{}:
	default:
	} // full hints queue does not drop records
	return nil
}

func (j *journalStorage) run() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if j.ctx.Err() != nil {
			return
		}
		name := j.claimReadyEntry()
		if name == "" {
			select {
			case <-j.ctx.Done():
				return
			case <-j.wake:
			case <-ticker.C:
			}
			continue
		}
		payload, err := os.ReadFile(filepath.Join(j.dir, name))
		if err != nil {
			// The content is unknown, so nothing says the record is bad.
			// A read can fail for reasons that pass (no free descriptor,
			// an I/O error, a permission change), and the record must
			// still be delivered afterwards.
			if err = j.recordReadFailure(name, err); err != nil && j.onFailure != nil {
				j.onFailure(err)
			}
			continue
		}
		var rec audit.Record
		if err = json.Unmarshal(payload, &rec); err != nil {
			// Not a delivery failure: the bytes were read and are not a
			// record, so retrying forever cannot succeed.
			err = j.recordUndecodable(name, err)
			if j.onFailure != nil {
				j.onFailure(err)
			}
			continue
		}
		j.recordDecoded(name)
		ctx, cancel := context.WithTimeout(j.ctx, 5*time.Second)
		err = j.backend.Write(ctx, &rec)
		cancel()
		j.mu.Lock()
		e := j.entries[name]
		if err == nil {
			err = os.Remove(filepath.Join(j.dir, name))
			if err == nil {
				j.bytes -= e.size
				delete(j.entries, name)
				err = j.dirFile.Sync()
			}
		}
		if _, ok := j.entries[name]; ok {
			e.busy = false
			e.next = time.Now().Add(time.Second)
		}
		j.mu.Unlock()
		if err != nil && j.onFailure != nil {
			j.onFailure(err)
		}
	}
}

// recordReadFailure notes one failed attempt to read a spool file and
// schedules the next one. The entry is never quarantined for this: it stays
// in the spool, counts towards the size limit and is retried with a growing
// delay until it can be read. The returned error is nil when this failure is
// not one of those reported; see reportsReadFailure.
func (j *journalStorage) recordReadFailure(name string, cause error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[name]
	if !ok {
		return cause
	}
	e.busy = false
	e.readFailures++
	delay := readRetryDelay(e.readFailures)
	e.next = time.Now().Add(delay)
	if !reportsReadFailure(e.readFailures) {
		return nil
	}
	return fmt.Errorf("audit spool entry %s could not be read (attempt %d, next in %s); it stays in the spool and is retried: %w", name, e.readFailures, delay, cause)
}

// recordDecoded clears the failure counts of an entry that was read and
// decoded, so earlier failures do not add up with later ones.
func (j *journalStorage) recordDecoded(name string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if e, ok := j.entries[name]; ok {
		e.readFailures, e.undecodable = 0, 0
	}
}

// recordUndecodable notes one attempt that read a spool file and could not
// decode it, and returns the error to report. After maxUnreadableAttempts
// consecutive such attempts the file is renamed into quarantine and dropped
// from the retry set; the returned error is then a *QuarantinedError.
func (j *journalStorage) recordUndecodable(name string, cause error) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	e, ok := j.entries[name]
	if !ok {
		return cause
	}
	e.busy = false
	e.readFailures = 0
	e.undecodable++
	if e.undecodable < maxUnreadableAttempts {
		e.next = time.Now().Add(unreadableRetryDelay)
		return fmt.Errorf("audit spool entry %s cannot be decoded (attempt %d of %d): %w", name, e.undecodable, maxUnreadableAttempts, cause)
	}
	quarantined := name + quarantineSuffix
	if err := os.Rename(filepath.Join(j.dir, name), filepath.Join(j.dir, quarantined)); err != nil {
		// Leave the entry where it is, but stop trying it every second.
		e.next = time.Now().Add(quarantineRetryDelay)
		return fmt.Errorf("audit spool entry %s cannot be decoded and could not be quarantined: %w", name, errors.Join(cause, err))
	}
	j.bytes -= e.size
	delete(j.entries, name)
	_ = j.dirFile.Sync()
	return &QuarantinedError{Name: quarantined, Err: cause}
}

// QuarantinedAtStart reports how many quarantined files the spool directory
// held when it was opened.
func (j *journalStorage) QuarantinedAtStart() int { return j.quarantinedAtStart }

func (j *journalStorage) Query(ctx context.Context, filter *audit.QueryFilter) ([]*audit.Record, error) {
	return j.backend.Query(ctx, filter)
}

func (j *journalStorage) Close() error {
	j.mu.Lock()
	j.closed = true
	j.cancel()
	j.mu.Unlock()
	select {
	case <-j.done:
		return nil
	case <-time.After(6 * time.Second):
		return errors.New("audit worker shutdown timed out; pending records remain on disk")
	}
}

func (j *journalStorage) recoverPendingEntries() error {
	dir := j.dir
	files, err := os.ReadDir(dir)
	if err == nil {
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".json") {
				info, e := f.Info()
				if e != nil {
					err = e
					break
				}
				j.entries[f.Name()] = &journalEntry{size: info.Size()}
				j.bytes += info.Size()
			} else if strings.HasSuffix(f.Name(), quarantineSuffix) {
				j.quarantinedAtStart++
			} else if strings.HasSuffix(f.Name(), ".tmp") {
				// Temporary files were never acknowledged to a caller.
				if e := os.Remove(filepath.Join(dir, f.Name())); e != nil {
					err = e
					break
				}
			}
		}
	}

	return err
}

func probeJournalDirectory(dir string, df *os.File) error {
	var err error
	if err == nil {
		var probe *os.File
		probe, err = os.CreateTemp(dir, ".probe-*.tmp")
		if err == nil {
			err = probe.Sync()
			closeErr := probe.Close()
			if err == nil {
				err = closeErr
			}
			removeErr := os.Remove(probe.Name())
			if err == nil {
				err = removeErr
			}
		}
	}
	if err == nil {
		err = df.Sync()
	} // also probes directory fsync support at startup

	return err
}

func (j *journalStorage) claimReadyEntry() string {
	j.mu.Lock()
	name := ""
	for n, e := range j.entries {
		if !e.busy && !time.Now().Before(e.next) {
			name = n
			e.busy = true
			break
		}
	}
	j.mu.Unlock()
	return name
}
