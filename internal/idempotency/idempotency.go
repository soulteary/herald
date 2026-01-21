package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPrefix      = "otp:idem:"
	defaultTTL     = 5 * time.Minute
	statusPending  = "processing"
	statusComplete = "completed"
)

// Response is the cached response payload.
type Response struct {
	ChallengeID  string `json:"challenge_id"`
	ExpiresIn    int    `json:"expires_in"`
	NextResendIn int    `json:"next_resend_in"`
}

// Record stores idempotency status and response.
type Record struct {
	Status      string    `json:"status"`
	RequestHash string    `json:"request_hash"`
	Response    *Response `json:"response,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Manager handles idempotency operations.
type Manager struct {
	redis *redis.Client
	ttl   time.Duration
}

// NewManager creates a new idempotency manager.
func NewManager(redisClient *redis.Client, ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &Manager{
		redis: redisClient,
		ttl:   ttl,
	}
}

// Get retrieves an idempotency record.
func (m *Manager) Get(ctx context.Context, key string) (*Record, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	val, err := m.redis.Get(ctx, keyPrefix+key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get idempotency key: %w", err)
	}

	var record Record
	if err := json.Unmarshal(val, &record); err != nil {
		return nil, fmt.Errorf("failed to decode idempotency record: %w", err)
	}
	return &record, nil
}

// Begin reserves an idempotency key for processing.
func (m *Manager) Begin(ctx context.Context, key string, requestHash string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	record := Record{
		Status:      statusPending,
		RequestHash: requestHash,
		CreatedAt:   time.Now(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return false, fmt.Errorf("failed to encode idempotency record: %w", err)
	}

	ok, err := m.redis.SetNX(ctx, keyPrefix+key, data, m.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to reserve idempotency key: %w", err)
	}
	return ok, nil
}

// Complete stores the final response for an idempotency key.
func (m *Manager) Complete(ctx context.Context, key, requestHash string, response Response) error {
	if ctx == nil {
		ctx = context.Background()
	}
	record := Record{
		Status:      statusComplete,
		RequestHash: requestHash,
		Response:    &response,
		CreatedAt:   time.Now(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode idempotency record: %w", err)
	}
	if err := m.redis.Set(ctx, keyPrefix+key, data, m.ttl).Err(); err != nil {
		return fmt.Errorf("failed to persist idempotency record: %w", err)
	}
	return nil
}

// Delete removes an idempotency key.
func (m *Manager) Delete(ctx context.Context, key string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.redis.Del(ctx, keyPrefix+key).Err(); err != nil {
		return fmt.Errorf("failed to delete idempotency key: %w", err)
	}
	return nil
}

// Status helpers.
func IsProcessing(record *Record) bool {
	return record != nil && record.Status == statusPending
}

func IsCompleted(record *Record) bool {
	return record != nil && record.Status == statusComplete && record.Response != nil
}
