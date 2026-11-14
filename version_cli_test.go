package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type versionFailWriter struct{ err error }

func (w versionFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestVersionWriterReturnsOriginalError(t *testing.T) {
	failure := errors.New("synthetic version output failure")
	if err := writeVersion(versionFailWriter{failure}); !errors.Is(err, failure) {
		t.Fatalf("writeVersion error = %v, want original writer failure", err)
	}
}

// versionFixtureBuildTimeout and versionFixtureRunTimeout only stop a build or
// a child process that hangs; the assertions are on exit status and output.
// The first start of a freshly linked executable can take several seconds on
// a loaded machine (on macOS the system checks a new binary before it runs),
// which the earlier 5-second limit did not allow for.
const (
	versionFixtureBuildTimeout = 5 * time.Minute
	versionFixtureRunTimeout   = time.Minute
)

func TestVersionCLIActualMain(t *testing.T) {
	// Build a NEW host executable with synthetic linker values. This never runs
	// a downloaded release asset or a normal service. Package initialization can
	// read this controlled environment before main; no no-env-read claim is made.
	binary := filepath.Join(t.TempDir(), "herald-version-fixture")
	buildEnv := make([]string, 0, 16)
	for _, key := range []string{"PATH", "HOME", "TMPDIR", "GOROOT", "GOPATH", "GOMODCACHE", "GOCACHE"} {
		if value, found := os.LookupEnv(key); found {
			buildEnv = append(buildEnv, key+"="+value)
		}
	}
	buildEnv = append(buildEnv, "GOENV=off", "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	const injectedVersion = "9.9.9-version-probe-fixture"
	const injectedCommit = "1111111111111111111111111111111111111111"
	const injectedDate = "2026-11-09T00:00:00Z"
	const prefix = "github.com/soulteary/version-kit/v4."
	flags := "-s -w -X " + prefix + "Version=" + injectedVersion + " -X " + prefix + "Commit=" + injectedCommit + " -X " + prefix + "BuildDate=" + injectedDate
	ctx, cancel := context.WithTimeout(context.Background(), versionFixtureBuildTimeout)
	defer cancel()
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("locate existing Go executable: %v", err)
	}
	build := exec.CommandContext(ctx, goBinary, "build", "-mod=readonly", "-trimpath", "-ldflags", flags, "-o", binary, "main.go")
	build.Env = buildEnv
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build synthetic version executable: %v\n%s", err, output)
	}

	// A regression into run() would fail configuration before Redis/provider
	// setup, and produce a startup banner instead of the exact JSON below.
	// No ambient credentials, provider endpoints or deployment files are passed.
	childEnv := []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent", "ENVIRONMENT=version-probe-invalid-fixture", "REDIS_ADDR=invalid-redis-address", "VERSION=must-not-override-linker", "COMMIT=must-not-override-linker", "BUILD_DATE=must-not-override-linker"}
	expected := "{\"version\":\"" + injectedVersion + "\",\"commit\":\"" + injectedCommit + "\",\"build_date\":\"" + injectedDate + "\"}\n"
	for _, args := range [][]string{{"-version"}, {"-version=true"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), versionFixtureRunTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Env = childEnv
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("version-only main: %v, stderr %q", err, stderr.String())
			}
			if stdout.String() != expected || stderr.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q, want only exact injected version JSON", stdout.String(), stderr.String())
			}
		})
	}
	for _, args := range [][]string{{"-version", "-healthcheck"}, {"-version", "unexpected-position"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), versionFixtureRunTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Env = childEnv
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			var exit *exec.ExitError
			if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 2 {
				t.Fatalf("conflict exit = %v, want 2", err)
			}
			if stdout.Len() != 0 || stderr.String() != "-version cannot be combined with -healthcheck or positional arguments\n" {
				t.Fatalf("unexpected startup or conflict output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
	// The writer unit control preserves the exact error. Also exercise main's
	// actual write-failure branch with a read-only /dev/null stdout descriptor.
	t.Run("main-write-failure", func(t *testing.T) {
		output, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = output.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), versionFixtureRunTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "-version")
		cmd.Env, cmd.Stdout = childEnv, output
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		var exit *exec.ExitError
		if err := cmd.Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("write-failure exit = %v, want 1", err)
		}
		if stderr.String() != "unable to write version JSON\n" {
			t.Fatalf("write-failure main produced unexpected output: %q", stderr.String())
		}
	})
}
