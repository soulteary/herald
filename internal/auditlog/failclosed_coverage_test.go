package auditlog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	audit "github.com/soulteary/audit-kit/v2"
	"github.com/soulteary/herald/internal/config"
)

func resetCoverageAudit(t *testing.T) {
	t.Helper()
	_ = Stop()
	auditLogger = nil
	auditStorage = nil
	auditInitErr = nil
	auditLoggerInit = sync.Once{}
	oldEnabled, oldEnv, oldType, oldURL, oldSpool := config.AuditEnabled, config.Env, config.AuditStorageType, config.AuditDatabaseURL, config.AuditSpoolDir
	oldLog := packageLogger.Load()
	t.Cleanup(func() {
		_ = Stop()
		auditLogger = nil
		auditStorage = nil
		auditInitErr = nil
		auditLoggerInit = sync.Once{}
		config.AuditEnabled, config.Env, config.AuditStorageType, config.AuditDatabaseURL, config.AuditSpoolDir = oldEnabled, oldEnv, oldType, oldURL, oldSpool
		packageLogger.Store(oldLog)
	})
	SetLogger(testLogger())
	config.AuditSpoolDir = t.TempDir()
}

func coverageAuditCalls() []func() error {
	ctx := context.Background()
	return []func() error{
		func() error {
			return LogChallengeCreated(ctx, "ch", "user", "email", "private@example.com", "login", "127.0.0.1")
		},
		func() error {
			return LogSendSuccess(ctx, "ch", "user", "email", "private@example.com", "login", "smtp", "message", "127.0.0.1")
		},
		func() error {
			return LogSendFailed(ctx, "ch", "user", "email", "private@example.com", "login", "smtp", "timeout", "127.0.0.1")
		},
		func() error { return LogVerificationAttempt(ctx, "ch", "127.0.0.1") },
		func() error { return LogRevokeAttempt(ctx, "ch", "127.0.0.1") },
		func() error {
			return LogVerificationSuccess(ctx, "ch", "user", "email", "private@example.com", "login", "127.0.0.1")
		},
		func() error { return LogVerificationFailed(ctx, "ch", "invalid", "127.0.0.1") },
		func() error { return LogChallengeRevoked(ctx, "ch", "127.0.0.1") },
	}
}

func TestAuditDisabledAndFailedInitialization(t *testing.T) {
	resetCoverageAudit(t)
	config.AuditEnabled = false
	for _, call := range coverageAuditCalls() {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	config.AuditEnabled = true
	auditLoggerInit.Do(func() {})
	auditInitErr = errors.New("storage initialization failed")
	for _, call := range coverageAuditCalls() {
		if err := call(); err == nil {
			t.Fatal("unavailable audit writer accepted an event")
		}
	}
	if records, err := Query(context.Background(), nil); records != nil || err != nil {
		t.Fatalf("uninitialized query = %v %v", records, err)
	}
	if err := Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestAuditStartupBackendAndSpoolFailures(t *testing.T) {
	for _, kind := range []string{"production backend", "development fallback", "spool", "redis"} {
		t.Run(kind, func(t *testing.T) {
			resetCoverageAudit(t)
			config.AuditEnabled = true
			config.Env = config.EnvProduction
			config.AuditStorageType = "database"
			config.AuditDatabaseURL = "unsupported://bad"
			var rc *redis.Client
			if kind == "development fallback" {
				config.Env = config.EnvDevelopment
			}
			if kind == "spool" {
				config.AuditStorageType = "none"
				config.AuditSpoolDir = filepath.Join(t.TempDir(), "file")
				if err := os.WriteFile(config.AuditSpoolDir, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "redis" {
				mr := miniredis.RunT(t)
				rc = redis.NewClient(&redis.Options{Addr: mr.Addr()})
				t.Cleanup(func() { _ = rc.Close() })
				config.AuditEnabled = false
				config.AuditStorageType = "redis"
			}
			err := InitWithError(rc)
			if kind == "production backend" || kind == "spool" {
				if err == nil {
					t.Fatal("broken required storage accepted")
				}
				if again := InitWithError(rc); again != err {
					t.Fatal("initialization error was not retained")
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

type coverageAuditSink struct {
	records []*audit.Record
	err     error
}

func (s *coverageAuditSink) Write(_ context.Context, r *audit.Record) error {
	s.records = append(s.records, r)
	return s.err
}
func (*coverageAuditSink) Query(context.Context, *audit.QueryFilter) ([]*audit.Record, error) {
	return nil, nil
}
func (*coverageAuditSink) Close() error { return nil }

func TestAuditIntentAndPersistenceErrors(t *testing.T) {
	resetCoverageAudit(t)
	config.AuditEnabled = true
	auditLoggerInit.Do(func() {})
	sink := &coverageAuditSink{}
	auditLogger = audit.NewLogger(sink, audit.DefaultConfig())
	auditStorage = sink
	for _, call := range coverageAuditCalls() {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.records) != 8 {
		t.Fatalf("event count = %d", len(sink.records))
	}
	if string(sink.records[3].EventType) != "verification_attempt" || string(sink.records[4].EventType) != "challenge_revoke_requested" {
		t.Fatal("intent events were not persisted")
	}
	want := errors.New("disk full")
	sink.err = want
	if err := LogVerificationAttempt(context.Background(), "ch", "ip"); !errors.Is(err, want) {
		t.Fatalf("disk error hidden: %v", err)
	}
	auditStorage = nil
	if err := LogRevokeAttempt(context.Background(), "ch", "ip"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing storage accepted: %v", err)
	}
}
