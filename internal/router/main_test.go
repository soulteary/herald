package router

import (
	"fmt"
	"os"
	"testing"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// TestMain points the audit spool at a temporary directory. The default,
// "./audit-spool", is relative to the working directory, which for a test
// binary is the package directory: every run left pending audit records in
// the source tree, and the next run delivered them again.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "herald-router-audit-spool-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create temporary audit spool:", err)
		os.Exit(1)
	}
	config.AuditSpoolDir = dir
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestRouterTestsDoNotSpoolIntoTheSourceTree builds a router with audit
// logging enabled, which opens the spool, and checks that nothing was created
// in the package directory.
func TestRouterTestsDoNotSpoolIntoTheSourceTree(t *testing.T) {
	if !config.AuditEnabled {
		t.Skip("audit logging is disabled")
	}
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()
	if app := NewRouterWithClient(redisClient, testLogger()); app == nil {
		t.Fatal("NewRouterWithClient() returned nil")
	}
	if _, err := os.Stat("audit-spool"); !os.IsNotExist(err) {
		t.Fatalf("the router tests created ./audit-spool in the package directory (stat error: %v)", err)
	}
	if entries, err := os.ReadDir(config.AuditSpoolDir); err != nil || len(entries) == 0 {
		t.Fatalf("the temporary spool %q was not used: %v", config.AuditSpoolDir, err)
	}
}
