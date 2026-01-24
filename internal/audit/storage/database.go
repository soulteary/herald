package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	_ "github.com/lib/pq"              // PostgreSQL driver
	"github.com/sirupsen/logrus"

	"github.com/soulteary/herald/internal/audit/types"
)

// DatabaseStorage implements Storage interface for database-based audit logging
// Supports PostgreSQL and MySQL
type DatabaseStorage struct {
	db     *sql.DB
	dbType string // "postgres" or "mysql"
}

// NewDatabaseStorage creates a new database storage instance
func NewDatabaseStorage(databaseURL string) (*DatabaseStorage, error) {
	// Detect database type from URL
	var dbType string
	var driver string
	var dsn string
	if len(databaseURL) >= 10 && databaseURL[:10] == "postgres://" {
		dbType = "postgres"
		driver = "postgres"
		dsn = databaseURL
	} else if len(databaseURL) >= 8 && databaseURL[:8] == "mysql://" {
		dbType = "mysql"
		driver = "mysql"
		// Convert mysql:// to DSN format
		// mysql://user:password@tcp(host:port)/dbname -> user:password@tcp(host:port)/dbname
		dsn = databaseURL[8:]
	} else {
		return nil, fmt.Errorf("unsupported database URL format, must start with postgres:// or mysql://")
	}

	// Open database connection
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	storage := &DatabaseStorage{
		db:     db,
		dbType: dbType,
	}

	// Create table if it doesn't exist
	if err := storage.createTable(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	logrus.Infof("Database storage initialized: type=%s", dbType)
	return storage, nil
}

// createTable creates the audit_logs table if it doesn't exist
func (s *DatabaseStorage) createTable(ctx context.Context) error {
	var createTableSQL string
	if s.dbType == "postgres" {
		createTableSQL = `
		CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGSERIAL PRIMARY KEY,
			event_type VARCHAR(50) NOT NULL,
			challenge_id VARCHAR(100),
			user_id VARCHAR(100),
			channel VARCHAR(20),
			destination VARCHAR(255),
			purpose VARCHAR(50),
			result VARCHAR(20),
			reason VARCHAR(100),
			provider VARCHAR(50),
			provider_message_id VARCHAR(255),
			ip VARCHAR(45),
			timestamp BIGINT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_audit_user_id ON audit_logs(user_id);
		CREATE INDEX IF NOT EXISTS idx_audit_challenge_id ON audit_logs(challenge_id);
		CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at);
		CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_logs(event_type);
		CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
		`
	} else {
		// MySQL
		createTableSQL = `
		CREATE TABLE IF NOT EXISTS audit_logs (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			event_type VARCHAR(50) NOT NULL,
			challenge_id VARCHAR(100),
			user_id VARCHAR(100),
			channel VARCHAR(20),
			destination VARCHAR(255),
			purpose VARCHAR(50),
			result VARCHAR(20),
			reason VARCHAR(100),
			provider VARCHAR(50),
			provider_message_id VARCHAR(255),
			ip VARCHAR(45),
			timestamp BIGINT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			INDEX idx_audit_user_id (user_id),
			INDEX idx_audit_challenge_id (challenge_id),
			INDEX idx_audit_created_at (created_at),
			INDEX idx_audit_event_type (event_type),
			INDEX idx_audit_timestamp (timestamp)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
		`
	}

	_, err := s.db.ExecContext(ctx, createTableSQL)
	return err
}

// Write writes an audit record to the database
func (s *DatabaseStorage) Write(ctx context.Context, record *types.AuditRecord) error {
	query := `
		INSERT INTO audit_logs (
			event_type, challenge_id, user_id, channel, destination,
			purpose, result, reason, provider, provider_message_id,
			ip, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	if s.dbType == "postgres" {
		// PostgreSQL uses $1, $2, etc.
		query = `
		INSERT INTO audit_logs (
			event_type, challenge_id, user_id, channel, destination,
			purpose, result, reason, provider, provider_message_id,
			ip, timestamp
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`
	}

	_, err := s.db.ExecContext(ctx, query,
		string(record.EventType),
		record.ChallengeID,
		record.UserID,
		record.Channel,
		record.Destination,
		record.Purpose,
		record.Result,
		record.Reason,
		record.Provider,
		record.ProviderMessageID,
		record.IP,
		record.Timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to insert audit record: %w", err)
	}

	return nil
}

// Query queries audit records from the database
func (s *DatabaseStorage) Query(ctx context.Context, filter *QueryFilter) ([]*types.AuditRecord, error) {
	if filter == nil {
		filter = DefaultQueryFilter()
	}

	// Build WHERE clause
	whereClauses := []string{}
	args := []interface{}{}
	argIndex := 1

	if filter.EventType != "" {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("event_type = $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "event_type = ?")
		}
		args = append(args, filter.EventType)
		argIndex++
	}

	if filter.UserID != "" {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("user_id = $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "user_id = ?")
		}
		args = append(args, filter.UserID)
		argIndex++
	}

	if filter.ChallengeID != "" {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("challenge_id = $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "challenge_id = ?")
		}
		args = append(args, filter.ChallengeID)
		argIndex++
	}

	if filter.Channel != "" {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("channel = $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "channel = ?")
		}
		args = append(args, filter.Channel)
		argIndex++
	}

	if filter.Result != "" {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("result = $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "result = ?")
		}
		args = append(args, filter.Result)
		argIndex++
	}

	if filter.StartTime > 0 {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp >= $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "timestamp >= ?")
		}
		args = append(args, filter.StartTime)
		argIndex++
	}

	if filter.EndTime > 0 {
		if s.dbType == "postgres" {
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp <= $%d", argIndex))
		} else {
			whereClauses = append(whereClauses, "timestamp <= ?")
		}
		args = append(args, filter.EndTime)
		argIndex++
	}

	// Build query
	whereClause := ""
	if len(whereClauses) > 0 {
		whereClause = "WHERE " + whereClauses[0]
		for i := 1; i < len(whereClauses); i++ {
			whereClause += " AND " + whereClauses[i]
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`
		SELECT event_type, challenge_id, user_id, channel, destination,
		       purpose, result, reason, provider, provider_message_id,
		       ip, timestamp
		FROM audit_logs
		%s
		ORDER BY timestamp DESC
		LIMIT %d OFFSET %d
	`, whereClause, limit, offset)

	if s.dbType == "postgres" {
		query = fmt.Sprintf(`
		SELECT event_type, challenge_id, user_id, channel, destination,
		       purpose, result, reason, provider, provider_message_id,
		       ip, timestamp
		FROM audit_logs
		%s
		ORDER BY timestamp DESC
		LIMIT $%d OFFSET $%d
		`, whereClause, argIndex, argIndex+1)
		args = append(args, limit, offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit records: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []*types.AuditRecord
	for rows.Next() {
		record := &types.AuditRecord{}
		err := rows.Scan(
			&record.EventType,
			&record.ChallengeID,
			&record.UserID,
			&record.Channel,
			&record.Destination,
			&record.Purpose,
			&record.Result,
			&record.Reason,
			&record.Provider,
			&record.ProviderMessageID,
			&record.IP,
			&record.Timestamp,
		)
		if err != nil {
			logrus.Warnf("Failed to scan audit record: %v", err)
			continue
		}
		results = append(results, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// Close closes the database connection
func (s *DatabaseStorage) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
