package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/soulteary/herald/internal/config"
)

func TestNewStorageFromConfig(t *testing.T) {
	// Save original config values
	originalStorageType := config.AuditStorageType
	originalDatabaseURL := config.AuditDatabaseURL
	originalFilePath := config.AuditFilePath

	defer func() {
		config.AuditStorageType = originalStorageType
		config.AuditDatabaseURL = originalDatabaseURL
		config.AuditFilePath = originalFilePath
	}()

	t.Run("database type", func(t *testing.T) {
		config.AuditStorageType = "database"
		config.AuditDatabaseURL = "postgres://user:password@localhost:5432/herald?sslmode=disable"

		storage, err := NewStorageFromConfig()

		if err != nil {
			// Expected if database is not available
			t.Logf("NewStorageFromConfig() with database type error = %v (expected if DB not available)", err)
		} else {
			if storage == nil {
				t.Error("NewStorageFromConfig() with database type should return storage")
			} else {
				_ = storage.Close()
			}
		}
	})

	t.Run("file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		config.AuditStorageType = "file"
		config.AuditFilePath = filePath

		storage, err := NewStorageFromConfig()
		if err != nil {
			t.Fatalf("NewStorageFromConfig() with file type error = %v", err)
		}

		if storage == nil {
			t.Fatal("NewStorageFromConfig() with file type returned nil")
		}
		defer func() { _ = storage.Close() }()

		// Verify file was created
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("NewStorageFromConfig() should create file")
		}
	})

	t.Run("no storage type returns nil", func(t *testing.T) {
		config.AuditStorageType = ""
		config.AuditDatabaseURL = ""
		config.AuditFilePath = ""

		storage, err := NewStorageFromConfig()
		if err != nil {
			t.Errorf("NewStorageFromConfig() with no type error = %v, want nil", err)
		}
		if storage != nil {
			t.Error("NewStorageFromConfig() with no type should return nil")
		}
	})

	t.Run("invalid type returns error", func(t *testing.T) {
		config.AuditStorageType = "invalid_type"
		config.AuditDatabaseURL = ""
		config.AuditFilePath = ""

		storage, err := NewStorageFromConfig()
		if err == nil {
			t.Error("NewStorageFromConfig() with invalid type should return error")
		}
		if storage != nil {
			t.Error("NewStorageFromConfig() with invalid type should return nil storage")
			_ = storage.Close()
		}
		if err != nil && err.Error() == "" {
			t.Error("NewStorageFromConfig() error message should not be empty")
		}
	})

	t.Run("multiple types uses first", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		config.AuditStorageType = "file,database"
		config.AuditFilePath = filePath
		config.AuditDatabaseURL = "postgres://user:password@localhost:5432/herald?sslmode=disable"

		storage, err := NewStorageFromConfig()
		if err != nil {
			t.Fatalf("NewStorageFromConfig() with multiple types error = %v", err)
		}

		if storage == nil {
			t.Fatal("NewStorageFromConfig() with multiple types returned nil")
		}
		defer func() { _ = storage.Close() }()

		// Should use file type (first in list)
		// Verify file was created
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("NewStorageFromConfig() should use first type (file)")
		}
	})

	t.Run("database type without URL returns error", func(t *testing.T) {
		config.AuditStorageType = "database"
		config.AuditDatabaseURL = ""
		config.AuditFilePath = ""

		storage, err := NewStorageFromConfig()
		if err == nil {
			t.Error("NewStorageFromConfig() with database type but no URL should return error")
		}
		if storage != nil {
			t.Error("NewStorageFromConfig() with database type but no URL should return nil storage")
			_ = storage.Close()
		}
		if err != nil && err.Error() == "" {
			t.Error("NewStorageFromConfig() error message should not be empty")
		}
	})

	t.Run("file type without path returns error", func(t *testing.T) {
		config.AuditStorageType = "file"
		config.AuditFilePath = ""
		config.AuditDatabaseURL = ""

		storage, err := NewStorageFromConfig()
		if err == nil {
			t.Error("NewStorageFromConfig() with file type but no path should return error")
		}
		if storage != nil {
			t.Error("NewStorageFromConfig() with file type but no path should return nil storage")
			_ = storage.Close()
		}
		if err != nil && err.Error() == "" {
			t.Error("NewStorageFromConfig() error message should not be empty")
		}
	})

	t.Run("loki type not implemented", func(t *testing.T) {
		config.AuditStorageType = "loki"
		config.AuditLokiURL = "http://localhost:3100"

		storage, err := NewStorageFromConfig()
		if err == nil {
			t.Error("NewStorageFromConfig() with loki type should return error (not implemented)")
		}
		if storage != nil {
			t.Error("NewStorageFromConfig() with loki type should return nil storage")
			_ = storage.Close()
		}
		if err != nil && err.Error() == "" {
			t.Error("NewStorageFromConfig() error message should not be empty")
		}
	})

	t.Run("db alias for database", func(t *testing.T) {
		config.AuditStorageType = "db"
		config.AuditDatabaseURL = "postgres://user:password@localhost:5432/herald?sslmode=disable"

		storage, err := NewStorageFromConfig()

		if err != nil {
			// Expected if database is not available
			t.Logf("NewStorageFromConfig() with db alias error = %v (expected if DB not available)", err)
		} else {
			if storage == nil {
				t.Error("NewStorageFromConfig() with db alias should return storage")
			} else {
				_ = storage.Close()
			}
		}
	})

	t.Run("case insensitive type", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		config.AuditStorageType = "FILE"
		config.AuditFilePath = filePath

		storage, err := NewStorageFromConfig()
		if err != nil {
			t.Fatalf("NewStorageFromConfig() with uppercase type error = %v", err)
		}

		if storage == nil {
			t.Fatal("NewStorageFromConfig() with uppercase type returned nil")
		}
		defer func() { _ = storage.Close() }()

		// Verify file was created
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("NewStorageFromConfig() should handle uppercase type")
		}
	})

	t.Run("trimmed whitespace", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "audit.log")

		config.AuditStorageType = " file "
		config.AuditFilePath = filePath

		storage, err := NewStorageFromConfig()
		if err != nil {
			t.Fatalf("NewStorageFromConfig() with whitespace error = %v", err)
		}

		if storage == nil {
			t.Fatal("NewStorageFromConfig() with whitespace returned nil")
		}
		defer func() { _ = storage.Close() }()

		// Verify file was created
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("NewStorageFromConfig() should trim whitespace")
		}
	})
}
