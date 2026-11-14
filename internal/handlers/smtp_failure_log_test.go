package handlers

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	logger "github.com/soulteary/logger-kit/v3"
	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

// smtpFailureFixture is the creation fixture with the built-in SMTP provider
// pointed at addr, and the handler log captured.
func smtpFailureFixture(t *testing.T, addr string) (*creationLimitFixture, *bytes.Buffer) {
	t.Helper()
	f := newCreationLimitFixture(t)
	oldSMTPURL := config.HeraldSMTPAPIURL
	t.Cleanup(func() { config.HeraldSMTPAPIURL = oldSMTPURL })
	config.HeraldSMTPAPIURL = ""

	host, portText, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := net.LookupPort("tcp", portText)
	require.NoError(t, err)
	smtp, err := provider.NewSMTPProvider(&provider.SMTPConfig{
		Host: host, Port: port, From: "herald@example.com", UseStartTLS: true, Timeout: 5 * time.Second,
	})
	require.NoError(t, err)
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(smtp))

	var logs bytes.Buffer
	f.h.log = logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON, Output: &logs})
	return f, &logs
}

func sendFailureLine(t *testing.T, logs *bytes.Buffer) string {
	t.Helper()
	for _, line := range strings.Split(logs.String(), "\n") {
		if strings.Contains(line, "Failed to send verification code via provider") {
			return line
		}
	}
	t.Fatalf("no send failure was logged: %s", logs.String())
	return ""
}

// TestSMTPFailureLogSaysWhereDeliveryStopped sends through the built-in SMTP
// provider to a port nothing listens on, and to a server that refuses
// STARTTLS. Both used to log only "reason":"send_failed", which does not tell
// a wrong host from a server without STARTTLS. The log line must name the
// category, and still must not contain the destination or the provider's
// error text.
func TestSMTPFailureLogSaysWhereDeliveryStopped(t *testing.T) {
	const destination = "inbox-smtp-failure@example.com"

	t.Run("nothing listens", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := listener.Addr().String()
		require.NoError(t, listener.Close())

		f, logs := smtpFailureFixture(t, addr)
		f.create(t, "smtp-failure-user", destination)

		line := sendFailureLine(t, logs)
		require.Contains(t, line, `"reason":"send_failed"`)
		require.Contains(t, line, `"failure_category":"connect"`, line)
		require.NotContains(t, line, "smtp_code")
		require.NotContains(t, line, destination)
		require.NotContains(t, line, "refused")
	})

	t.Run("server refuses STARTTLS", func(t *testing.T) {
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
				switch {
				case strings.HasPrefix(strings.ToUpper(command), "EHLO"):
					_, _ = conn.Write([]byte("250 mail.example.com\r\n"))
				case strings.HasPrefix(strings.ToUpper(command), "STARTTLS"):
					_, _ = conn.Write([]byte("502 5.5.1 STARTTLS is not offered to " + destination + "\r\n"))
				default:
					_, _ = conn.Write([]byte("221 bye\r\n"))
					return
				}
			}
		}()

		f, logs := smtpFailureFixture(t, listener.Addr().String())
		f.create(t, "smtp-failure-user", destination)

		line := sendFailureLine(t, logs)
		require.Contains(t, line, `"failure_category":"rejected"`, line)
		require.Contains(t, line, `"smtp_code":502`, line)
		require.NotContains(t, line, destination)
		require.NotContains(t, line, "not offered")
	})
}

// A channel that is not served by the built-in SMTP provider keeps its log
// line as it was.
func TestSendFailureLogForOtherProvidersHasNoSMTPCategory(t *testing.T) {
	f := newCreationLimitFixture(t)
	oldSMTPURL := config.HeraldSMTPAPIURL
	t.Cleanup(func() { config.HeraldSMTPAPIURL = oldSMTPURL })
	config.HeraldSMTPAPIURL = "https://herald-smtp.internal"
	f.h.providerRegistry = provider.NewRegistry()
	require.NoError(t, f.h.providerRegistry.Register(refusingEmailProvider{}))
	var logs bytes.Buffer
	f.h.log = logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON, Output: &logs})

	f.create(t, "http-provider-user", "inbox-http-provider@example.com")
	line := sendFailureLine(t, &logs)
	require.Contains(t, line, `"reason":`)
	require.NotContains(t, line, "failure_category")
}

// refusingEmailProvider stands in for the herald-smtp HTTP provider and fails
// every send.
type refusingEmailProvider struct{}

func (refusingEmailProvider) Send(context.Context, *provider.Message) (*provider.SendResult, error) {
	failure := provider.ErrProviderDown("upstream unavailable", nil)
	return provider.NewFailureResult("smtp", provider.ChannelEmail, failure), failure
}
func (refusingEmailProvider) Channel() provider.Channel { return provider.ChannelEmail }
func (refusingEmailProvider) Name() string              { return "smtp" }
func (refusingEmailProvider) Validate() error           { return nil }
