package handlers

import (
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/config"
	provider "github.com/soulteary/provider-kit"
)

func TestProviderInitialization(t *testing.T) {
	oldURL, oldHost, oldFrom, oldPort := config.HeraldSMTPAPIURL, config.SMTPHost, config.SMTPFrom, config.SMTPPort
	oldSMS, oldSMSURL, oldDing, oldRedirect := config.SMSProvider, config.SMSAPIBaseURL, config.HeraldDingtalkAPIURL, config.ProviderRedirectPolicy
	defer func() {
		config.HeraldSMTPAPIURL, config.SMTPHost, config.SMTPFrom, config.SMTPPort = oldURL, oldHost, oldFrom, oldPort
		config.SMSProvider, config.SMSAPIBaseURL, config.HeraldDingtalkAPIURL, config.ProviderRedirectPolicy = oldSMS, oldSMSURL, oldDing, oldRedirect
	}()
	rc := testRedisClient(t)
	defer func() { _ = rc.Close() }()
	config.HeraldSMTPAPIURL = ""
	config.SMTPHost, config.SMTPFrom, config.SMTPPort = "smtp.example.com", "sender@example.com", 587
	config.SMSProvider, config.SMSAPIBaseURL = "sms", "https://sms.example.com"
	config.HeraldDingtalkAPIURL = "https://dingtalk.example.com"
	config.ProviderRedirectPolicy = "same-origin"
	h, err := NewHandlersWithError(rc, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range []provider.Channel{provider.ChannelEmail, provider.ChannelSMS, provider.ChannelDingTalk} {
		if _, err := h.providerRegistry.Get(channel); err != nil {
			t.Fatalf("provider %s not registered: %v", channel, err)
		}
	}
	config.SMTPFrom = ""
	if _, err := NewHandlersWithError(rc, testLogger()); err == nil || !strings.Contains(err.Error(), "SMTP provider config invalid") {
		t.Fatalf("missing SMTP sender: %v", err)
	}
	config.SMTPFrom = "sender@example.com"
	config.HeraldDingtalkAPIURL = "://invalid"
	if _, err := NewHandlersWithError(rc, testLogger()); err == nil || !strings.Contains(err.Error(), "herald-dingtalk provider config invalid") {
		t.Fatalf("invalid DingTalk URL: %v", err)
	}
	if h := NewHandlers(rc, testLogger()); h != nil {
		t.Fatal("legacy constructor returned partially configured handlers")
	}
}
