package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	tracingkit "github.com/soulteary/tracing-kit/v2"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/soulteary/herald/internal/destination"
)

// TestCreateSpanCarriesDestinationDigestOnly proves the creation spans export
// the destination as the peppered digest used for its rate-limit key, and no
// part of the address itself. A partial mask kept the first characters and
// the whole domain.
func TestCreateSpanCarriesDestinationDigestOnly(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracingkit.Install(tp, "span-destination-digest-test")
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); tracingkit.Uninstall() })

	f := newCreationLimitFixture(t)

	const dest = "spanrecipient@mail.tenant.example"
	resp := f.create(t, "span-destination-user", dest)
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))

	wantDigest := f.h.digester.Digest(destination.LimitKey("email", dest))
	digests := 0
	for _, span := range recorder.Ended() {
		for _, attr := range span.Attributes() {
			require.NotEqual(t, "destination", string(attr.Key), "span %s exports a destination attribute", span.Name())
			if attr.Key == "destination_digest" {
				require.Equal(t, wantDigest, attr.Value.AsString(), "span %s", span.Name())
				digests++
			}
			for _, fragment := range []string{dest, "spanrecipient", "mail.tenant.example"} {
				require.False(t, strings.Contains(attr.Value.String(), fragment), "span %s attribute %s=%q contains destination fragment %q", span.Name(), attr.Key, attr.Value.String(), fragment)
			}
		}
	}
	require.Equal(t, 1, digests, "exactly one span carries destination_digest")
}
