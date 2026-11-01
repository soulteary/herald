package config

import "testing"

func TestSingleSecretRejectsUnconfiguredKeyIDs(t *testing.T) {
	oldSecret, oldMap := HMACSecret, hmacKeysMap
	HMACSecret, hmacKeysMap = "single-secret", nil
	t.Cleanup(func() { HMACSecret, hmacKeysMap = oldSecret, oldMap })
	for _, id := range []string{"", "default", "unknown", "Default", "default ", "retired-key"} {
		want := ""
		if id == "" || id == "default" {
			want = HMACSecret
		}
		if got := GetHMACSecret(id); got != want {
			t.Errorf("key ID %q: accepted=%v, want accepted=%v", id, got != "", want != "")
		}
	}
}

func TestExplicitKeyMapDoesNotFallBackToSingleSecret(t *testing.T) {
	oldSecret, oldMap, oldDefault := HMACSecret, hmacKeysMap, hmacDefaultKeyID
	HMACSecret = "unused-fallback"
	hmacKeysMap = map[string]string{"custom": "mapped-secret", "default": "other-secret"}
	hmacDefaultKeyID = "custom"
	t.Cleanup(func() {
		HMACSecret, hmacKeysMap, hmacDefaultKeyID = oldSecret, oldMap, oldDefault
	})
	for id, want := range map[string]string{"": "mapped-secret", "custom": "mapped-secret", "default": "other-secret", "unknown": ""} {
		if GetHMACSecret(id) != want {
			t.Errorf("incorrect configured-map lookup for key ID %q", id)
		}
	}
}
