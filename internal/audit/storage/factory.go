package storage

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/soulteary/herald/internal/config"
)

// NewStorageFromConfig creates a storage instance based on configuration
func NewStorageFromConfig() (Storage, error) {
	storageType := config.AuditStorageType
	if storageType == "" {
		// No persistent storage configured
		return nil, nil
	}

	// Support comma-separated list of storage types
	// For now, we only support one storage type at a time
	types := strings.Split(storageType, ",")
	if len(types) > 1 {
		logrus.Warnf("Multiple storage types specified, using first: %s", types[0])
	}
	storageType = strings.TrimSpace(types[0])

	switch strings.ToLower(storageType) {
	case "database", "db":
		if config.AuditDatabaseURL == "" {
			return nil, fmt.Errorf("AUDIT_DATABASE_URL is required for database storage")
		}
		return NewDatabaseStorage(config.AuditDatabaseURL)

	case "file":
		if config.AuditFilePath == "" {
			return nil, fmt.Errorf("AUDIT_FILE_PATH is required for file storage")
		}
		return NewFileStorage(config.AuditFilePath)

	case "loki":
		// Loki storage not implemented yet
		return nil, fmt.Errorf("loki storage not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
