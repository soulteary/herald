package config

import (
	"strings"

	"github.com/soulteary/herald-totp/v2/pkg/heraldtotp"
)

func totpSigningIdentityProblems() []string {
	if !TOTPEnabled || TOTPHMACSecret == "" {
		return nil
	}
	var problems []string
	for _, field := range []struct{ name, value string }{
		{"HERALD_TOTP_HMAC_KEY_ID", TOTPHMACKeyID},
		{"HERALD_TOTP_HMAC_SERVICE", TOTPHMACService},
	} {
		if len(field.value) > heraldtotp.MaxAuthIdentifierBytes || strings.TrimSpace(field.value) != field.value || strings.ContainsFunc(field.value, func(r rune) bool { return r < 32 || r == 127 }) {
			problems = append(problems, field.name+" must be at most 128 bytes without control characters or surrounding whitespace")
		}
	}
	return problems
}
