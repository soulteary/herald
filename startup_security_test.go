package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestStartupRejectsInvalidSecurityEnums(t *testing.T) {
	if os.Getenv("HERALD_SECURITY_STARTUP_HELPER") == "1" {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	for _, tc := range []struct{ name, key, value string }{{"environment", "ENVIRONMENT", "productionn"}, {"auth mode", "REQUEST_AUTH_MODE", "api-keey"}} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStartupRejectsInvalidSecurityEnums$")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, tc.key+"=") && !strings.HasPrefix(entry, "HERALD_SECURITY_STARTUP_HELPER=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env, "HERALD_SECURITY_STARTUP_HELPER=1", tc.key+"="+tc.value)
			output, err := cmd.CombinedOutput()
			if err == nil || ctx.Err() != nil || !strings.Contains(string(output), tc.key) {
				t.Fatalf("error=%v timeout=%v output=%s", err, ctx.Err(), output)
			}
		})
	}
}
