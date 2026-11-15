package config

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	challengekit "github.com/soulteary/challenge-kit"
	logger "github.com/soulteary/logger-kit/v3"
)

// A 32-byte key and a 31-byte one, both recognisable in a log or an error.
const (
	testCodeHashKey      = "code-hash-key-0123456789abcdefgh"
	testShortCodeHashKey = "code-hash-key-0123456789abcdefg"
)

func withCodeHashSettings(t *testing.T) {
	t.Helper()
	oldKey, oldVerifyOnly := CodeHashKey, CodeHashVerifyOnly
	t.Cleanup(func() { CodeHashKey, CodeHashVerifyOnly = oldKey, oldVerifyOnly })
}

// The minimum is the one challenge-kit enforces, so a key Validate accepts is
// never refused when the challenge manager is built.
func TestCodeHashKeyMinimumIsTheLibraryMinimum(t *testing.T) {
	if CodeHashKeyMinBytes != challengekit.HMACCodeHasherMinKeyLen {
		t.Fatalf("CodeHashKeyMinBytes = %d, challenge-kit requires %d", CodeHashKeyMinBytes, challengekit.HMACCodeHasherMinKeyLen)
	}
	if len(testCodeHashKey) != CodeHashKeyMinBytes || len(testShortCodeHashKey) != CodeHashKeyMinBytes-1 {
		t.Fatalf("test keys are %d and %d bytes", len(testCodeHashKey), len(testShortCodeHashKey))
	}
	if _, err := challengekit.NewHMACCodeHasher([]byte(testCodeHashKey)); err != nil {
		t.Fatalf("challenge-kit refuses a key of the minimum length: %v", err)
	}
}

func TestCodeHashKeyLengthIsCheckedInEveryEnvironment(t *testing.T) {
	defer withProdEnv(t)()
	withCodeHashSettings(t)

	for _, environment := range []Environment{EnvDevelopment, EnvTest, EnvProduction} {
		Env = environment

		CodeHashKey = testShortCodeHashKey
		err := Validate()
		if err == nil {
			t.Fatalf("%s: a %d-byte HERALD_CODE_HASH_KEY must fail validation", environment, len(testShortCodeHashKey))
		}
		for _, want := range []string{"HERALD_CODE_HASH_KEY", "at least 32 bytes", "got 31"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: error %q does not contain %q", environment, err, want)
			}
		}
		if strings.Contains(err.Error(), testShortCodeHashKey) {
			t.Errorf("%s: error carries the key: %q", environment, err)
		}

		for name, key := range map[string]string{"unset": "", "minimum length": testCodeHashKey, "longer": testCodeHashKey + "more"} {
			CodeHashKey = key
			if err := Validate(); err != nil {
				t.Errorf("%s, %s key: unexpected error: %v", environment, name, err)
			}
		}
	}
}

func TestCodeHashMode(t *testing.T) {
	withCodeHashSettings(t)
	for _, tc := range []struct {
		key        string
		verifyOnly bool
		want       string
	}{
		{"", false, "argon2"},
		{"", true, "argon2"},
		{testCodeHashKey, false, "hmac-sha256"},
		{testCodeHashKey, true, "argon2 (hmac-sha256 verify-only)"},
	} {
		CodeHashKey, CodeHashVerifyOnly = tc.key, tc.verifyOnly
		if got := CodeHashMode(); got != tc.want {
			t.Errorf("key set=%t verifyOnly=%t: mode %q, want %q", tc.key != "", tc.verifyOnly, got, tc.want)
		}
	}
}

func TestCodeHashVerifyOnlyAdvisoryWithoutKey(t *testing.T) {
	withCodeHashSettings(t)

	CodeHashKey, CodeHashVerifyOnly = "", true
	message := advisoryFor("config.code_hash")
	for _, want := range []string{"HERALD_CODE_HASH_VERIFY_ONLY=true", "no effect", "HERALD_CODE_HASH_KEY is not set"} {
		if !strings.Contains(message, want) {
			t.Errorf("advisory %q does not contain %q", message, want)
		}
	}

	for name, set := range map[string]func(){
		"key set":  func() { CodeHashKey, CodeHashVerifyOnly = testCodeHashKey, true },
		"flag off": func() { CodeHashKey, CodeHashVerifyOnly = "", false },
	} {
		set()
		if message := advisoryFor("config.code_hash"); message != "" {
			t.Errorf("%s: unexpected advisory %q", name, message)
		}
	}
}

// Initialize reports which scheme is in use and never writes the key, at any
// log level, whether the key is accepted or refused.
func TestInitializeDoesNotLogTheCodeHashKey(t *testing.T) {
	defer withProdEnv(t)()
	withCodeHashSettings(t)
	oldMap, oldDefault := hmacKeysMap, hmacDefaultKeyID
	defer func() {
		hmacKeysMap, hmacDefaultKeyID = oldMap, oldDefault
		hmacKeysMapOnce = sync.Once{}
	}()
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	Env = EnvDevelopment

	initialize := func() (string, error) {
		var logs bytes.Buffer
		err := Initialize(logger.New(logger.Config{Level: logger.DebugLevel, Format: logger.FormatJSON, Output: &logs}))
		return logs.String(), err
	}

	CodeHashKey = testCodeHashKey
	out, err := initialize()
	if err != nil {
		t.Fatalf("Initialize with a valid key: %v", err)
	}
	if !strings.Contains(out, `"code_hash":"hmac-sha256"`) {
		t.Errorf("startup log does not report the keyed scheme: %s", out)
	}
	if strings.Contains(out, testCodeHashKey) {
		t.Errorf("startup log carries the key: %s", out)
	}

	CodeHashKey = ""
	out, err = initialize()
	if err != nil {
		t.Fatalf("Initialize without a key: %v", err)
	}
	if !strings.Contains(out, `"code_hash":"argon2"`) {
		t.Errorf("startup log does not report the default scheme: %s", out)
	}

	CodeHashKey = testShortCodeHashKey
	out, err = initialize()
	if err == nil || !strings.Contains(err.Error(), "HERALD_CODE_HASH_KEY") {
		t.Fatalf("Initialize with a short key: got %v, want an error naming HERALD_CODE_HASH_KEY", err)
	}
	if strings.Contains(out, testShortCodeHashKey) || strings.Contains(err.Error(), testShortCodeHashKey) {
		t.Errorf("a refused key is written out: log %s, error %v", out, err)
	}
}
