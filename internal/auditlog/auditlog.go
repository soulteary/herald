package auditlog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	audit "github.com/soulteary/audit-kit/v2"
	redisstore "github.com/soulteary/audit-kit/v2/redisstore"
	logger "github.com/soulteary/logger-kit/v3"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/metrics"
)

var packageLogger atomic.Pointer[logger.Logger]

// SetLogger sets the logger used by the next audit-writer initialization.
// Atomic storage keeps concurrent handler construction race-free.
func SetLogger(l *logger.Logger) {
	packageLogger.Store(l)
}

var (
	auditLogger     *audit.Logger
	auditLoggerInit sync.Once
	auditInitErr    error
	auditStorage    audit.Storage
)

// Init initializes the audit logger with the given storage. Any initialization
// error is retained and can be inspected via InitWithError; callers that need
// fail-closed behavior in production should use InitWithError.
func Init(redisClient *redis.Client) {
	_ = InitWithError(redisClient)
}

// InitWithError initializes the audit logger and returns an error if the
// configured storage backend could not be created. In production a failed
// storage init is a hard error (the caller should refuse to start) instead of
// silently degrading to a no-op sink that loses the audit trail.
func InitWithError(redisClient *redis.Client) error {
	auditLoggerInit.Do(func() {
		// Capture one logger for this writer. Later handler construction must not
		// reroute callbacks that belong to an already-running audit writer.
		currentLog := packageLogger.Load()
		cfg := audit.DefaultConfig()
		cfg.Enabled = config.AuditEnabled
		cfg.MaskDestination = config.AuditMaskDestination

		// Create storage based on config
		var storage audit.Storage
		var err error

		storageType := audit.ParseStorageType(config.AuditStorageType)
		if storageType != audit.StorageTypeNone && storageType != "" {
			opts := &audit.StorageOptions{
				FilePath:    config.AuditFilePath,
				DatabaseURL: config.AuditDatabaseURL,
				TableName:   config.AuditTableName,
			}

			// Add Redis storage if client provided. CloseClient stays false:
			// the client is shared with routing, rate limiting and caching, so
			// stopping the audit writer must not close it.
			if redisClient != nil {
				opts.RedisStorage = redisstore.NewWithConfig(redisClient, &redisstore.Config{
					KeyPrefix: "otp:audit:",
					TTL:       config.AuditTTL,
				})
			}

			storage, err = audit.NewStorageFromType(storageType, opts)
			if err != nil {
				// In production, a configured-but-broken audit backend must not be
				// silently swapped for a no-op sink.
				if config.AuditEnabled && config.IsProduction() {
					auditInitErr = fmt.Errorf("audit: failed to initialize %s storage: %w", storageType, err)
					return
				}
				if currentLog != nil {
					currentLog.Warn().Err(err).Msg("Failed to initialize audit storage, using no-op storage")
				}
				storage = audit.NewNoopStorage()
			}
		} else if redisClient != nil {
			// Default to Redis storage if client provided
			storage = redisstore.NewWithConfig(redisClient, &redisstore.Config{
				KeyPrefix: "otp:audit:",
				TTL:       config.AuditTTL,
			})
		} else {
			storage = audit.NewNoopStorage()
		}

		if cfg.Enabled {
			journal, err := newJournalStorage(storage, config.AuditSpoolDir, int64(config.AuditSpoolMaxBytes), config.AuditWriterQueueSize, config.AuditWriterWorkers, journalFailureReporter(currentLog))
			if err != nil {
				_ = storage.Close()
				auditInitErr = fmt.Errorf("initialize audit spool: %w", err)
				return
			}
			if n := journal.QuarantinedAtStart(); n > 0 && currentLog != nil {
				currentLog.Warn().Int("quarantined_files", n).Str("spool_dir", config.AuditSpoolDir).
					Msg("audit spool holds quarantined records that were never delivered; inspect the *" + quarantineSuffix + " files")
			}
			storage = journal
		}
		auditStorage = storage
		// The logger is synchronous here: Write confirms durable local acceptance;
		// journalStorage performs backend delivery in the background.
		auditLogger = audit.NewLogger(storage, cfg)
	})
	return auditInitErr
}

// journalFailureReporter returns the callback the journal reports to. A
// quarantined entry is a lost audit record, not a retry, so it has its own
// counter and message.
func journalFailureReporter(log *logger.Logger) func(error) {
	return func(err error) {
		var quarantined *QuarantinedError
		if errors.As(err, &quarantined) {
			metrics.RecordAuditQuarantined()
			if log != nil {
				log.Error().Err(quarantined.Err).Str("file", quarantined.Name).Str("spool_dir", config.AuditSpoolDir).
					Msg("audit spool entry could not be read or decoded and was quarantined; it will not be delivered or retried")
			}
			return
		}
		metrics.RecordAuditWriteFailure()
		if log != nil {
			log.Error().Err(err).Msg("audit delivery or cleanup failed; undelivered records remain in spool")
		}
	}
}

// GetLogger returns the audit logger instance
func GetLogger() *audit.Logger {
	if auditLogger == nil {
		Init(nil)
	}
	return auditLogger
}

// Stop stops the audit logger
func Stop() error {
	if auditLogger != nil {
		return auditLogger.Stop()
	}
	return nil
}

// LogChallengeCreated records a challenge creation event
func LogChallengeCreated(ctx context.Context, challengeID, userID, channel, destination, purpose, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventChallengeCreated, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordIP(ip),
	)
}

// LogSendSuccess records a successful send event
func LogSendSuccess(ctx context.Context, challengeID, userID, channel, destination, purpose, provider, messageID, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventSendSuccess, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordProvider(provider, messageID),
		audit.WithRecordIP(ip),
	)
}

// LogSendFailed records a failed send event
func LogSendFailed(ctx context.Context, challengeID, userID, channel, destination, purpose, provider, reason, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventSendFailed, challengeID, userID, audit.ResultFailure,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordProvider(provider, ""),
		audit.WithRecordReason(reason),
		audit.WithRecordIP(ip),
	)
}

// LogVerificationAttempt durably records intent before a code can be consumed.
func LogVerificationAttempt(ctx context.Context, challengeID, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	if GetLogger() == nil {
		return fmt.Errorf("audit logger unavailable")
	}
	return logChallenge(ctx, audit.EventType("verification_attempt"), challengeID, "", audit.ResultPending, audit.WithRecordIP(ip))
}

// LogRevokeAttempt durably records intent before challenge state is changed.
func LogRevokeAttempt(ctx context.Context, challengeID, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	if GetLogger() == nil {
		return fmt.Errorf("audit logger unavailable")
	}
	return logChallenge(ctx, audit.EventType("challenge_revoke_requested"), challengeID, "", audit.ResultPending, audit.WithRecordIP(ip))
}

// LogVerificationSuccess records a successful verification event
func LogVerificationSuccess(ctx context.Context, challengeID, userID, channel, destination, purpose, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventChallengeVerified, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordIP(ip),
	)
}

// LogVerificationFailed records a failed verification event
func LogVerificationFailed(ctx context.Context, challengeID, reason, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventVerificationFailed, challengeID, "", audit.ResultFailure,
		audit.WithRecordReason(reason),
		audit.WithRecordIP(ip),
	)
}

// LogChallengeRevoked records a challenge revocation event
func LogChallengeRevoked(ctx context.Context, challengeID, ip string) error {
	if !config.AuditEnabled {
		return nil
	}
	l := GetLogger()
	if l == nil {
		return fmt.Errorf("audit logger unavailable: %w", auditInitErr)
	}

	return logChallenge(ctx, audit.EventChallengeRevoked, challengeID, "", audit.ResultSuccess,
		audit.WithRecordIP(ip),
	)
}

// logChallenge acknowledges only durable acceptance, never an in-memory queue.
func logChallenge(ctx context.Context, event audit.EventType, challengeID, userID string, result audit.Result, opts ...audit.RecordOption) error {
	record := audit.NewRecord(event, result).WithChallengeID(challengeID).WithUserID(userID)
	for _, opt := range opts {
		opt(record)
	}
	if config.AuditMaskDestination && record.Destination != "" {
		record.Destination = audit.MaskDestination(record.Destination, record.Channel)
	}
	if auditStorage == nil {
		return fmt.Errorf("audit storage unavailable")
	}
	if err := auditStorage.Write(ctx, record); err != nil {
		metrics.RecordAuditDropped()
		if l := packageLogger.Load(); l != nil {
			l.Error().Err(err).Msg("audit record could not be persisted")
		}
		return err
	}
	return nil
}

// Query queries audit records
func Query(ctx context.Context, filter *audit.QueryFilter) ([]*audit.Record, error) {
	l := GetLogger()
	if l == nil {
		return nil, nil
	}
	return l.Query(ctx, filter)
}
