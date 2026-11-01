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
}

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
		var rec audit.Record
		if err == nil {
			err = json.Unmarshal(payload, &rec)
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(j.ctx, 5*time.Second)
			err = j.backend.Write(ctx, &rec)
			cancel()
		}
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
