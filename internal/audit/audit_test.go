package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_audit.log")

	logger, err := NewLogger(true, logPath, true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		_ = logger.Close()
	}()

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}

	if !logger.enabled {
		t.Error("NewLogger() enabled should be true")
	}

	if !logger.maskEnabled {
		t.Error("NewLogger() maskEnabled should be true")
	}
}

func TestLogger_Log(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_audit.log")

	logger, err := NewLogger(true, logPath, true)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		_ = logger.Close()
	}()

	event := Event{
		Event:       "test_event",
		ChallengeID: "ch_123",
		UserID:      "user_123",
		Channel:     "email",
		Destination: "test@example.com",
		Result:      "ok",
		ClientIP:    "127.0.0.1",
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}

	logger.Log(event)

	// Read file and verify
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("No line written to log file")
	}

	line := scanner.Text()
	var loggedEvent Event
	if err := json.Unmarshal([]byte(line), &loggedEvent); err != nil {
		t.Fatalf("Failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.Event != "test_event" {
		t.Errorf("Logged event Event = %v, want 'test_event'", loggedEvent.Event)
	}

	if loggedEvent.Destination != "" {
		t.Error("Logged event Destination should be empty when masking is enabled")
	}

	if loggedEvent.DestinationMasked == "" {
		t.Error("Logged event DestinationMasked should be set when masking is enabled")
	}

	if !strings.Contains(loggedEvent.DestinationMasked, "@") {
		t.Errorf("Logged event DestinationMasked = %v, should contain masked email", loggedEvent.DestinationMasked)
	}
}

func TestLogger_Log_Disabled(t *testing.T) {
	logger, err := NewLogger(false, "", false)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	if logger.enabled {
		t.Error("NewLogger() enabled should be false when disabled")
	}

	// Logging should not fail when disabled
	event := Event{
		Event: "test_event",
	}
	logger.Log(event)
}

func TestMaskDestination(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "email masking",
			input:    "test@example.com",
			expected: "t***@example.com",
		},
		{
			name:     "phone masking",
			input:    "+8613800138000",
			expected: "+86***8000",
		},
		{
			name:     "short email",
			input:    "a@b.com",
			expected: "***@b.com",
		},
		{
			name:     "short phone",
			input:    "123",
			expected: "***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskDestination(tt.input)
			if !strings.Contains(result, "***") {
				t.Errorf("maskDestination() = %v, should contain '***'", result)
			}
		})
	}
}

func TestLogger_Log_WithoutMasking(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test_audit_no_mask.log")

	logger, err := NewLogger(true, logPath, false)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer func() {
		_ = logger.Close()
	}()

	event := Event{
		Event:       "test_event",
		Destination: "test@example.com",
	}

	logger.Log(event)

	// Read file and verify
	file, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("Failed to open log file: %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		t.Fatal("No line written to log file")
	}

	line := scanner.Text()
	var loggedEvent Event
	if err := json.Unmarshal([]byte(line), &loggedEvent); err != nil {
		t.Fatalf("Failed to unmarshal logged event: %v", err)
	}

	if loggedEvent.Destination != "test@example.com" {
		t.Errorf("Logged event Destination = %v, want 'test@example.com'", loggedEvent.Destination)
	}

	if loggedEvent.DestinationMasked != "" {
		t.Error("Logged event DestinationMasked should be empty when masking is disabled")
	}
}

func TestEvent_Fields(t *testing.T) {
	event := Event{
		Event:       "test_event",
		ChallengeID: "ch_123",
		UserID:      "user_123",
		Channel:     "email",
		Destination: "test@example.com",
		Provider:    "smtp",
		Result:      "ok",
		Reason:      "",
		ClientIP:    "127.0.0.1",
		Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		Tracestate:  "rojo=00f067aa0ba902b7",
		CreatedAt:   time.Now(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	var unmarshaled Event
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	if unmarshaled.Event != event.Event {
		t.Errorf("Unmarshaled Event = %v, want %v", unmarshaled.Event, event.Event)
	}
}
