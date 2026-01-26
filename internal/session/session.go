package session

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	logger "github.com/soulteary/logger-kit"
	rediskitcache "github.com/soulteary/redis-kit/cache"
	secure "github.com/soulteary/secure-kit"
)

// Session represents a session object stored in Redis
type Session struct {
	ID        string                 `json:"id"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
}

// Manager handles session storage operations
type Manager struct {
	cache      rediskitcache.Cache
	defaultTTL time.Duration
	log        *logger.Logger
}

// NewManager creates a new session manager
func NewManager(redisClient *redis.Client, keyPrefix string, defaultTTL time.Duration, log *logger.Logger) *Manager {
	cache := rediskitcache.NewCache(redisClient, keyPrefix)
	return &Manager{
		cache:      cache,
		defaultTTL: defaultTTL,
		log:        log,
	}
}

// Create creates a new session and stores it in Redis
// Returns the session ID and error
func (m *Manager) Create(ctx context.Context, data map[string]interface{}, ttl time.Duration) (string, error) {
	// Generate session ID
	sessionID := generateSessionID()

	// Use provided TTL or default TTL
	if ttl == 0 {
		ttl = m.defaultTTL
	}

	// Create session
	session := &Session{
		ID:        sessionID,
		Data:      data,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	// Store in Redis using cache interface
	if err := m.cache.Set(ctx, sessionID, session, ttl); err != nil {
		return "", fmt.Errorf("failed to store session: %w", err)
	}

	m.log.Debug().Str("session_id", sessionID).Dur("ttl", ttl).Msg("Session created")
	return sessionID, nil
}

// Get retrieves a session by ID
func (m *Manager) Get(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	if err := m.cache.Get(ctx, sessionID, &session); err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Check if expired (additional check, though Redis TTL should handle this)
	if time.Now().After(session.ExpiresAt) {
		// Delete expired session
		_ = m.cache.Del(ctx, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	return &session, nil
}

// Set updates session data
func (m *Manager) Set(ctx context.Context, sessionID string, data map[string]interface{}, ttl time.Duration) error {
	// Get existing session to preserve CreatedAt
	existingSession, err := m.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Use provided TTL or keep existing TTL
	if ttl == 0 {
		// Get remaining TTL from Redis
		remainingTTL, err := m.cache.TTL(ctx, sessionID)
		if err != nil || remainingTTL <= 0 {
			ttl = m.defaultTTL
		} else {
			ttl = remainingTTL
		}
	}

	// Update session
	session := &Session{
		ID:        sessionID,
		Data:      data,
		CreatedAt: existingSession.CreatedAt,
		ExpiresAt: time.Now().Add(ttl),
	}

	// Update in Redis with new TTL
	if err := m.cache.Set(ctx, sessionID, session, ttl); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	m.log.Debug().Str("session_id", sessionID).Dur("ttl", ttl).Msg("Session updated")
	return nil
}

// Delete removes a session from Redis
func (m *Manager) Delete(ctx context.Context, sessionID string) error {
	if err := m.cache.Del(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	m.log.Debug().Str("session_id", sessionID).Msg("Session deleted")
	return nil
}

// Exists checks if a session exists
func (m *Manager) Exists(ctx context.Context, sessionID string) (bool, error) {
	return m.cache.Exists(ctx, sessionID)
}

// Refresh extends the expiration time of a session
func (m *Manager) Refresh(ctx context.Context, sessionID string, ttl time.Duration) error {
	// Get existing session
	session, err := m.Get(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Use provided TTL or default TTL
	if ttl == 0 {
		ttl = m.defaultTTL
	}

	// Update expiration time
	session.ExpiresAt = time.Now().Add(ttl)

	// Update in Redis with new TTL
	if err := m.cache.Set(ctx, sessionID, session, ttl); err != nil {
		return fmt.Errorf("failed to refresh session: %w", err)
	}

	m.log.Debug().Str("session_id", sessionID).Dur("ttl", ttl).Msg("Session refreshed")
	return nil
}

// Helper functions

func generateSessionID() string {
	token, err := secure.RandomToken(16)
	if err != nil {
		// This should never happen with crypto/rand, but handle gracefully
		// Use fallback without logging since we don't have logger instance here
		token, _ = secure.RandomHex(16)
	}
	return "sess_" + token[:22]
}
