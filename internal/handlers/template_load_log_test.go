package handlers

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	logger "github.com/soulteary/logger-kit/v3"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

// TestStartupLogsTemplatesThatWereNotLoaded points TEMPLATE_DIR at a
// directory holding one template that does not parse. Startup succeeds, the
// built-in text remains in use, and the file is named in a warning.
func TestStartupLogsTemplatesThatWereNotLoaded(t *testing.T) {
	oldDir := config.TemplateDir
	t.Cleanup(func() { config.TemplateDir = oldDir })
	dir := t.TempDir()
	broken := filepath.Join(dir, "en", "sms")
	require.NoError(t, os.MkdirAll(broken, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(broken, "login.txt"), []byte("unterminated {{.Code"), 0o644))
	config.TemplateDir = dir

	rc := testRedisClient(t)
	t.Cleanup(func() { _ = rc.Close() })
	var logs bytes.Buffer
	_, err := NewHandlersWithError(rc, logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs}))
	require.NoError(t, err)

	// One JSON log line per problem; find the one for the broken file.
	var line map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var candidate map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &candidate), raw)
		if msg, _ := candidate["message"].(string); strings.Contains(msg, "template not loaded") {
			line = candidate
		}
	}
	require.NotNil(t, line, logs.String())
	require.Equal(t, "warn", line["level"])
	require.Equal(t, "templates", line["check"])
	require.Equal(t, dir, line["template_dir"])
	require.Contains(t, line["message"], filepath.Join("en", "sms", "login.txt"))
}
