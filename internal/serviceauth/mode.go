package serviceauth

import (
	"fmt"
	"strings"
)

// Mode selects the request authentication scheme.
type Mode string

const (
	ModeHMACV2 Mode = "hmac_v2"
	ModeAPIKey Mode = "api_key"
	ModeNone   Mode = "none"
)

// ParseMode rejects unrecognized modes and normalizes supported aliases.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "hmac_v2", "hmac-v2", "hmac":
		return ModeHMACV2, nil
	case "api_key", "apikey":
		return ModeAPIKey, nil
	case "none", "off", "disabled":
		return ModeNone, nil
	default:
		return "", fmt.Errorf("REQUEST_AUTH_MODE must be hmac_v2, api_key, or none")
	}
}
