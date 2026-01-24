package storage

import (
	"context"

	"github.com/soulteary/herald/internal/audit/types"
)

// Storage defines the interface for audit log storage backends
type Storage interface {
	// Write writes an audit record to the storage backend
	Write(ctx context.Context, record *types.AuditRecord) error

	// Query queries audit records based on filter criteria
	// Returns records matching the filter, ordered by timestamp (newest first)
	Query(ctx context.Context, filter *QueryFilter) ([]*types.AuditRecord, error)

	// Close closes the storage connection and releases resources
	Close() error
}

// QueryFilter defines filter criteria for querying audit records
type QueryFilter struct {
	EventType   string // Filter by event type (e.g., "challenge_created")
	UserID      string // Filter by user ID
	ChallengeID string // Filter by challenge ID
	Channel     string // Filter by channel (e.g., "sms", "email")
	Result      string // Filter by result (e.g., "success", "failure")
	StartTime   int64  // Start timestamp (Unix timestamp)
	EndTime     int64  // End timestamp (Unix timestamp)
	Limit       int    // Maximum number of records to return (default: 100)
	Offset      int    // Offset for pagination (default: 0)
}

// DefaultQueryFilter returns a default query filter
func DefaultQueryFilter() *QueryFilter {
	return &QueryFilter{
		Limit: 100,
	}
}
