package handlers

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald/internal/auth"
)

func (h *Handlers) beginIdempotency(c fiber.Ctx, ctx context.Context, principal, key, fingerprint string) (json.RawMessage, string, error) {
	if service, _, ok := auth.VerifiedHMACIdentity(c); ok {
		return h.idempotencyStore.BeginWithLegacyGuard(ctx, principal, "svc:"+service+"|kid:", key, fingerprint)
	}
	return h.idempotencyStore.Begin(ctx, principal, key, fingerprint)
}
