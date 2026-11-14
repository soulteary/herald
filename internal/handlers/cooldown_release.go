package handlers

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/http/httptrace"
	"sync/atomic"

	provider "github.com/soulteary/provider-kit"
)

// cooldownHold identifies the resend cooldowns one admitted creation started:
// the cooldown keys and the token stored in them. The token makes a release
// remove only this request's cooldowns, never one that a later request has
// started under the same keys.
type cooldownHold struct {
	keys  []string
	token string
}

// newCooldownToken returns a random cooldown value. On an entropy failure it
// returns "", which stores the plain marker; such a cooldown is never
// released.
func newCooldownToken() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}

// releaseCooldown removes the cooldowns held by this request and reports
// whether any was removed. A Redis failure leaves them to expire.
func (h *Handlers) releaseCooldown(ctx context.Context, hold *cooldownHold) bool {
	if hold == nil || hold.token == "" || len(hold.keys) == 0 {
		return false
	}
	released, err := h.rateLimitManager.ReleaseCooldowns(ctx, hold.keys, hold.token)
	if err != nil {
		h.log.Warn().Err(err).Msg("Failed to release the resend cooldown after an undelivered send; it expires on its own")
		return false
	}
	return released > 0
}

// requestWireTrace records, through net/http's client trace, whether a
// provider's HTTP request was written to a connection. attempts counts
// connection requests made by the transport, which shows that the transport
// reports to the trace at all; written counts requests whose headers went
// out. A transport retry after a broken keep-alive connection shows up as a
// written request even when the final error is a failed dial.
type requestWireTrace struct {
	attempts atomic.Int64
	written  atomic.Int64
}

func (w *requestWireTrace) attach(ctx context.Context) context.Context {
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		GetConn:      func(string) { w.attempts.Add(1) },
		WroteHeaders: func() { w.written.Add(1) },
		WroteRequest: func(httptrace.WroteRequestInfo) { w.written.Add(1) },
	})
}

// sendProvablyAcceptedNothing reports whether a failed send is one where the
// provider cannot have accepted the message. It answers true only on positive
// evidence and false in every unclear case: a timeout, a cancelled request, a
// connection that broke after it was established, an error reply over HTTP,
// an unrecognised error, or a provider type it does not know.
func (h *Handlers) sendProvablyAcceptedNothing(channel provider.Channel, result *provider.SendResult, err error, wire *requestWireTrace) bool {
	cause := err
	if cause == nil && result != nil && result.Error != nil {
		cause = result.Error
	}
	if cause == nil || sendMayHaveTimedOut(cause) {
		return false
	}
	registered, getErr := h.providerRegistry.Get(channel)
	if getErr != nil {
		// No provider for the channel: Send returned before calling one.
		var failure *provider.ProviderError
		return errors.As(cause, &failure) && failure.Reason == provider.ReasonNotRegistered
	}
	switch registered.(type) {
	case *provider.SMTPProvider:
		return smtpAcceptedNothing(result, err)
	case *provider.HTTPProvider:
		return httpRequestNeverLeft(cause, wire)
	}
	return false
}

// sendMayHaveTimedOut covers deadlines and cancellation at any step. After
// either, the message may be on its way although no reply was read.
func sendMayHaveTimedOut(err error) bool {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return true
	}
	var failure *provider.ProviderError
	return errors.As(err, &failure) && failure.Reason == provider.ReasonTimeout
}

// smtpAcceptedNothing reports whether a failed delivery through the built-in
// SMTP provider stopped before the server could accept the message:
//
//   - dns, connect: no connection existed;
//   - auth: the server refused the credentials, which precedes MAIL FROM;
//   - validation: the provider refused the message before dialling;
//   - tls: only a certificate that failed verification, which can happen
//     during the handshake alone. TLS alerts and record errors can also occur
//     on an established session and are not counted.
//
// timeout, canceled, connection, rejected and other keep the cooldown.
func smtpAcceptedNothing(result *provider.SendResult, err error) bool {
	category, _ := smtpFailureCategory(result, err)
	switch category {
	case smtpFailureDNS, smtpFailureConnect, smtpFailureAuth, smtpFailureValidation:
		return true
	case smtpFailureTLS:
		if err == nil && result != nil && result.Error != nil {
			err = result.Error
		}
		return isCertificateFailure(err)
	}
	return false
}

// httpRequestNeverLeft reports whether a failed call to an HTTP provider
// (herald-smtp, herald-dingtalk, an SMS gateway) ended before any request was
// written. Both conditions must hold: the transport reported at least one
// connection attempt and no written request, and the error is one that
// arises while a connection is being set up (name resolution, dial,
// certificate verification). Everything the remote service answers, and
// every failure after the request went out, keeps the cooldown.
func httpRequestNeverLeft(cause error, wire *requestWireTrace) bool {
	if wire == nil || wire.attempts.Load() == 0 || wire.written.Load() != 0 {
		return false
	}
	var dnsErr *net.DNSError
	if errors.As(cause, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(cause, &opErr) && opErr.Op == "dial" {
		return true
	}
	return isCertificateFailure(cause)
}

// isCertificateFailure reports a server certificate that failed verification.
func isCertificateFailure(err error) bool {
	var (
		verification *tls.CertificateVerificationError
		unknownCA    x509.UnknownAuthorityError
		hostname     x509.HostnameError
		invalid      x509.CertificateInvalidError
	)
	return errors.As(err, &verification) || errors.As(err, &unknownCA) || errors.As(err, &hostname) || errors.As(err, &invalid)
}
