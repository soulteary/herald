package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit"

	"github.com/soulteary/herald/internal/audit/storage"
	"github.com/soulteary/herald/internal/audit/types"
	"github.com/soulteary/herald/internal/config"
)

const (
	auditKeyPrefix = "otp:audit:"
)

// Manager handles audit logging
type Manager struct {
	cache             rediskitcache.Cache
	persistentStorage storage.Storage // Long-term storage (optional)
	writer            *Writer         // Async writer for persistent storage
}

// NewManager creates a new audit manager
func NewManager(redisClient *redis.Client) *Manager {
	cache := rediskitcache.NewCache(redisClient, auditKeyPrefix)
	return &Manager{
		cache: cache,
	}
}

// NewManagerWithStorage creates a new audit manager with persistent storage
func NewManagerWithStorage(redisClient *redis.Client, persistentStorage storage.Storage, queueSize, workers int) *Manager {
	cache := rediskitcache.NewCache(redisClient, auditKeyPrefix)
	writer := NewWriter(persistentStorage, queueSize, workers)
	writer.Start()

	return &Manager{
		cache:             cache,
		persistentStorage: persistentStorage,
		writer:            writer,
	}
}

// Stop stops the async writer gracefully
func (m *Manager) Stop() error {
	if m.writer != nil {
		return m.writer.Stop()
	}
	return nil
}

// Log records an audit event
func (m *Manager) Log(ctx context.Context, record *types.AuditRecord) {
	if !config.AuditEnabled {
		return
	}

	// Set timestamp if not set
	if record.Timestamp == 0 {
		record.Timestamp = time.Now().Unix()
	}

	// Mask destination if configured
	if config.AuditMaskDestination && record.Destination != "" {
		record.Destination = MaskDestination(record.Destination, record.Channel)
	}

	// Generate audit key: otp:audit:{timestamp}:{challenge_id or user_id}
	var key string
	if record.ChallengeID != "" {
		key = fmt.Sprintf("%d:%s", record.Timestamp, record.ChallengeID)
	} else if record.UserID != "" {
		key = fmt.Sprintf("%d:%s", record.Timestamp, record.UserID)
	} else {
		key = fmt.Sprintf("%d", record.Timestamp)
	}

	// Store in Redis with TTL
	ttl := config.AuditTTL
	if ttl == 0 {
		ttl = 7 * 24 * time.Hour // Default 7 days
	}

	// Store in Redis (immediate, for short-term access)
	if err := m.cache.Set(ctx, key, record, ttl); err != nil {
		logrus.Warnf("Failed to store audit record in Redis: %v", err)
	}

	// Enqueue for persistent storage (async, for long-term storage)
	if m.writer != nil {
		if !m.writer.Enqueue(record) {
			logrus.Warnf("Failed to enqueue audit record for persistent storage (queue full)")
		}
	}

	// Also log to standard logger for immediate visibility
	logrus.WithFields(logrus.Fields{
		"event_type":   record.EventType,
		"challenge_id": record.ChallengeID,
		"user_id":      record.UserID,
		"channel":      record.Channel,
		"destination":  record.Destination,
		"result":       record.Result,
		"reason":       record.Reason,
	}).Info("Audit log")
}

// Query queries audit records from persistent storage
func (m *Manager) Query(ctx context.Context, filter *storage.QueryFilter) ([]*types.AuditRecord, error) {
	if m.persistentStorage == nil {
		return nil, fmt.Errorf("persistent storage not configured")
	}
	return m.persistentStorage.Query(ctx, filter)
}

// MaskDestination masks a destination (phone or email) based on channel
func MaskDestination(dest string, channel string) string {
	if dest == "" {
		return ""
	}

	switch channel {
	case "sms", "phone":
		// Use secure-kit MaskPhone for phone number masking
		return secure.MaskPhone(dest)
	case "email":
		// Use secure-kit MaskEmail for email masking
		return secure.MaskEmail(dest)
	default:
		// Unknown channel, mask everything
		return "****"
	}
}
