package handlers

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	provider "github.com/soulteary/provider-kit"
	"github.com/stretchr/testify/require"

	"github.com/soulteary/herald/internal/config"
)

const (
	starttlsProbePassword = "smtp-password-must-not-leave-in-clear"
	starttlsProbeCode     = "918273"
	starttlsProbeBody     = "Your code is 918273"
)

// smtpTranscript is what a fake SMTP server received, in clear or inside TLS.
type smtpTranscript struct {
	mu    sync.Mutex
	lines []string
}

func (s *smtpTranscript) add(line string) {
	s.mu.Lock()
	s.lines = append(s.lines, strings.TrimRight(line, "\r\n"))
	s.mu.Unlock()
}

func (s *smtpTranscript) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.lines, "\n")
}

// startSMTPServer runs one SMTP conversation. onSTARTTLS answers the STARTTLS
// command and returns true when the conversation is over; every other command after EHLO is
// accepted, as a server controlled by an attacker would, so that anything the
// client is willing to send in clear ends up in the transcript.
func startSMTPServer(t *testing.T, ehlo string, onSTARTTLS func(conn net.Conn, received *smtpTranscript) bool) (string, *smtpTranscript, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	received := &smtpTranscript{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		_, _ = conn.Write([]byte("220 mail.example.com ESMTP\r\n"))
		inData := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			received.add(line)
			command := strings.ToUpper(line)
			switch {
			case inData:
				if strings.TrimRight(line, "\r\n") == "." {
					inData = false
					_, _ = conn.Write([]byte("250 queued\r\n"))
				}
			case strings.HasPrefix(command, "EHLO"), strings.HasPrefix(command, "HELO"):
				_, _ = conn.Write([]byte(ehlo))
			case strings.HasPrefix(command, "STARTTLS"):
				if onSTARTTLS(conn, received) {
					return
				}
			case strings.HasPrefix(command, "AUTH"):
				_, _ = conn.Write([]byte("235 authenticated\r\n"))
			case strings.HasPrefix(command, "DATA"):
				inData = true
				_, _ = conn.Write([]byte("354 go ahead\r\n"))
			case strings.HasPrefix(command, "QUIT"):
				_, _ = conn.Write([]byte("221 bye\r\n"))
				return
			default:
				_, _ = conn.Write([]byte("250 ok\r\n"))
			}
		}
	}()
	return listener.Addr().String(), received, done
}

// sendThroughConfiguredSMTP registers the email provider exactly as startup
// does, from the SMTP_* settings, and sends one message with a code.
func sendThroughConfiguredSMTP(t *testing.T, addr string) (*provider.SendResult, error) {
	t.Helper()
	oldURL, oldHost, oldPort := config.HeraldSMTPAPIURL, config.SMTPHost, config.SMTPPort
	oldUser, oldPassword, oldFrom, oldTimeout := config.SMTPUser, config.SMTPPassword, config.SMTPFrom, config.ProviderTimeout
	t.Cleanup(func() {
		config.HeraldSMTPAPIURL, config.SMTPHost, config.SMTPPort = oldURL, oldHost, oldPort
		config.SMTPUser, config.SMTPPassword, config.SMTPFrom, config.ProviderTimeout = oldUser, oldPassword, oldFrom, oldTimeout
	})
	host, portText, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	port, err := net.LookupPort("tcp", portText)
	require.NoError(t, err)
	// 127.0.0.1 counts as localhost for net/smtp, which would allow PLAIN
	// authentication without TLS. The STARTTLS requirement is therefore the
	// only thing that keeps the password off the wire here.
	config.HeraldSMTPAPIURL, config.SMTPHost, config.SMTPPort = "", host, port
	config.SMTPUser, config.SMTPPassword, config.SMTPFrom = "herald", starttlsProbePassword, "herald@example.com"
	config.ProviderTimeout = 5 * time.Second

	registry := provider.NewRegistry()
	require.NoError(t, registerEmailProvider(registry, testLogger()))
	registered, err := registry.Get(provider.ChannelEmail)
	require.NoError(t, err)
	require.IsType(t, &provider.SMTPProvider{}, registered)

	message := provider.NewMessage("inbox-starttls@example.com").WithCode(starttlsProbeCode).
		WithSubject("Verification code").WithBody(starttlsProbeBody)
	return registry.Send(context.Background(), provider.ChannelEmail, message)
}

func requireNothingSentInClear(t *testing.T, received *smtpTranscript) {
	t.Helper()
	transcript := received.text()
	for _, forbidden := range []string{"AUTH", "MAIL FROM", "RCPT TO", "DATA", starttlsProbeCode, "inbox-starttls@example.com"} {
		require.NotContains(t, transcript, forbidden, "sent without TLS:\n%s", transcript)
	}
	require.Contains(t, transcript, "STARTTLS", "the client did not ask for TLS:\n%s", transcript)
}

func selfSignedCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "untrusted smtp"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestBuiltInSMTPSendsNothingWithoutVerifiedTLS covers what someone on the
// network path between Herald and the SMTP server can do to STARTTLS. In
// every case the send must fail, and the password, the recipient and the
// code must not be sent: STARTTLS is required, not used when offered.
func TestBuiltInSMTPSendsNothingWithoutVerifiedTLS(t *testing.T) {
	t.Run("STARTTLS removed from the EHLO reply and refused", func(t *testing.T) {
		addr, received, done := startSMTPServer(t, "250-mail.example.com\r\n250 AUTH PLAIN LOGIN\r\n", func(conn net.Conn, _ *smtpTranscript) bool {
			_, _ = conn.Write([]byte("502 5.5.1 command not implemented\r\n"))
			return false
		})
		result, err := sendThroughConfiguredSMTP(t, addr)
		require.Error(t, err)
		require.False(t, result.OK)
		<-done
		requireNothingSentInClear(t, received)
	})

	t.Run("STARTTLS accepted and the connection left in clear", func(t *testing.T) {
		addr, received, done := startSMTPServer(t, "250-mail.example.com\r\n250-STARTTLS\r\n250 AUTH PLAIN LOGIN\r\n", func(conn net.Conn, _ *smtpTranscript) bool {
			// Answer as if TLS would follow, then keep speaking SMTP.
			_, _ = conn.Write([]byte("220 ready\r\n250-mail.example.com\r\n250 AUTH PLAIN LOGIN\r\n"))
			return false
		})
		result, err := sendThroughConfiguredSMTP(t, addr)
		require.Error(t, err)
		require.False(t, result.OK)
		<-done
		requireNothingSentInClear(t, received)
	})

	t.Run("STARTTLS with a certificate that is not trusted", func(t *testing.T) {
		certificate := selfSignedCertificate(t)
		addr, received, done := startSMTPServer(t, "250-mail.example.com\r\n250-STARTTLS\r\n250 AUTH PLAIN LOGIN\r\n", func(conn net.Conn, received *smtpTranscript) bool {
			_, _ = conn.Write([]byte("220 ready\r\n"))
			secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
			if err := secured.Handshake(); err != nil {
				return true
			}
			// Anything the client sends over a session it should not trust.
			reader := bufio.NewReader(secured)
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					return true
				}
				received.add("[tls] " + line)
				_, _ = secured.Write([]byte("250 ok\r\n"))
			}
		})
		result, err := sendThroughConfiguredSMTP(t, addr)
		require.Error(t, err)
		require.False(t, result.OK)
		category, _ := smtpFailureCategory(result, err)
		require.Equal(t, smtpFailureTLS, category)
		<-done
		requireNothingSentInClear(t, received)
		require.NotContains(t, received.text(), "[tls]", "the client used a TLS session with an untrusted certificate")
	})
}
