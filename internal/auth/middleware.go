package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/metrics"
	"github.com/soulteary/herald/internal/serviceauth"
)

// Clock is injectable for deterministic tests.
type Clock func() time.Time

// KeyProvider returns the shared secret for a key id, or "" if unknown.
type KeyProvider func(keyID string) string

// Mode selects the request-body authentication scheme.
type Mode = serviceauth.Mode

const (
	ModeHMACV2 = serviceauth.ModeHMACV2
	ModeAPIKey = serviceauth.ModeAPIKey
	ModeNone   = serviceauth.ModeNone
)

// ParseMode maps supported aliases; unknown values return an empty mode.
func ParseMode(s string) Mode { mode, _ := ParseModeStrict(s); return mode }

// ParseModeStrict rejects unrecognized authentication modes.
func ParseModeStrict(s string) (Mode, error) { return serviceauth.ParseMode(s) }

// Config configures the Herald auth middleware.
type Config struct {
	// RequireV3 rejects older HMAC schemes on routes whose side effects depend on idempotency.
	RequireV3 bool

	// RequireKeyID rejects omitted wire key IDs in v2/v3 requests. Key maps
	// enable this so a changed default cannot reinterpret a signed request.
	RequireKeyID bool

	// ServicePolicies binds a key to its configured service after signature verification.
	ServicePolicies serviceauth.ServicePolicies

	Mode Mode

	// KeyProvider resolves HMAC secrets by key id (required for ModeHMACV2).
	KeyProvider KeyProvider
	// DefaultKeyID is used when X-Key-Id is absent and RequireKeyID is false;
	// empty means X-Key-Id is mandatory.
	DefaultKeyID string

	// APIKey is the expected API key for ModeAPIKey.
	APIKey string

	// NonceStore consumes single-use nonces (required for ModeHMACV2).
	NonceStore *NonceStore

	// MaxDrift bounds accepted timestamp skew (default 60s).
	MaxDrift time.Duration

	// V1Enabled allows the legacy v1 fallback when a request explicitly selects
	// it (no recognized v2/v3 version) AND V1Handler is set.
	V1Enabled bool
	// V1Handler runs the legacy v1 verification (delegated to middleware-kit).
	V1Handler fiber.Handler

	// Clock is injectable for tests; defaults to time.Now.
	Clock Clock
}

func deny(c fiber.Ctx, reason string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"ok": false, "reason": reason})
}

// New builds the Herald request-auth middleware. It never downgrades from a
// present-but-invalid HMAC to API-key auth.
func New(cfg Config) fiber.Handler {
	if cfg.MaxDrift <= 0 {
		cfg.MaxDrift = 60 * time.Second
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if cfg.NonceStore != nil {
		// A timestamp first accepted MaxDrift in the future remains acceptable
		// for another 2*MaxDrift, including the exact lower boundary. Keep a
		// margin beyond that boundary, and saturate rather than overflow for
		// unusually large drift settings.
		const margin = 30 * time.Second
		const maxTTL = time.Duration(1<<63 - 1)
		minTTL := maxTTL
		if cfg.MaxDrift <= (maxTTL-margin)/2 {
			minTTL = 2*cfg.MaxDrift + margin
		}
		// Copy the store so constructing middleware cannot mutate a store
		// shared with another middleware or a caller using a longer TTL.
		store := *cfg.NonceStore
		if store.ttl < minTTL {
			store.ttl = minTTL
		}
		cfg.NonceStore = &store
	}

	return func(c fiber.Ctx) error {
		// These components are joined with |kid: for idempotency namespacing.
		// Reject the reserved delimiter before authentication or any nonce/store
		// mutation. Keeping normal namespaces unchanged also preserves existing
		// non-expiring sending records across an upgrade.
		if strings.Contains(c.Get(HeaderService), "|kid:") || strings.Contains(c.Get(HeaderKeyID), "|kid:") {
			return deny(c, "invalid_auth_headers")
		}
		switch cfg.Mode {
		case ModeNone:
			return c.Next()
		case ModeAPIKey:
			return verifyAPIKey(c, cfg)
		case ModeHMACV2:
			return verifyHMACV2(c, cfg)
		default:
			return deny(c, "invalid_auth_mode")
		}
	}
}

func verifyAPIKey(c fiber.Ctx, cfg Config) error {
	provided := c.Get("X-API-Key")
	if provided == "" {
		if authz := c.Get("Authorization"); strings.HasPrefix(authz, "Bearer ") {
			provided = strings.TrimPrefix(authz, "Bearer ")
		}
	}
	if provided == "" || cfg.APIKey == "" ||
		subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.APIKey)) != 1 {
		return deny(c, "unauthorized")
	}
	return c.Next()
}

func verifyHMACV2(c fiber.Ctx, cfg Config) error {
	version := c.Get(HeaderSignatureVersion)
	if cfg.RequireV3 && version != SignatureVersionV3 {
		return deny(c, "signature_version_required")
	}

	// Legacy v1 selection: only when explicitly enabled AND the request does not
	// claim v2 or v3. Invalid recognized signatures never fall back to v1.
	if version != SignatureVersionV2 && version != SignatureVersionV3 {
		if cfg.V1Enabled && cfg.V1Handler != nil {
			return cfg.V1Handler(c)
		}
		// A v2 deployment requires the version header; reject anything else so a
		// client cannot silently avoid replay protection.
		return deny(c, "signature_version_required")
	}

	signature := c.Get(HeaderSignature)
	timestamp := c.Get(HeaderTimestamp)
	nonce := c.Get(HeaderNonce)
	service := c.Get(HeaderService)
	keyID := c.Get(HeaderKeyID)

	if missingHMACHeaders(signature, timestamp, nonce, service) {
		return deny(c, "missing_auth_headers")
	}
	if cfg.RequireKeyID && keyID == "" {
		return deny(c, "key_id_required")
	}
	// keyID as sent by the client (may be empty). It is bound into the canonical
	// string exactly as received. lookupKeyID is the id used to resolve the
	// secret, which may fall back to a configured default.
	lookupKeyID, validKey := effectiveHMACKeyID(keyID, cfg.DefaultKeyID)
	if !validKey {
		return deny(c, unusableHMACKeyReason(lookupKeyID))
	}

	if !TimestampWithinDrift(timestamp, cfg.Clock(), cfg.MaxDrift) {
		return deny(c, "timestamp_out_of_range")
	}

	if cfg.KeyProvider == nil {
		return deny(c, "unauthorized")
	}
	// An unknown key ID and a wrong signature get the same answer, so the
	// response does not tell an unauthenticated caller which key IDs exist.
	// The unknown-key path still computes a signature, against a secret no
	// caller can hold, so the two do not differ in the work done either.
	secret := cfg.KeyProvider(lookupKeyID)
	knownKey := secret != ""
	if !knownKey {
		secret = unknownKeySecret()
	}

	canonical := CanonicalRequest{
		Method:         c.Method(),
		Path:           c.Path(),
		Query:          string(c.Request().URI().QueryString()),
		Timestamp:      timestamp,
		Nonce:          nonce,
		Service:        service,
		KeyID:          keyID,
		IdempotencyKey: c.Get("Idempotency-Key"),
		Body:           c.Body(),
	}

	// Verify the signature BEFORE consuming the nonce so a forged request cannot
	// burn a legitimate nonce.
	valid := false
	if version == SignatureVersionV3 {
		valid = VerifyV3(secret, signature, canonical)
	} else {
		valid = VerifyV2(secret, signature, canonical)
	}
	if !knownKey {
		metrics.RecordAuthFailure("invalid_key_id")
		return deny(c, "invalid_signature")
	}
	if !valid {
		metrics.RecordAuthFailure("invalid_signature")
		return deny(c, "invalid_signature")
	}

	if cfg.ServicePolicies != nil {
		policy, ok := cfg.ServicePolicies[lookupKeyID]
		if !ok || policy.Service != service {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"ok": false, "reason": "forbidden"})
		}
	}
	if cfg.NonceStore == nil {
		return deny(c, "unauthorized")
	}
	return consumeVerifiedNonce(c, cfg, service, lookupKeyID, nonce)
}

func consumeVerifiedNonce(c fiber.Ctx, cfg Config, service, lookupKeyID, nonce string) error {
	fresh, err := cfg.NonceStore.Consume(c.Context(), service, lookupKeyID, nonce)
	if err != nil {
		// Backend failure: reject in every environment. Accepting the request
		// would accept replays for as long as the store is down.
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "nonce_store_unavailable"})
	}
	if !fresh {
		metrics.RecordNonceReplay()
		return deny(c, "replayed_nonce")
	}

	return continueVerifiedHMAC(c, service, lookupKeyID)
}

func missingHMACHeaders(signature, timestamp, nonce, service string) bool {
	return signature == "" || timestamp == "" || nonce == "" || service == ""
}
func effectiveHMACKeyID(keyID, defaultKeyID string) (string, bool) {
	if keyID == "" {
		keyID = defaultKeyID
	}
	return keyID, keyID != "" && !strings.Contains(keyID, "|kid:")
}

// unknownKeySecret returns a random per-process secret used to verify
// requests that name an unknown key ID. If the random source fails the value
// is empty, and the caller rejects the request regardless of the comparison.
var unknownKeySecret = sync.OnceValue(func() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
})

// LegacyKeyProvider wraps a key provider for the legacy v1 handler, which
// answers an unknown key ID with its own reason before it looks at the
// signature. A non-empty key ID the provider does not know resolves to the
// same per-process secret the v2/v3 path uses for unknown keys. The handler
// then runs its timestamp check and signature comparison as it does for a
// configured key, and the request ends as invalid_signature: no caller can
// hold that secret. An empty key ID is passed through unchanged, so a request
// that names no key is still answered by the provider's default.
func LegacyKeyProvider(next KeyProvider) KeyProvider {
	return func(keyID string) string {
		if next != nil {
			if secret := next(keyID); secret != "" {
				return secret
			}
		}
		if keyID == "" {
			return ""
		}
		metrics.RecordAuthFailure("invalid_key_id")
		return unknownKeySecret()
	}
}
