package config

import (
	"strings"
	"testing"
)

func TestTOTPSigningIdentityConfiguration(t *testing.T) {
	oldEnabled, oldSecret, oldID, oldService := TOTPEnabled, TOTPHMACSecret, TOTPHMACKeyID, TOTPHMACService
	t.Cleanup(func() {
		TOTPEnabled, TOTPHMACSecret, TOTPHMACKeyID, TOTPHMACService = oldEnabled, oldSecret, oldID, oldService
	})
	TOTPEnabled = true
	TOTPHMACSecret = "secret"
	for _, value := range []string{"", "herald-next", strings.Repeat("a", 128)} {
		TOTPHMACKeyID = value
		TOTPHMACService = value
		if got := totpSigningIdentityProblems(); len(got) != 0 {
			t.Fatal(got)
		}
	}
	for _, value := range []string{strings.Repeat("a", 129), "bad\nheader", " bad", "bad ", "bad\x7f"} {
		TOTPHMACKeyID = value
		TOTPHMACService = "herald"
		if len(totpSigningIdentityProblems()) != 1 {
			t.Fatalf("accepted key ID %q", value)
		}
		TOTPHMACKeyID = "next"
		TOTPHMACService = value
		if len(totpSigningIdentityProblems()) != 1 {
			t.Fatalf("accepted service %q", value)
		}
	}
	TOTPHMACSecret = ""
	if len(totpSigningIdentityProblems()) != 0 {
		t.Fatal("unused HMAC fields changed API-key-only configuration")
	}
	TOTPEnabled = false
	TOTPHMACSecret = "secret"
	if len(totpSigningIdentityProblems()) != 0 {
		t.Fatal("disabled proxy validated unused identity")
	}
}

func TestValidateRejectsInvalidTOTPSigningIdentity(t *testing.T) {
	restore := withProdEnv(t)
	defer restore()
	oldEnabled, oldSecret, oldID, oldService := TOTPEnabled, TOTPHMACSecret, TOTPHMACKeyID, TOTPHMACService
	t.Cleanup(func() {
		TOTPEnabled, TOTPHMACSecret, TOTPHMACKeyID, TOTPHMACService = oldEnabled, oldSecret, oldID, oldService
	})
	TOTPEnabled, TOTPHMACSecret, TOTPHMACKeyID, TOTPHMACService = true, "synthetic-secret", "bad\nkey", "herald"
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "HERALD_TOTP_HMAC_KEY_ID") {
		t.Fatalf("invalid signing identity did not fail startup: %v", err)
	}
}
