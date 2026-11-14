package handlers

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

func closedLocalAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return addr
}

func requireSendFailed(t *testing.T, resp *fiberTestResp) {
	t.Helper()
	require.Equal(t, fiber.StatusInternalServerError, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"reason":"send_failed"`)
}

func (f *creationLimitFixture) requireCooldown(t *testing.T, userID, dest string, held bool) {
	t.Helper()
	require.Equal(t, held, f.mr.Exists(f.userCooldownKey(userID, dest)), "user+destination cooldown")
	require.Equal(t, held, f.mr.Exists(f.destCooldownKey(dest)), "destination cooldown")
}

// useHTTPEmailProvider replaces the fixture's provider with a real HTTP
// provider for the email channel, as herald-smtp is wired.
func (f *creationLimitFixture) useHTTPEmailProvider(t *testing.T, baseURL string) {
	t.Helper()
	httpProvider, err := provider.NewHTTPProvider(&provider.HTTPConfig{
		BaseURL: baseURL, SendEndpoint: "/v1/send", ChannelType: provider.ChannelEmail, ProviderName: "smtp", Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(httpProvider))
}

// TestCreateChallenge_UndeliveredSendReleasesCooldown sends through the
// built-in SMTP provider to a port nothing listens on. No connection exists,
// so no message can have been accepted, and the caller must not be answered
// 429/resend_cooldown for 60 seconds afterwards, as it used to be. The quota
// counters are not given back and stop the retries.
func TestCreateChallenge_UndeliveredSendReleasesCooldown(t *testing.T) {
	const user, dest = "undelivered-user", "undelivered@example.com"
	f, logs := smtpFailureFixture(t, closedLocalAddr(t))
	config.RateLimitPerDestination = 3

	for attempt := 1; attempt <= 3; attempt++ {
		requireSendFailed(t, f.create(t, user, dest))
		f.requireCooldown(t, user, dest, false)
		state := f.limiterState(t)
		want := fmt.Sprint(attempt)
		require.Equal(t, want, state[f.userKey(user)], "the user counter stays spent")
		require.Equal(t, want, state[f.destKey(dest)], "the destination counter stays spent")
		require.Equal(t, want, f.ipCounter(t), "the IP counter stays spent")
	}
	require.Contains(t, sendFailureLine(t, logs), `"cooldown_released":true`)

	// The destination limit, not the cooldown, ends the retries.
	resp := f.create(t, user, dest)
	requireRateLimited(t, resp, "rate_limit_exceeded", 3600)
	require.Contains(t, string(resp.body), `"limit":"destination"`)
}

// A server that answers and then refuses a command has been reached. That
// category is not among the releasing ones, so the cooldown stays.
func TestCreateChallenge_RejectedSendKeepsCooldown(t *testing.T) {
	const user, dest = "rejected-user", "rejected@example.com"
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 mail.example.com ESMTP\r\n"))
		for {
			command, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(strings.ToUpper(command), "EHLO") {
				_, _ = conn.Write([]byte("250 mail.example.com\r\n"))
				continue
			}
			_, _ = conn.Write([]byte("502 5.5.1 not implemented\r\n"))
		}
	}()

	f, logs := smtpFailureFixture(t, listener.Addr().String())
	requireSendFailed(t, f.create(t, user, dest))
	f.requireCooldown(t, user, dest, true)
	require.Contains(t, sendFailureLine(t, logs), `"cooldown_released":false`)
	requireRateLimited(t, f.create(t, user, dest), "resend_cooldown", 60)
}

// A delivered message keeps its cooldown, and a later request cannot remove
// it: it is refused at admission and never reaches the provider.
func TestCreateChallenge_DeliveredSendKeepsCooldown(t *testing.T) {
	const user, dest = "delivered-user", "delivered@example.com"
	f := newCreationLimitFixture(t)
	require.Equal(t, fiber.StatusOK, f.create(t, user, dest).status)
	f.requireCooldown(t, user, dest, true)

	// Every later send would fail without a connection.
	smtp, err := provider.NewSMTPProvider(&provider.SMTPConfig{Host: "127.0.0.1", Port: 1, From: "herald@example.com", UseStartTLS: true, Timeout: time.Second})
	require.NoError(t, err)
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(smtp))
	requireRateLimited(t, f.create(t, user, dest), "resend_cooldown", 60)
	requireRateLimited(t, f.create(t, "another-user", dest), "resend_cooldown", 60)
	f.requireCooldown(t, user, dest, true)
}

// Under the soft policy the challenge stays usable and the response is 200
// with next_resend_in, so the cooldown is kept.
func TestCreateChallenge_SoftPolicyKeepsCooldownAfterUndeliveredSend(t *testing.T) {
	const user, dest = "soft-user", "soft@example.com"
	f, _ := smtpFailureFixture(t, closedLocalAddr(t))
	config.ProviderFailurePolicy = "soft"
	resp := f.create(t, user, dest)
	require.Equal(t, fiber.StatusOK, resp.status, string(resp.body))
	require.Contains(t, string(resp.body), `"delivery_status":"failed"`)
	f.requireCooldown(t, user, dest, true)
}

// A provider type Herald does not know gives no evidence either way.
func TestCreateChallenge_UnknownProviderTypeKeepsCooldown(t *testing.T) {
	const user, dest = "custom-provider-user", "custom-provider@example.com"
	f := newCreationLimitFixture(t)
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(dialFailingProvider{}))
	requireSendFailed(t, f.create(t, user, dest))
	f.requireCooldown(t, user, dest, true)
}

// dialFailingProvider is not one of provider-kit's providers and reports an
// error that looks like a failed dial.
type dialFailingProvider struct{}

func (dialFailingProvider) Send(context.Context, *provider.Message) (*provider.SendResult, error) {
	failure := provider.ErrProviderDown("unreachable", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})
	return provider.NewFailureResult("custom", provider.ChannelEmail, failure), failure
}
func (dialFailingProvider) Channel() provider.Channel { return provider.ChannelEmail }
func (dialFailingProvider) Name() string              { return "custom" }
func (dialFailingProvider) Validate() error           { return nil }

// An HTTP provider whose service cannot be connected to never received the
// request, so the cooldown is released.
func TestCreateChallenge_HTTPProviderUnreachableReleasesCooldown(t *testing.T) {
	const user, dest = "http-unreachable-user", "http-unreachable@example.com"
	f := newCreationLimitFixture(t)
	f.useHTTPEmailProvider(t, "http://"+closedLocalAddr(t))
	requireSendFailed(t, f.create(t, user, dest))
	f.requireCooldown(t, user, dest, false)
	requireSendFailed(t, f.create(t, user, dest))
}

// An HTTP provider that received the request keeps the cooldown whatever it
// answers: the service behind it may have sent the message.
func TestCreateChallenge_HTTPProviderReachedKeepsCooldown(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"error status": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		},
		"structured failure": func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":false,"error_code":"provider_down"}`))
		},
		"connection dropped after the request": func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			const user, dest = "http-reached-user", "http-reached@example.com"
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				handler(w, r)
			}))
			defer server.Close()
			f := newCreationLimitFixture(t)
			f.useHTTPEmailProvider(t, server.URL)
			requireSendFailed(t, f.create(t, user, dest))
			require.Positive(t, requests.Load())
			f.requireCooldown(t, user, dest, true)
			requireRateLimited(t, f.create(t, user, dest), "resend_cooldown", 60)
		})
	}
}

func sendFailure(cause error) *provider.ProviderError {
	return provider.ErrSendFailed("failed to send email", cause).WithProvider("smtp", provider.ChannelEmail)
}

// The SMTP decision per failure category, from error types alone.
func TestSMTPAcceptedNothing(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"dns":                         {sendFailure(&net.DNSError{Err: "no such host", Name: "smtp.invalid", IsNotFound: true}), true},
		"connect":                     {sendFailure(dial), true},
		"auth":                        {sendFailure(&textproto.Error{Code: 535, Msg: "authentication failed"}), true},
		"validation":                  {provider.ErrValidationFailed("subject contains prohibited control characters"), true},
		"invalid destination":         {provider.ErrInvalidDestination("recipient email address is invalid"), true},
		"tls certificate":             {sendFailure(&tls.CertificateVerificationError{Err: x509.UnknownAuthorityError{}}), true},
		"tls hostname":                {sendFailure(x509.HostnameError{Host: "smtp.example.com"}), true},
		"tls alert":                   {sendFailure(tls.AlertError(40)), false},
		"tls record header":           {sendFailure(tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}), false},
		"rejected":                    {sendFailure(&textproto.Error{Code: 550, Msg: "mailbox unavailable"}), false},
		"connection reset":            {sendFailure(&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}), false},
		"connection closed":           {sendFailure(io.EOF), false},
		"timeout":                     {provider.ErrTimeout("SMTP send timed out", context.DeadlineExceeded), false},
		"dial timeout":                {sendFailure(&net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}), false},
		"dns timeout":                 {sendFailure(&net.DNSError{Err: "i/o timeout", Name: "smtp.example.com", IsTimeout: true}), false},
		"canceled":                    {sendFailure(context.Canceled), false},
		"unrecognised":                {sendFailure(errors.New("something else")), false},
		"unrecognised provider error": {provider.ErrSendFailed("failed", nil), false},
	} {
		t.Run(name, func(t *testing.T) {
			got := !sendMayHaveTimedOut(tc.err) && smtpAcceptedNothing(nil, tc.err)
			require.Equal(t, tc.want, got)
			// The same error carried only in the result.
			var failure *provider.ProviderError
			if errors.As(tc.err, &failure) {
				result := provider.NewFailureResult("smtp", provider.ChannelEmail, failure)
				require.Equal(t, tc.want, !sendMayHaveTimedOut(failure) && smtpAcceptedNothing(result, nil))
			}
		})
	}
}

// The HTTP decision needs both the wire evidence and a connection-setup
// error. A request that was written counts as sent even when the final
// error is a failed dial, as after a transport retry.
func TestHTTPRequestNeverLeft(t *testing.T) {
	dial := provider.ErrProviderDown("HTTP request failed", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})
	wire := func(attempts, written int64) *requestWireTrace {
		w := &requestWireTrace{}
		w.attempts.Store(attempts)
		w.written.Store(written)
		return w
	}
	require.True(t, httpRequestNeverLeft(dial, wire(1, 0)))
	require.True(t, httpRequestNeverLeft(provider.ErrProviderDown("HTTP request failed", &net.DNSError{Err: "no such host", IsNotFound: true}), wire(1, 0)))
	require.True(t, httpRequestNeverLeft(provider.ErrProviderDown("HTTP request failed", x509.UnknownAuthorityError{}), wire(1, 0)))

	require.False(t, httpRequestNeverLeft(dial, wire(2, 1)), "an earlier attempt wrote the request")
	require.False(t, httpRequestNeverLeft(dial, wire(0, 0)), "the transport did not report to the trace")
	require.False(t, httpRequestNeverLeft(dial, nil))
	require.False(t, httpRequestNeverLeft(provider.ErrProviderDown("HTTP request failed", io.EOF), wire(1, 0)))
	require.False(t, httpRequestNeverLeft(provider.ErrSendFailed("provider returned HTTP 502", nil), wire(1, 0)))
}

// Without a provider for the channel Send returns before calling one.
func TestSendProvablyAcceptedNothing_NoProviderAndNoError(t *testing.T) {
	f := newCreationLimitFixture(t)
	f.h.providerRegistry = provider.NewRegistry()
	_, err := f.h.providerRegistry.Send(context.Background(), provider.ChannelEmail, provider.NewMessage("a@example.com"))
	require.Error(t, err)
	require.True(t, f.h.sendProvablyAcceptedNothing(provider.ChannelEmail, nil, err, &requestWireTrace{}))
	require.False(t, f.h.sendProvablyAcceptedNothing(provider.ChannelEmail, nil, errors.New("other"), &requestWireTrace{}))
	require.False(t, f.h.sendProvablyAcceptedNothing(provider.ChannelEmail, nil, nil, &requestWireTrace{}))
	require.False(t, f.h.releaseCooldown(context.Background(), nil))
	require.False(t, f.h.releaseCooldown(context.Background(), &cooldownHold{}))
}
