package router

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/config"
	"github.com/soulteary/herald/internal/testutil"
)

// sourceTreeSpool is the default spool location, "./audit-spool", which for a
// test binary is inside the package directory.
const sourceTreeSpool = "audit-spool"

// sourceTreeSpoolBefore is the state of that directory before any test ran.
var sourceTreeSpoolBefore string

// sourceTreeSpoolState describes ./audit-spool: "absent", or its modification
// time and entry names. Opening a spool there creates and removes a probe
// file, and every record adds and removes one, so any use changes the state.
func sourceTreeSpoolState() string {
	info, err := os.Stat(sourceTreeSpool)
	if os.IsNotExist(err) {
		return "absent"
	}
	if err != nil {
		return "stat error: " + err.Error()
	}
	entries, err := os.ReadDir(sourceTreeSpool)
	if err != nil {
		return "read error: " + err.Error()
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return fmt.Sprintf("modified %d, entries [%s]", info.ModTime().UnixNano(), strings.Join(names, " "))
}

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
	sourceTreeSpoolBefore = sourceTreeSpoolState()
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestRouterTestsDoNotSpoolIntoTheSourceTree builds a router with audit
// logging enabled, which opens the spool, and checks that the package
// directory's ./audit-spool was neither created nor written to. A checkout
// that ran these tests before the spool was moved still holds that directory;
// it is compared with its state at the start of the run instead of being
// reported as created by this run.
func TestRouterTestsDoNotSpoolIntoTheSourceTree(t *testing.T) {
	if !config.AuditEnabled {
		t.Skip("audit logging is disabled")
	}
	redisClient, _ := testutil.NewTestRedisClient()
	defer func() { _ = redisClient.Close() }()
	if app := NewRouterWithClient(redisClient, testLogger()); app == nil {
		t.Fatal("NewRouterWithClient() returned nil")
	}
	if after := sourceTreeSpoolState(); after != sourceTreeSpoolBefore {
		t.Fatalf("the router tests wrote to ./audit-spool in the package directory:\n before: %s\n after:  %s", sourceTreeSpoolBefore, after)
	}
	if sourceTreeSpoolBefore != "absent" {
		t.Log("./audit-spool in the package directory is left over from an earlier test run and was not touched; it can be deleted")
	}
	if entries, err := os.ReadDir(config.AuditSpoolDir); err != nil || len(entries) == 0 {
		t.Fatalf("the temporary spool %q was not used: %v", config.AuditSpoolDir, err)
	}
}
