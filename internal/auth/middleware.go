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
	// ModeHMACV2 requires a valid replay-resistant v2 signature.
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
	// it (no X-Signature-Version or version != v2) AND V1Handler is set.
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

	return func(c fiber.Ctx) error {
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

	// Legacy v1 selection: only when explicitly enabled AND the request does not
	// claim v2. There is never a v2 -> v1 downgrade retry.
	if version != SignatureVersionV2 {
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
		Method:    c.Method(),
		Path:      c.Path(),
		Query:     string(c.Request().URI().QueryString()),
		Timestamp: timestamp,
		Nonce:     nonce,
		Service:   service,
		KeyID:     keyID,
		Body:      c.Body(),
	}

	// Verify the signature BEFORE consuming the nonce so a forged request cannot
	// burn a legitimate nonce.
	if !VerifyV2(secret, signature, canonical) {
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
