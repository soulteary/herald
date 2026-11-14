package config

import (
	"fmt"
	"strings"

	"github.com/soulteary/herald/internal/serviceauth"
)

// advisory is a startup warning about a setting that is valid but probably
// not what the operator intends. Advisories never stop startup.
type advisory struct{ check, message string }

// startupAdvisories returns the warnings Initialize logs for the current
// configuration.
func startupAdvisories() []advisory {
	var advisories []advisory
	if message := ipLimitScopeAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.rate_limit_ip", message})
	}
	if message := plaintextListenerAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.listener_tls", message})
	}
	if message := shortCodeAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.code_length", message})
	}
	if message := testModeAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.test_mode", message})
	}
	if message := totpBaseURLAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.totp", message})
	}
	if message := otlpEndpointAdvisory(); message != "" {
		advisories = append(advisories, advisory{"config.otlp", message})
	}
	return advisories
}

// testModeAdvisory warns when HERALD_TEST_MODE=true is set outside the test
// environment. Code exposure needs ENVIRONMENT=test as well, so the flag then
// changes nothing, and an operator who set it to see debug_code gets no hint
// why it is missing. Production is not covered here: Validate() refuses the
// flag there.
func testModeAdvisory() string {
	if !TestMode || Env == EnvTest || Env == EnvProduction {
		return ""
	}
	return "HERALD_TEST_MODE=true has no effect because ENVIRONMENT is not test: challenge responses carry no debug_code and the test-code endpoint is not started; set ENVIRONMENT=test as well to expose codes for testing, or remove HERALD_TEST_MODE"
}

// totpBaseURLAdvisory warns when the TOTP proxy is switched on without the
// address of the service it forwards to. The routes are registered either
// way and answer 503/totp_not_configured.
func totpBaseURLAdvisory() string {
	if !TOTPEnabled || strings.TrimSpace(TOTPBaseURL) != "" {
		return ""
	}
	return "HERALD_TOTP_ENABLED=true but HERALD_TOTP_BASE_URL is empty: the TOTP proxy is not configured and every /v1/totp request answers 503/totp_not_configured; set HERALD_TOTP_BASE_URL to the herald-totp service"
}

// otlpEndpointAdvisory warns when tracing is switched on without a collector
// to export to. Spans are then created and discarded.
func otlpEndpointAdvisory() string {
	if !OTLPEnabled || strings.TrimSpace(OTLPEndpoint) != "" {
		return ""
	}
	return "OTLP_ENABLED=true but OTLP_ENDPOINT is empty: no trace exporter is configured and no spans are exported; set OTLP_ENDPOINT to the collector"
}

// plaintextListenerAdvisory warns when a production listener has no TLS.
// Verification requests carry the code in their body in every auth mode, and
// in API-key mode each request also carries the static key, which has no
// replay protection. This is a warning, not an error: TLS may be terminated
// by a sidecar or mesh proxy next to the process, which Herald cannot see.
func plaintextListenerAdvisory() string {
	if Env != EnvProduction || (TLSCertFile != "" && TLSKeyFile != "") {
		return ""
	}
	mode, err := serviceauth.ParseMode(RequestAuthMode)
	if err != nil {
		return ""
	}
	const tail = "set TLS_CERT_FILE and TLS_KEY_FILE, or make sure TLS is terminated in front of this listener on a trusted local hop"
	if mode == serviceauth.ModeAPIKey {
		return "TLS is not configured on the listener while REQUEST_AUTH_MODE=api_key: the API key and verification codes are sent in clear text, and a captured key can be replayed; " + tail
	}
	return "TLS is not configured on the listener: verification codes and destinations in request bodies are sent in clear text (request signatures protect integrity, not confidentiality); " + tail
}

// ipLimitScopeAdvisory describes what RATE_LIMIT_PER_IP counts when no
// trusted proxy is configured. The limit then applies to the direct peer, so
// a gateway calling Herald for many end users shares a single quota.
func ipLimitScopeAdvisory() string {
	if TrustedProxyHeader != "" && len(TrustedProxies) > 0 {
		return ""
	}
	return fmt.Sprintf("RATE_LIMIT_PER_IP=%d per minute counts the direct peer because HERALD_TRUSTED_PROXY_HEADER and HERALD_TRUSTED_PROXIES are not set: a gateway that calls Herald for many end users shares this one quota among all of them; configure both settings to limit per end user, or size RATE_LIMIT_PER_IP for the gateway's total traffic", RateLimitPerIP)
}

// recommendedCodeLength is the shortest code length that does not draw a
// startup warning. CODE_LENGTH accepts 4 and 5 as well.
const recommendedCodeLength = 6

// shortCodeAdvisory warns about codes shorter than six digits. Each digit
// removed makes a guess ten times more likely to succeed, while the attempt
// and rate limits stay the same.
func shortCodeAdvisory() string {
	if CodeLength >= recommendedCodeLength {
		return ""
	}
	space := 1
	for i := 0; i < CodeLength; i++ {
		space *= 10
	}
	return fmt.Sprintf("CODE_LENGTH=%d gives only %d possible codes, so each guess is %d times more likely to succeed than with the default of %d digits; use at least %d unless the attempt and rate limits are tightened to match", CodeLength, space, 1000000/space, recommendedCodeLength, recommendedCodeLength)
}
