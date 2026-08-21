// Package auth implements Herald's request authentication policy. It provides a
// replay-resistant HMAC v2 scheme that binds the signature to the full request
// (method, path, query, timestamp, nonce, service, key id, and a SHA-256 of the
// raw body) and consumes a single-use nonce via Redis SET NX EX.
//
// Design goals (see security hardening plan, Phase 4):
//   - v2 canonical string prevents cross-endpoint / cross-body replay.
//   - Nonce is verified BEFORE being written so a forged signature cannot burn a
//     victim's nonce, and the nonce is stored under a keyed digest.
//   - No silent downgrade: a present-but-invalid v2 signature is rejected, never
//     falling back to API key.
//   - No random default key id; the default must be configured explicitly.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// SignatureVersionV2 is the value of the X-Signature-Version header selecting
// the v2 scheme.
const SignatureVersionV2 = "v2"

// canonicalPrefix is the domain-separation prefix for the v2 canonical string.
const canonicalPrefix = "HERALD-HMAC-V2"

// Headers used by the v2 scheme.
const (
	HeaderSignature        = "X-Signature"
	HeaderSignatureVersion = "X-Signature-Version"
	HeaderTimestamp        = "X-Timestamp"
	HeaderNonce            = "X-Nonce"
	HeaderService          = "X-Service"
	HeaderKeyID            = "X-Key-Id"
)

// CanonicalRequest holds the fields bound into a v2 signature.
type CanonicalRequest struct {
	Method    string
	Path      string
	Query     string
	Timestamp string
	Nonce     string
	Service   string
	KeyID     string
	Body      []byte
}

// Canonical returns the canonical string that is signed for v2. The raw body is
// hashed (never included verbatim) so large bodies do not blow up the signed
// material and binary bodies are handled safely.
func (r CanonicalRequest) Canonical() string {
	bodyHash := sha256.Sum256(r.Body)
	var b strings.Builder
	b.WriteString(canonicalPrefix)
	b.WriteByte('\n')
	b.WriteString(strings.ToUpper(r.Method))
	b.WriteByte('\n')
	b.WriteString(r.Path)
	b.WriteByte('\n')
	b.WriteString(r.Query)
	b.WriteByte('\n')
	b.WriteString(r.Timestamp)
	b.WriteByte('\n')
	b.WriteString(r.Nonce)
	b.WriteByte('\n')
	b.WriteString(r.Service)
	b.WriteByte('\n')
	b.WriteString(r.KeyID)
	b.WriteByte('\n')
	b.WriteString(hex.EncodeToString(bodyHash[:]))
	return b.String()
}

// SignV2 computes the hex HMAC-SHA256 v2 signature for the canonical request
// using the provided secret.
func SignV2(secret string, r CanonicalRequest) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(r.Canonical()))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyV2 checks the provided signature against the expected v2 signature in
// constant time.
func VerifyV2(secret, providedSig string, r CanonicalRequest) bool {
	expected := SignV2(secret, r)
	return hmac.Equal([]byte(providedSig), []byte(expected))
}

// TimestampWithinDrift reports whether the given unix-seconds timestamp string
// is within +/- drift of now.
func TimestampWithinDrift(timestamp string, now time.Time, drift time.Duration) bool {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Unix() - ts
	if delta < 0 {
		delta = -delta
	}
	return time.Duration(delta)*time.Second <= drift
}
