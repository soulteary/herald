package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Event represents an audit log event
type Event struct {
	Event             string    `json:"event"`
	ChallengeID       string    `json:"challenge_id,omitempty"`
	UserID            string    `json:"user_id,omitempty"`
	Channel           string    `json:"channel,omitempty"`
	Destination       string    `json:"destination,omitempty"`
	DestinationMasked string    `json:"destination_masked,omitempty"`
	Provider          string    `json:"provider,omitempty"`
	Result            string    `json:"result,omitempty"`
	Reason            string    `json:"reason,omitempty"`
	ClientIP          string    `json:"client_ip,omitempty"`
	Traceparent       string    `json:"traceparent,omitempty"`
	Tracestate        string    `json:"tracestate,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// Logger handles audit log writing
type Logger struct {
	enabled     bool
	maskEnabled bool
	file        *os.File
	writer      *bufio.Writer
	mu          sync.Mutex
}

// NewLogger creates a new audit logger
func NewLogger(enabled bool, logPath string, maskEnabled bool) (*Logger, error) {
	if !enabled {
		return &Logger{enabled: false}, nil
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit log file: %w", err)
	}

	return &Logger{
		enabled:     true,
		maskEnabled: maskEnabled,
		file:        file,
		writer:      bufio.NewWriter(file),
	}, nil
}

// Log writes an audit event
func (l *Logger) Log(event Event) {
	if !l.enabled {
		return
	}

	// Mask destination if enabled
	if l.maskEnabled && event.Destination != "" {
		event.DestinationMasked = maskDestination(event.Destination)
		event.Destination = "" // Clear original
	}

	event.CreatedAt = time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		// Log error but don't fail
		return
	}

	if _, err := l.writer.Write(data); err != nil {
		// Log error but don't fail
		return
	}
	if _, err := l.writer.WriteString("\n"); err != nil {
		// Log error but don't fail
		return
	}
	if err := l.writer.Flush(); err != nil {
		// Log error but don't fail
		return
	}
}

// Close closes the audit logger
func (l *Logger) Close() error {
	if !l.enabled || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.writer != nil {
		_ = l.writer.Flush() // Best effort flush on close
	}
	return l.file.Close()
}

// maskDestination masks email/phone for privacy
func maskDestination(dest string) string {
	if strings.Contains(dest, "@") {
		// Email: mask@example.com -> m***@example.com
		parts := strings.Split(dest, "@")
		if len(parts) == 2 {
			local := parts[0]
			if len(local) > 1 {
				return string(local[0]) + "***@" + parts[1]
			}
			return "***@" + parts[1]
		}
		return "***"
	}

	// Phone: +8613800138000 -> +8613***8000
	if len(dest) > 4 {
		return dest[:3] + "***" + dest[len(dest)-4:]
	}
	return "***"
}
