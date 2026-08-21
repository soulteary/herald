// Package security provides small privacy-preserving primitives shared across
// Herald: peppered digests for PII that must be used as a stable key/label
// (rate-limit keys, audit fingerprints) without landing raw email/phone values
// in Redis or logs.
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Digester computes stable, non-reversible digests of PII using an HMAC keyed
// by a server-side pepper. The same input always maps to the same digest (so it
// is usable as a rate-limit / dedup key) but the raw value cannot be recovered
// from Redis or exported metrics.
type Digester struct {
	pepper []byte
}

// NewDigester creates a Digester. An empty pepper is allowed (it degrades to a
// plain, unkeyed SHA-256) but callers SHOULD supply a non-empty pepper in
// production; config.Validate enforces this at startup.
func NewDigester(pepper []byte) *Digester {
	return &Digester{pepper: pepper}
}

// Digest returns a hex-encoded, peppered digest of the given value. Empty input
// returns an empty string so callers can distinguish "no value" from a hash of
// the empty string.
func (d *Digester) Digest(value string) string {
	if value == "" {
		return ""
	}
	mac := hmac.New(sha256.New, d.pepper)
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// DigestParts returns a peppered digest over multiple parts with an unambiguous
// separator so ("a","bc") and ("ab","c") do not collide.
func (d *Digester) DigestParts(parts ...string) string {
	mac := hmac.New(sha256.New, d.pepper)
	for _, p := range parts {
		_, _ = mac.Write([]byte(p))
		_, _ = mac.Write([]byte{0})
	}
	return hex.EncodeToString(mac.Sum(nil))
}
