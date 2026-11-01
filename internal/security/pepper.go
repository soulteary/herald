// Package security provides small privacy-preserving primitives shared across
// Herald: peppered digests for PII that must be used as a stable key/label
// (rate-limit keys, audit fingerprints) without landing raw email/phone values
// in Redis or logs.
package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sync"
)

// Digester computes stable, non-reversible digests of PII using an HMAC keyed
// by a server-side pepper. The same input always maps to the same digest (so it
// is usable as a rate-limit / dedup key) but the raw value cannot be recovered
// from Redis or exported metrics.
type Digester struct {
	pepper []byte
	pool   sync.Pool
}

type digestState struct {
	mac       hash.Hash
	sum       [sha256.Size]byte
	encoded   [sha256.Size * 2]byte
	separator [1]byte
}

func (d *Digester) acquire() *digestState {
	if cached := d.pool.Get(); cached != nil {
		state := cached.(*digestState)
		state.mac.Reset()
		return state
	}
	return &digestState{mac: hmac.New(sha256.New, d.pepper)}
}

// NewDigester creates a Digester and copies its pepper. A Digester must not
// be copied after first use. An empty pepper is allowed (it uses HMAC-SHA256 with
// an empty key) but callers SHOULD supply a non-empty pepper in
// production; config.Validate enforces this at startup.
func NewDigester(pepper []byte) *Digester {
	return &Digester{pepper: append([]byte(nil), pepper...)}
}

// Digest returns a hex-encoded, peppered digest of the given value. Empty input
// returns an empty string so callers can distinguish "no value" from a hash of
// the empty string.
func (d *Digester) Digest(value string) string {
	if value == "" {
		return ""
	}
	state := d.acquire()
	mac := state.mac
	_, _ = mac.Write([]byte(value))
	hex.Encode(state.encoded[:], mac.Sum(state.sum[:0]))
	out := string(state.encoded[:])
	d.pool.Put(state)
	return out
}

// DigestParts returns a peppered digest over multiple parts with an unambiguous
// separator so ("a","bc") and ("ab","c") do not collide.
func (d *Digester) DigestParts(parts ...string) string {
	state := d.acquire()
	mac := state.mac
	for _, p := range parts {
		_, _ = mac.Write([]byte(p))
		_, _ = mac.Write(state.separator[:])
	}
	hex.Encode(state.encoded[:], mac.Sum(state.sum[:0]))
	out := string(state.encoded[:])
	d.pool.Put(state)
	return out
}
