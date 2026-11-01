package auth

import "github.com/gofiber/fiber/v3"

type hmacIdentityKey struct{}
type hmacIdentity struct{ service, keyID string }

// VerifiedHMACIdentity returns the signed service and the actual key used for
// verification. The wire key-ID field remains unchanged in the signed request.
func VerifiedHMACIdentity(c fiber.Ctx) (service, keyID string, ok bool) {
	identity, ok := c.Locals(hmacIdentityKey{}).(hmacIdentity)
	return identity.service, identity.keyID, ok
}
func continueVerifiedHMAC(c fiber.Ctx, service, keyID string) error {
	c.Locals(hmacIdentityKey{}, hmacIdentity{service: service, keyID: keyID})
	return c.Next()
}

func unusableHMACKeyReason(keyID string) string {
	if keyID == "" {
		return "key_id_required"
	}
	return "invalid_auth_headers"
}
