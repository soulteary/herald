package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/soulteary/herald/internal/audit/types"
)

// FileStorage implements Storage interface for file-based audit logging
// Uses JSON Lines format (one JSON object per line)
type FileStorage struct {
	filePath string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
}

// NewFileStorage creates a new file storage instance
func NewFileStorage(filePath string) (*FileStorage, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &FileStorage{
		filePath: filePath,
		file:     file,
		writer:   bufio.NewWriter(file),
	}, nil
}

// Write writes an audit record to the file (JSON Lines format)
func (s *FileStorage) Write(ctx context.Context, record *types.AuditRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Marshal record to JSON
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal audit record: %w", err)
	}

	// Write JSON line
	if _, err := s.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	if _, err := s.writer.WriteString("\n"); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush to ensure data is written
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush buffer: %w", err)
	}

	return nil
}

// Query reads audit records from the file matching the filter
// Note: File storage query is simple and may be slow for large files
// For production use, consider using database storage or log aggregation tools
func (s *FileStorage) Query(ctx context.Context, filter *QueryFilter) ([]*types.AuditRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Close current writer and reopen file for reading
	if err := s.writer.Flush(); err != nil {
		logrus.Warnf("Failed to flush writer before query: %v", err)
	}

	file, err := os.Open(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for reading: %w", err)
	}
	defer func() { _ = file.Close() }()

	var results []*types.AuditRecord
	scanner := bufio.NewScanner(file)
	offset := 0
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}

	// Read file line by line (from end to start for newest first)
	// For simplicity, we read all lines and filter in memory
	// For large files, consider using a more efficient approach
	var allRecords []*types.AuditRecord
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record types.AuditRecord
		if err := json.Unmarshal(line, &record); err != nil {
			logrus.Warnf("Failed to unmarshal audit record: %v", err)
			continue
		}

		allRecords = append(allRecords, &record)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Filter records (reverse order for newest first)
	for i := len(allRecords) - 1; i >= 0; i-- {
		record := allRecords[i]

		// Apply filters
		if filter.EventType != "" && string(record.EventType) != filter.EventType {
			continue
		}
		if filter.UserID != "" && record.UserID != filter.UserID {
			continue
		}
		if filter.ChallengeID != "" && record.ChallengeID != filter.ChallengeID {
			continue
		}
		if filter.Channel != "" && record.Channel != filter.Channel {
			continue
		}
		if filter.Result != "" && record.Result != filter.Result {
			continue
		}
		if filter.StartTime > 0 && record.Timestamp < filter.StartTime {
			continue
		}
		if filter.EndTime > 0 && record.Timestamp > filter.EndTime {
			continue
		}

		// Apply offset
		if offset < filter.Offset {
			offset++
			continue
		}

		results = append(results, record)
		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// Close closes the file and releases resources
func (s *FileStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			logrus.Warnf("Failed to flush writer on close: %v", err)
		}
	}

	if s.file != nil {
		return s.file.Close()
	}

	return nil
}

// Rotate rotates the audit log file
// Creates a new file with timestamp suffix and reopens the main file
func (s *FileStorage) Rotate() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Flush current writer
	if err := s.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush writer: %w", err)
	}

	// Close current file
	if err := s.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	// Rename current file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := fmt.Sprintf("%s.%s", s.filePath, timestamp)
	if err := os.Rename(s.filePath, rotatedPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	// Open new file
	file, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open new file: %w", err)
	}

	s.file = file
	s.writer = bufio.NewWriter(file)

	logrus.Infof("Audit log file rotated: %s -> %s", s.filePath, rotatedPath)
	return nil
}
