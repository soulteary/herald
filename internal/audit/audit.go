package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sirupsen/logrus"
	rediskitcache "github.com/soulteary/redis-kit/cache"

	"github.com/soulteary/herald/internal/config"
)

const (
	auditKeyPrefix = "otp:audit:"
)

// EventType represents the type of audit event
type EventType string

const (
	EventChallengeCreated   EventType = "challenge_created"
	EventChallengeVerified  EventType = "challenge_verified"
	EventChallengeRevoked   EventType = "challenge_revoked"
	EventSendSuccess        EventType = "send_success"
	EventSendFailed         EventType = "send_failed"
	EventVerificationFailed EventType = "verification_failed"
)

// AuditRecord represents an audit log entry
type AuditRecord struct {
	EventType         EventType `json:"event_type"`
	ChallengeID       string    `json:"challenge_id,omitempty"`
	UserID            string    `json:"user_id,omitempty"`
	Channel           string    `json:"channel,omitempty"`
	Destination       string    `json:"destination,omitempty"` // May be masked
	Purpose           string    `json:"purpose,omitempty"`
	Result            string    `json:"result"` // "success" | "failure"
	Reason            string    `json:"reason,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	ProviderMessageID string    `json:"provider_message_id,omitempty"`
	IP                string    `json:"ip,omitempty"`
	Timestamp         int64     `json:"timestamp"`
}

// Manager handles audit logging
type Manager struct {
	cache rediskitcache.Cache
}

// NewManager creates a new audit manager
func NewManager(redisClient *redis.Client) *Manager {
	cache := rediskitcache.NewCache(redisClient, auditKeyPrefix)
	return &Manager{
		cache: cache,
	}
}

// Log records an audit event
func (m *Manager) Log(ctx context.Context, record *AuditRecord) {
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

	if err := m.cache.Set(ctx, key, record, ttl); err != nil {
		logrus.Warnf("Failed to store audit record: %v", err)
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

// MaskDestination masks a destination (phone or email) based on channel
func MaskDestination(dest string, channel string) string {
	if dest == "" {
		return ""
	}

	switch channel {
	case "sms", "phone":
		// Phone number masking: +8613800138000 -> +861380****8000
		// Keep first 6 digits and last 4 digits
		if len(dest) <= 10 {
			return "****"
		}
		if len(dest) <= 14 {
			// Short number: keep first 3 and last 3
			return dest[:3] + "****" + dest[len(dest)-3:]
		}
		// Long number: keep first 6 and last 4
		return dest[:6] + "****" + dest[len(dest)-4:]
	case "email":
		// Email masking: user@example.com -> u***@example.com
		parts := strings.Split(dest, "@")
		if len(parts) != 2 {
			return "****"
		}
		localPart := parts[0]
		domain := parts[1]
		if len(localPart) == 0 {
			return "****@" + domain
		}
		if len(localPart) == 1 {
			return localPart[0:1] + "***@" + domain
		}
		return localPart[0:1] + "***@" + domain
	default:
		// Unknown channel, mask everything
		return "****"
	}
}
