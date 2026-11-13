package config

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	logger "github.com/soulteary/logger-kit/v3"
)

func advisoryFor(check string) string {
	for _, a := range startupAdvisories() {
		if a.check == check {
			return a.message
		}
	}
	return ""
}

func TestIPLimitAdvisoryDependsOnTrustedProxyConfiguration(t *testing.T) {
	oldHeader, oldProxies, oldLimit := TrustedProxyHeader, TrustedProxies, RateLimitPerIP
	t.Cleanup(func() { TrustedProxyHeader, TrustedProxies, RateLimitPerIP = oldHeader, oldProxies, oldLimit })
	RateLimitPerIP = 7

	TrustedProxyHeader, TrustedProxies = "", nil
	message := advisoryFor("config.rate_limit_ip")
	for _, want := range []string{"RATE_LIMIT_PER_IP=7", "direct peer", "HERALD_TRUSTED_PROXY_HEADER", "HERALD_TRUSTED_PROXIES"} {
		if !strings.Contains(message, want) {
			t.Errorf("advisory %q does not contain %q", message, want)
		}
	}

	TrustedProxyHeader, TrustedProxies = "X-Forwarded-For", []string{"10.0.0.0/8"}
	if message := advisoryFor("config.rate_limit_ip"); message != "" {
		t.Errorf("unexpected advisory with trusted proxies configured: %q", message)
	}
}

// TestInitializeLogsIPLimitAdvisory checks the advisory reaches the startup
// log exactly once, as a warning.
func TestInitializeLogsIPLimitAdvisory(t *testing.T) {
	oldHeader, oldProxies := TrustedProxyHeader, TrustedProxies
	t.Cleanup(func() { TrustedProxyHeader, TrustedProxies = oldHeader, oldProxies })
	TrustedProxyHeader, TrustedProxies = "", nil
	strong := strings.Repeat("k", 32)

	out, _, _ := initializeDevelopment(t, strong, "", strong)
	if got := strings.Count(out, "counts the direct peer"); got != 1 {
		t.Fatalf("want exactly one per-IP limit warning, got %d: %s", got, out)
	}
	if !strings.Contains(out, `"check":"config.rate_limit_ip"`) || !strings.Contains(out, `"level":"warn"`) {
		t.Fatalf("warning is missing its check name or level: %s", out)
	}

	TrustedProxyHeader, TrustedProxies = "X-Forwarded-For", []string{"10.0.0.0/8"}
	out, _, _ = initializeDevelopment(t, strong, "", strong)
	if strings.Contains(out, "counts the direct peer") {
		t.Fatalf("warning logged although trusted proxies are configured: %s", out)
	}
}

func TestPlaintextListenerAdvisory(t *testing.T) {
	defer withProdEnv(t)() // production, api_key mode, no TLS
	oldSecret := HMACSecret
	t.Cleanup(func() { HMACSecret = oldSecret })

	message := advisoryFor("config.listener_tls")
	for _, want := range []string{"REQUEST_AUTH_MODE=api_key", "API key", "clear text", "TLS_CERT_FILE"} {
		if !strings.Contains(message, want) {
			t.Errorf("api_key advisory %q does not contain %q", message, want)
		}
	}

	RequestAuthMode, HMACSecret = "hmac_v2", strings.Repeat("h", 32)
	message = advisoryFor("config.listener_tls")
	if !strings.Contains(message, "verification codes") || strings.Contains(message, "API key") {
		t.Errorf("unexpected hmac advisory: %q", message)
	}

	TLSCertFile, TLSKeyFile = "/etc/herald/tls.crt", "/etc/herald/tls.key"
	if message := advisoryFor("config.listener_tls"); message != "" {
		t.Errorf("advisory with TLS configured: %q", message)
	}

	TLSCertFile, TLSKeyFile = "", ""
	Env = EnvDevelopment
	if message := advisoryFor("config.listener_tls"); message != "" {
		t.Errorf("advisory outside production: %q", message)
	}
}

// TestProductionStartupWarnsAboutPlaintextAPIKeyListener checks that a valid
// production API-key configuration without TLS still starts, and says so.
func TestProductionStartupWarnsAboutPlaintextAPIKeyListener(t *testing.T) {
	defer withProdEnv(t)()
	oldMap := hmacKeysMap
	defer func() { hmacKeysMap = oldMap; hmacKeysMapOnce = sync.Once{} }()
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}

	var logs bytes.Buffer
	if err := Initialize(logger.New(logger.Config{Level: logger.WarnLevel, Format: logger.FormatJSON, Output: &logs})); err != nil {
		t.Fatalf("a plaintext listener must stay a warning, got error: %v", err)
	}
	out := logs.String()
	if got := strings.Count(out, `"check":"config.listener_tls"`); got != 1 {
		t.Fatalf("want exactly one listener TLS warning, got %d: %s", got, out)
	}
	if !strings.Contains(out, "REQUEST_AUTH_MODE=api_key") {
		t.Fatalf("warning does not name the auth mode: %s", out)
	}
}

func TestShortCodeAdvisory(t *testing.T) {
	oldLength := CodeLength
	t.Cleanup(func() { CodeLength = oldLength })

	for length, fragments := range map[int][]string{
		4: {"CODE_LENGTH=4", "10000 possible codes", "100 times"},
		5: {"CODE_LENGTH=5", "100000 possible codes", "10 times"},
	} {
		CodeLength = length
		message := advisoryFor("config.code_length")
		for _, want := range fragments {
			if !strings.Contains(message, want) {
				t.Errorf("CODE_LENGTH=%d advisory %q does not contain %q", length, message, want)
			}
		}
	}
	for _, length := range []int{6, 8, 10} {
		CodeLength = length
		if message := advisoryFor("config.code_length"); message != "" {
			t.Errorf("unexpected advisory for CODE_LENGTH=%d: %q", length, message)
		}
	}
}

// TestInitializeWarnsAboutShortCodes checks a short code length is accepted
// and reported once at startup.
func TestInitializeWarnsAboutShortCodes(t *testing.T) {
	oldLength := CodeLength
	t.Cleanup(func() { CodeLength = oldLength })
	strong := strings.Repeat("k", 32)

	CodeLength = 4
	out, _, _ := initializeDevelopment(t, strong, "", strong)
	if got := strings.Count(out, `"check":"config.code_length"`); got != 1 {
		t.Fatalf("want exactly one short-code warning, got %d: %s", got, out)
	}
	CodeLength = 6
	out, _, _ = initializeDevelopment(t, strong, "", strong)
	if strings.Contains(out, "config.code_length") {
		t.Fatalf("warning logged for the default code length: %s", out)
	}
}
