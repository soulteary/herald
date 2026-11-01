package auth

import (
	"crypto/subtle"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"
	"github.com/soulteary/herald/internal/metrics"
)

// Clock is injectable for deterministic tests.
type Clock func() time.Time

// KeyProvider returns the shared secret for a key id, or "" if unknown.
type KeyProvider func(keyID string) string

// Mode selects the request-body authentication scheme.
type Mode string

const (
	// ModeHMACV2 selects HMAC authentication; v2/v3 acceptance is route-specific.
	ModeHMACV2 Mode = "hmac_v2"
	// ModeAPIKey requires a matching API key (no HMAC).
	ModeAPIKey Mode = "api_key"
	// ModeNone disables request-body auth (development only).
	ModeNone Mode = "none"
)

// ParseMode maps a config string to a Mode, defaulting to ModeHMACV2.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "api_key", "apikey":
		return ModeAPIKey
	case "none", "off", "disabled":
		return ModeNone
	default:
		return ModeHMACV2
	}
}

// Config configures the Herald auth middleware.
type Config struct {
	// RequireV3 rejects older HMAC schemes on routes whose side effects depend on idempotency.
	RequireV3 bool

	Mode Mode

	// KeyProvider resolves HMAC secrets by key id (required for ModeHMACV2).
	KeyProvider KeyProvider
	// DefaultKeyID is used when X-Key-Id is absent; empty means X-Key-Id is
	// mandatory.
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

	// FailClosed makes nonce-store backend failures reject the request. Set true
	// in production.
	FailClosed bool

	// Clock is injectable for tests; defaults to time.Now.
	Clock Clock

	Logger *zerolog.Logger
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
		default:
			return verifyHMACV2(c, cfg)
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

	if signature == "" || timestamp == "" || nonce == "" || service == "" {
		return deny(c, "missing_auth_headers")
	}
	// keyID as sent by the client (may be empty). It is bound into the canonical
	// string exactly as received. lookupKeyID is the id used to resolve the
	// secret, which may fall back to a configured default.
	lookupKeyID := keyID
	if lookupKeyID == "" {
		if cfg.DefaultKeyID == "" {
			return deny(c, "key_id_required")
		}
		lookupKeyID = cfg.DefaultKeyID
	}

	if !TimestampWithinDrift(timestamp, cfg.Clock(), cfg.MaxDrift) {
		return deny(c, "timestamp_out_of_range")
	}

	if cfg.KeyProvider == nil {
		return deny(c, "unauthorized")
	}
	secret := cfg.KeyProvider(lookupKeyID)
	if secret == "" {
		return deny(c, "invalid_key_id")
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
	if !valid {
		return deny(c, "invalid_signature")
	}

	if cfg.NonceStore == nil {
		return deny(c, "unauthorized")
	}
	fresh, err := cfg.NonceStore.Consume(c.Context(), service, lookupKeyID, nonce)
	if err != nil {
		// Backend failure: fail closed in production, otherwise allow (dev).
		if cfg.FailClosed {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"ok": false, "reason": "nonce_store_unavailable"})
		}
		if cfg.Logger != nil {
			cfg.Logger.Warn().Msg("nonce store unavailable; allowing request (non-production)")
		}
		return c.Next()
	}
	if !fresh {
		metrics.RecordNonceReplay()
		return deny(c, "replayed_nonce")
	}

	return c.Next()
}
