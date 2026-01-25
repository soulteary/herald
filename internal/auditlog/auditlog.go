package auditlog

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	audit "github.com/soulteary/audit-kit"

	"github.com/soulteary/herald/internal/config"
)

var (
	logger     *audit.Logger
	loggerInit sync.Once
)

// Init initializes the audit logger with the given storage
func Init(redisClient *redis.Client) {
	loggerInit.Do(func() {
		cfg := audit.DefaultConfig()
		cfg.Enabled = config.AuditEnabled
		cfg.MaskDestination = config.AuditMaskDestination
		cfg.TTL = config.AuditTTL

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
				logrus.Warnf("Failed to initialize audit storage: %v, using no-op storage", err)
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

		logger = audit.NewLoggerWithWriter(storage, cfg)
	})
}

// GetLogger returns the audit logger instance
func GetLogger() *audit.Logger {
	if logger == nil {
		Init(nil)
	}
	return logger
}

// Stop stops the audit logger
func Stop() error {
	if logger != nil {
		return logger.Stop()
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
