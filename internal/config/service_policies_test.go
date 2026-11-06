package config

import (
	logger "github.com/soulteary/logger-kit/v3"
	"strings"
	"sync"
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

func TestServicePoliciesRejectSharedSecretsAcrossAllKeys(t *testing.T) {
	oldPolicy, oldMode, oldV1, oldMap := ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, hmacKeysMap
	defer func() {
		ServicePoliciesJSON, RequestAuthMode, HMACV1Enabled, hmacKeysMap = oldPolicy, oldMode, oldV1, oldMap
	}()
	ServicePoliciesJSON = `{"admin":{"service":"admin","actions":["totp:revoke"]}}`
	RequestAuthMode, HMACV1Enabled = "hmac_v2", false
	for _, keys := range []map[string]string{
		{"admin": "shared", "reader": "shared"},
		{"admin": "admin-secret", "unmapped-a": "shared", "unmapped-b": "shared"},
	} {
		hmacKeysMap = keys
		if _, err := ServicePolicies(); err == nil || !strings.Contains(err.Error(), "distinct secrets") {
			t.Fatalf("shared keys error=%v", err)
		}
	}
	hmacKeysMap = map[string]string{"admin": "admin-secret", "reader": "reader-secret"}
	if _, err := ServicePolicies(); err != nil {
		t.Fatalf("distinct keys error=%v", err)
	}
}

func TestInitializePolicyRejectsMalformedHMACKeys(t *testing.T) {
	oldPolicy, oldRaw := ServicePoliciesJSON, HMACKeysJSON
	oldMap, oldDefault := hmacKeysMap, hmacDefaultKeyID
	defer func() {
		ServicePoliciesJSON, HMACKeysJSON = oldPolicy, oldRaw
		hmacKeysMap, hmacDefaultKeyID = oldMap, oldDefault
		hmacKeysMapOnce = sync.Once{}
	}()
	ServicePoliciesJSON = `{"default":{"service":"gateway","actions":["otp:verify"]}}`
	HMACKeysJSON = "{"
	hmacKeysMap = nil
	hmacKeysMapOnce = sync.Once{}
	if err := Initialize(logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})); err == nil {
		t.Fatal("malformed key map accepted with policies")
	}
}

func TestMetricsCredentialCannotAuthenticateBusinessRequests(t *testing.T) {
	oldMetrics, oldAPI, oldHMAC, oldKeys := MetricsAPIKey, APIKey, HMACSecret, hmacKeysMap
	defer func() { MetricsAPIKey, APIKey, HMACSecret, hmacKeysMap = oldMetrics, oldAPI, oldHMAC, oldKeys }()
	for _, tc := range []struct {
		name, metrics, api, hmac string
		keys                     map[string]string
		bad                      bool
	}{
		{name: "disabled"},
		{name: "distinct", metrics: "metrics", api: "api", hmac: "hmac", keys: map[string]string{"k": "other"}},
		{name: "API key", metrics: "shared", api: "shared", bad: true},
		{name: "single HMAC", metrics: "shared", hmac: "shared", bad: true},
		{name: "rotation key", metrics: "shared", keys: map[string]string{"primary": "other", "rotating": "shared"}, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			MetricsAPIKey, APIKey, HMACSecret, hmacKeysMap = tc.metrics, tc.api, tc.hmac, tc.keys
			problems := strictCredentialProblems()
			if (len(problems) > 0) != tc.bad {
				t.Fatalf("problems=%v", problems)
			}
			if tc.bad {
				if err := validateStrictSettings(); err == nil || !strings.Contains(err.Error(), "must differ") {
					t.Fatalf("strict validation error=%v", err)
				}
			}
		})
	}
}
