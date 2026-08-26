package auditlog

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/redis/go-redis/v9"
	audit "github.com/soulteary/audit-kit"
	logger "github.com/soulteary/logger-kit/v2"

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
		cfg.TTL = config.AuditTTL

		// Observe dropped/failed audit records instead of losing them silently.
		cfg.OnEnqueueFailed = func(_ *audit.Record) {
			metrics.RecordAuditDropped()
			if currentLog != nil {
				currentLog.Warn().Msg("audit record dropped: writer queue full")
			}
		}
		cfg.OnWriteFailed = func(_ *audit.Record, err error) {
			metrics.RecordAuditDropped()
			if currentLog != nil {
				currentLog.Error().Err(err).Msg("audit record write failed")
			}
		}

		// Configure async writer
		if config.AuditWriterQueueSize > 0 || config.AuditWriterWorkers > 0 {
			cfg.Writer = &audit.WriterConfig{
				QueueSize: config.AuditWriterQueueSize,
				Workers:   config.AuditWriterWorkers,
			}
		}

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

			// Add Redis storage if client provided
			if redisClient != nil {
				opts.RedisClient = redisClient
				opts.RedisPrefix = "otp:audit:"
				opts.RedisTTL = config.AuditTTL
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
			storage = audit.NewRedisStorageWithConfig(redisClient, &audit.RedisConfig{
				KeyPrefix: "otp:audit:",
				TTL:       config.AuditTTL,
			})
		} else {
			storage = audit.NewNoopStorage()
		}

		auditLogger = audit.NewLoggerWithWriter(storage, cfg)
	})
	return auditInitErr
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
func LogChallengeCreated(ctx context.Context, challengeID, userID, channel, destination, purpose, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventChallengeCreated, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordIP(ip),
	)
}

// LogSendSuccess records a successful send event
func LogSendSuccess(ctx context.Context, challengeID, userID, channel, destination, purpose, provider, messageID, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventSendSuccess, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordProvider(provider, messageID),
		audit.WithRecordIP(ip),
	)
}

// LogSendFailed records a failed send event
func LogSendFailed(ctx context.Context, challengeID, userID, channel, destination, purpose, provider, reason, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventSendFailed, challengeID, userID, audit.ResultFailure,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordProvider(provider, ""),
		audit.WithRecordReason(reason),
		audit.WithRecordIP(ip),
	)
}

// LogVerificationSuccess records a successful verification event
func LogVerificationSuccess(ctx context.Context, challengeID, userID, channel, destination, purpose, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventChallengeVerified, challengeID, userID, audit.ResultSuccess,
		audit.WithRecordChannel(channel),
		audit.WithRecordDestination(destination),
		audit.WithRecordPurpose(purpose),
		audit.WithRecordIP(ip),
	)
}

// LogVerificationFailed records a failed verification event
func LogVerificationFailed(ctx context.Context, challengeID, reason, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventVerificationFailed, challengeID, "", audit.ResultFailure,
		audit.WithRecordReason(reason),
		audit.WithRecordIP(ip),
	)
}

// LogChallengeRevoked records a challenge revocation event
func LogChallengeRevoked(ctx context.Context, challengeID, ip string) {
	l := GetLogger()
	if l == nil {
		return
	}

	l.LogChallenge(ctx, audit.EventChallengeRevoked, challengeID, "", audit.ResultSuccess,
		audit.WithRecordIP(ip),
	)
}

// Query queries audit records
func Query(ctx context.Context, filter *audit.QueryFilter) ([]*audit.Record, error) {
	l := GetLogger()
	if l == nil {
		return nil, nil
	}
	return l.Query(ctx, filter)
}
