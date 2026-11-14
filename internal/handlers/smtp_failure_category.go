package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/textproto"

	provider "github.com/soulteary/provider-kit"

	"github.com/soulteary/herald/internal/config"
)

// Categories of a failed delivery through the built-in SMTP provider. They
// say where the attempt stopped, so an operator can tell a wrong host from a
// wrong password without the provider's error text, which is never logged.
const (
	smtpFailureTimeout    = "timeout"    // a deadline passed at any step
	smtpFailureCanceled   = "canceled"   // the request was abandoned
	smtpFailureDNS        = "dns"        // SMTP_HOST did not resolve
	smtpFailureConnect    = "connect"    // the TCP connection was not established
	smtpFailureTLS        = "tls"        // the TLS handshake or certificate check failed
	smtpFailureAuth       = "auth"       // the server refused the credentials
	smtpFailureRejected   = "rejected"   // the server answered a command with an error
	smtpFailureConnection = "connection" // the connection broke after it was established
	smtpFailureValidation = "validation" // the message or configuration was refused before sending
	smtpFailureOther      = "other"      // none of the above could be recognised
)

// builtInSMTPDelivers reports whether the channel is served by the built-in
// SMTP provider, which is the case for email unless herald-smtp is used.
func builtInSMTPDelivers(channel string) bool {
	return channel == string(provider.ChannelEmail) && config.HeraldSMTPAPIURL == ""
}

// smtpFailureCategory classifies a failed SMTP delivery and returns the SMTP
// reply code when the server answered with one (0 otherwise).
//
// Only the types in the error chain are examined, never its text: provider
// messages can repeat the message body, the destination or credentials. The
// two values returned are a fixed word and a three-digit number, so both are
// safe to log. The reply code cannot say which command was refused; the
// provider reports that in text only.
func smtpFailureCategory(result *provider.SendResult, err error) (string, int) {
	if err == nil && result != nil && result.Error != nil {
		err = result.Error
	}
	if err == nil {
		return smtpFailureOther, 0
	}

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return smtpFailureTimeout, 0
	}
	if errors.Is(err, context.Canceled) {
		return smtpFailureCanceled, 0
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return smtpFailureDNS, 0
	}
	if isTLSFailure(err) {
		return smtpFailureTLS, 0
	}
	var reply *textproto.Error
	if errors.As(err, &reply) {
		switch reply.Code {
		case 454, 530, 534, 535, 538:
			return smtpFailureAuth, reply.Code
		}
		if reply.Code >= 100 && reply.Code <= 599 {
			return smtpFailureRejected, reply.Code
		}
		return smtpFailureRejected, 0
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Op == "dial" {
			return smtpFailureConnect, 0
		}
		return smtpFailureConnection, 0
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return smtpFailureConnection, 0
	}
	var failure *provider.ProviderError
	if errors.As(err, &failure) {
		switch failure.Reason {
		case provider.ReasonValidationFailed, provider.ReasonInvalidDestination, provider.ReasonInvalidConfig:
			return smtpFailureValidation, 0
		case provider.ReasonTimeout:
			return smtpFailureTimeout, 0
		}
	}
	return smtpFailureOther, 0
}

func isTLSFailure(err error) bool {
	var (
		verification *tls.CertificateVerificationError
		unknownCA    x509.UnknownAuthorityError
		hostname     x509.HostnameError
		invalid      x509.CertificateInvalidError
		header       tls.RecordHeaderError
		alert        tls.AlertError
	)
	return errors.As(err, &verification) || errors.As(err, &unknownCA) || errors.As(err, &hostname) ||
		errors.As(err, &invalid) || errors.As(err, &header) || errors.As(err, &alert)
}
