package config

import (
	"strings"
	"testing"
)

func TestServicePolicyConfiguration(t *testing.T) {
	oldPolicy, oldMode, oldV1, oldSecret, oldMap := ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, HMACSecret, hmacKeysMap
	defer func() {
		ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, HMACSecret, hmacKeysMap = oldPolicy, oldMode, oldV1, oldSecret, oldMap
	}()
	for _, tc := range []struct {
		name, raw, mode, secret string
		v1, wantErr             bool
	}{
		{name: "disabled", mode: "api_key"},
		{name: "invalid JSON", raw: "{", mode: "hmac_v2", wantErr: true},
		{name: "invalid auth", raw: `{"default":{"service":"gateway","actions":["otp:verify"]}}`, mode: "typo", wantErr: true},
		{name: "API key", raw: `{"default":{"service":"gateway","actions":["otp:verify"]}}`, mode: "api_key", secret: "secret", wantErr: true},
		{name: "legacy signature", raw: `{"default":{"service":"gateway","actions":["otp:verify"]}}`, mode: "hmac_v2", secret: "secret", v1: true, wantErr: true},
		{name: "missing key", raw: `{"default":{"service":"gateway","actions":["otp:verify"]}}`, mode: "hmac_v2", wantErr: true},
		{name: "unknown key", raw: `{"other":{"service":"gateway","actions":["otp:verify"]}}`, mode: "hmac_v2", secret: "secret", wantErr: true},
		{name: "single key", raw: `{"default":{"service":"gateway","actions":["otp:verify"]}}`, mode: "hmac_v2", secret: "secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, HMACSecret, hmacKeysMap = tc.raw, tc.mode, tc.v1, tc.secret, nil
			_, err := ServicePolicies()
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v", err)
			}
		})
	}
	ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, HMACSecret, hmacKeysMap = `{"k":{"service":"gateway","actions":["otp:verify"]}}`, "hmac_v2", false, "", map[string]string{"k": "secret"}
	if _, err := ServicePolicies(); err != nil {
		t.Fatal(err)
	}
	ServicePoliciesJSON = "{"
	if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "HERALD_SERVICE_POLICIES") {
		t.Fatalf("strict error=%v", err)
	}
}

func TestMetricsProductionSecretLength(t *testing.T) {
	old := MetricsAPIKey
	defer func() { MetricsAPIKey = old }()
	for _, tc := range []struct {
		key string
		bad bool
	}{{"", false}, {"short", true}, {strings.Repeat("m", 32), false}} {
		MetricsAPIKey = tc.key
		found := false
		for _, p := range validateProductionSecretMaterial() {
			if strings.Contains(p, "HERALD_METRICS_API_KEY") {
				found = true
			}
		}
		if found != tc.bad {
			t.Fatalf("key length=%d invalid=%v", len(tc.key), found)
		}
	}
}
