// Package destination provides a single canonical normalization and validation
// point for OTP destinations (email / SMS / DingTalk). Centralizing this
// prevents trivial variants of the same destination (case, spacing, separators)
// from bypassing rate limits or idempotency dedup, and rejects malformed input
// before it reaches a provider.
package destination

import (
	"errors"
	"net/mail"
	"strings"
)

// Channel constants mirror the request channel values.
const (
	ChannelEmail    = "email"
	ChannelSMS      = "sms"
	ChannelDingTalk = "dingtalk"
)

// Field length caps. These are deliberately conservative upper bounds to stop
// oversized inputs from reaching providers or Redis keys.
const (
	MaxEmailLen    = 254 // RFC 5321
	MaxPhoneDigits = 15  // E.164 maximum
	MinPhoneDigits = 8
	MaxDingTalkLen = 64
)

// ErrInvalidDestination is returned when a destination fails validation. The
// error intentionally does not embed the raw value so it is never logged.
var ErrInvalidDestination = errors.New("invalid destination")

// Normalize applies the single canonical normalization for a channel. It is
// pure (no validation side effects) so it can be used for fingerprints and
// rate-limit keys consistently. Validate should be called to reject malformed
// values.
func Normalize(channel, dest string) string {
	d := strings.TrimSpace(dest)
	switch strings.ToLower(channel) {
	case ChannelEmail:
		// Email: lowercase whole address. Domain is case-insensitive; local
		// part is treated case-insensitively by virtually all providers, and
		// treating it so closes a dedup-bypass gap.
		return strings.ToLower(d)
	case ChannelSMS:
		// SMS: keep a leading '+', strip all other non-digits so
		// "+1 (555) 123-4567" and "+15551234567" collapse to one key.
		var b strings.Builder
		for i, r := range d {
			switch {
			case r >= '0' && r <= '9':
				b.WriteRune(r)
			case r == '+' && i == 0:
				b.WriteRune(r)
			}
		}
		return b.String()
	default:
		// DingTalk / other: strip surrounding space, lowercase.
		return strings.ToLower(d)
	}
}

// Validate normalizes and validates a destination for the given channel,
// returning the normalized value on success or ErrInvalidDestination.
func Validate(channel, dest string) (string, error) {
	n := Normalize(channel, dest)
	if n == "" {
		return "", ErrInvalidDestination
	}
	switch strings.ToLower(channel) {
	case ChannelEmail:
		if len(n) > MaxEmailLen {
			return "", ErrInvalidDestination
		}
		addr, err := mail.ParseAddress(n)
		if err != nil {
			return "", ErrInvalidDestination
		}
		// Reject display-name forms ("Foo <a@b.com>"); require bare address.
		if addr.Address != n {
			return "", ErrInvalidDestination
		}
		at := strings.LastIndexByte(n, '@')
		if at <= 0 || at == len(n)-1 {
			return "", ErrInvalidDestination
		}
		if !strings.Contains(n[at+1:], ".") {
			return "", ErrInvalidDestination
		}
		return n, nil
	case ChannelSMS:
		digits := strings.TrimPrefix(n, "+")
		if len(digits) < MinPhoneDigits || len(digits) > MaxPhoneDigits {
			return "", ErrInvalidDestination
		}
		for _, r := range digits {
			if r < '0' || r > '9' {
				return "", ErrInvalidDestination
			}
		}
		return n, nil
	case ChannelDingTalk:
		if len(n) == 0 || len(n) > MaxDingTalkLen {
			return "", ErrInvalidDestination
		}
		return n, nil
	default:
		return "", ErrInvalidDestination
	}
}
