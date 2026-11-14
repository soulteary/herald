package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"testing"

	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// The provider wraps every failure as it does in production: a ProviderError
// around a stage message around the underlying error.
func wrappedSMTPFailure(cause error) error {
	return provider.ErrSendFailed("failed to send email", fmt.Errorf("stage otp=123456 user@example.com: %w", cause))
}

func TestSMTPFailureCategoryFromErrorTypes(t *testing.T) {
	for name, tc := range map[string]struct {
		cause    error
		category string
		code     int
	}{
		"deadline":             {context.DeadlineExceeded, smtpFailureTimeout, 0},
		"network timeout":      {&net.OpError{Op: "read", Err: timeoutError{}}, smtpFailureTimeout, 0},
		"canceled":             {context.Canceled, smtpFailureCanceled, 0},
		"dns":                  {&net.OpError{Op: "dial", Err: &net.DNSError{Err: "no such host", Name: "smtp.example.com", IsNotFound: true}}, smtpFailureDNS, 0},
		"connection refused":   {&net.OpError{Op: "dial", Err: errors.New("connect: connection refused")}, smtpFailureConnect, 0},
		"unknown authority":    {x509.UnknownAuthorityError{}, smtpFailureTLS, 0},
		"wrong host name":      {x509.HostnameError{Host: "smtp.example.com"}, smtpFailureTLS, 0},
		"verification":         {&tls.CertificateVerificationError{Err: errors.New("expired")}, smtpFailureTLS, 0},
		"not a TLS server":     {tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}, smtpFailureTLS, 0},
		"bad credentials":      {&textproto.Error{Code: 535, Msg: "5.7.8 Username and Password not accepted"}, smtpFailureAuth, 535},
		"auth required":        {&textproto.Error{Code: 530, Msg: "5.7.0 Authentication Required"}, smtpFailureAuth, 530},
		"recipient refused":    {&textproto.Error{Code: 550, Msg: "5.1.1 user@example.com unknown"}, smtpFailureRejected, 550},
		"STARTTLS not offered": {&textproto.Error{Code: 502, Msg: "command not implemented"}, smtpFailureRejected, 502},
		"reset mid-session":    {&net.OpError{Op: "write", Err: errors.New("broken pipe")}, smtpFailureConnection, 0},
		"server hung up":       {io.EOF, smtpFailureConnection, 0},
		"unrecognised":         {errors.New("unencrypted connection otp=123456"), smtpFailureOther, 0},
	} {
		t.Run(name, func(t *testing.T) {
			err := wrappedSMTPFailure(tc.cause)
			category, code := smtpFailureCategory(nil, err)
			require.Equal(t, tc.category, category)
			require.Equal(t, tc.code, code)

			// The same failure carried on the result only.
			var failure *provider.ProviderError
			require.True(t, errors.As(err, &failure))
			category, code = smtpFailureCategory(provider.NewFailureResult("smtp", provider.ChannelEmail, failure), nil)
			require.Equal(t, tc.category, category)
			require.Equal(t, tc.code, code)
		})
	}

	category, code := smtpFailureCategory(nil, provider.ErrValidationFailed("subject contains prohibited control characters"))
	require.Equal(t, smtpFailureValidation, category)
	require.Zero(t, code)
	category, _ = smtpFailureCategory(nil, provider.ErrTimeout("SMTP send timed out", nil))
	require.Equal(t, smtpFailureTimeout, category)
	category, code = smtpFailureCategory(nil, nil)
	require.Equal(t, smtpFailureOther, category)
	require.Zero(t, code)
	// A reply code outside the SMTP range is not passed on.
	_, code = smtpFailureCategory(nil, wrappedSMTPFailure(&textproto.Error{Code: 123456, Msg: "x"}))
	require.Zero(t, code)
}

func TestBuiltInSMTPDeliversEmailOnlyWithoutHeraldSMTP(t *testing.T) {
	old := config.HeraldSMTPAPIURL
	t.Cleanup(func() { config.HeraldSMTPAPIURL = old })

	config.HeraldSMTPAPIURL = ""
	require.True(t, builtInSMTPDelivers("email"))
	require.False(t, builtInSMTPDelivers("sms"))
	require.False(t, builtInSMTPDelivers("dingtalk"))
	config.HeraldSMTPAPIURL = "https://herald-smtp.internal"
	require.False(t, builtInSMTPDelivers("email"))
}
