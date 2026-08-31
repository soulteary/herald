package config

import (
	"testing"

	logger "github.com/soulteary/logger-kit/v2"
)

// withProdEnv sets production environment and restores all touched globals.
func withProdEnv(t *testing.T) func() {
	t.Helper()
	orig := struct {
		env                    Environment
		apiKey, hmac           string
		idempotency, piiPepper string
		hmacKeysJSON           string
		requestAuthMode        string
		testMode               bool
		providerPolicy         string
		redisPassword          string
		riskAck                bool
		cors                   string
		tlsCert, tlsKey, tlsCA string
		smtpURL                string
	}{
		env:             Env,
		apiKey:          APIKey,
		hmac:            HMACSecret,
		idempotency:     IdempotencySecret,
		piiPepper:       PIIPepper,
		hmacKeysJSON:    HMACKeysJSON,
		requestAuthMode: RequestAuthMode,
		testMode:        TestMode,
		providerPolicy:  ProviderFailurePolicy,
		redisPassword:   RedisPassword,
		riskAck:         RiskAckPasswordlessRedis,
		cors:            CORSAllowOrigins,
		tlsCert:         TLSCertFile,
		tlsKey:          TLSKeyFile,
		tlsCA:           TLSCACertFile,
		smtpURL:         HeraldSMTPAPIURL,
	}
	// Baseline: a valid production config.
	Env = EnvProduction
	APIKey = "prod-api-key-01234567890123456789"
	HMACSecret = ""
	IdempotencySecret = "prod-idempotency-012345678901234"
	PIIPepper = "prod-pii-pepper-012345678901234567"
	RequestAuthMode = "api_key"
	HMACKeysJSON = ""
	hmacKeysMap = nil
	TestMode = false
	ProviderFailurePolicy = "strict"
	RedisPassword = "redis-pass"
	RiskAckPasswordlessRedis = false
	CORSAllowOrigins = ""
	TLSCertFile = ""
	TLSKeyFile = ""
	TLSCACertFile = ""
	HeraldSMTPAPIURL = ""
	log = logger.New(logger.Config{Level: logger.ErrorLevel, Format: logger.FormatJSON})

	return func() {
		Env = orig.env
		APIKey = orig.apiKey
		HMACSecret = orig.hmac
		IdempotencySecret = orig.idempotency
		PIIPepper = orig.piiPepper
		HMACKeysJSON = orig.hmacKeysJSON
		RequestAuthMode = orig.requestAuthMode
		TestMode = orig.testMode
		ProviderFailurePolicy = orig.providerPolicy
		RedisPassword = orig.redisPassword
		RiskAckPasswordlessRedis = orig.riskAck
		CORSAllowOrigins = orig.cors
		TLSCertFile = orig.tlsCert
		TLSKeyFile = orig.tlsKey
		TLSCACertFile = orig.tlsCA
		HeraldSMTPAPIURL = orig.smtpURL
	}
}

func TestValidate_ProductionBaselineOK(t *testing.T) {
	defer withProdEnv(t)()
	if err := Validate(); err != nil {
		t.Fatalf("baseline production config should pass, got: %v", err)
	}
}

func TestValidate_ProductionRefusesNoAuth(t *testing.T) {
	defer withProdEnv(t)()
	APIKey = ""
	HMACSecret = ""
	hmacKeysMap = nil
	if err := Validate(); err == nil {
		t.Fatal("production with no auth must fail Validate()")
	}
}

func TestValidate_ProductionRequiresCredentialForSelectedAuthMode(t *testing.T) {
	t.Run("api_key mode requires API_KEY", func(t *testing.T) {
		defer withProdEnv(t)()
		RequestAuthMode = "api_key"
		APIKey = ""
		HMACSecret = "configured-but-unused"
		if err := Validate(); err == nil {
			t.Fatal("api_key mode without API_KEY must fail even when HMAC is configured")
		}
	})

	t.Run("hmac_v2 mode requires HMAC credential", func(t *testing.T) {
		defer withProdEnv(t)()
		RequestAuthMode = "hmac_v2"
		APIKey = "configured-but-unused"
		HMACSecret = ""
		hmacKeysMap = nil
		if err := Validate(); err == nil {
			t.Fatal("hmac_v2 mode without an HMAC credential must fail even when API_KEY is configured")
		}
	})

	t.Run("hmac_v2 accepts a single secret", func(t *testing.T) {
		defer withProdEnv(t)()
		RequestAuthMode = "hmac_v2"
		APIKey = ""
		HMACSecret = "prod-hmac-secret-01234567890123456"
		if err := Validate(); err != nil {
			t.Fatalf("hmac_v2 with HMAC_SECRET should pass: %v", err)
		}
	})
}

func TestValidate_ProductionRefusesTestMode(t *testing.T) {
	defer withProdEnv(t)()
	TestMode = true
	if err := Validate(); err == nil {
		t.Fatal("production with HERALD_TEST_MODE=true must fail Validate()")
	}
}

func TestValidate_ProductionRefusesNoAuthAliases(t *testing.T) {
	for _, mode := range []string{"none", "off", "disabled", " OFF "} {
		t.Run(mode, func(t *testing.T) {
			defer withProdEnv(t)()
			RequestAuthMode = mode
			if err := Validate(); err == nil {
				t.Fatalf("production with REQUEST_AUTH_MODE=%q must fail Validate()", mode)
			}
		})
	}
}

func TestValidate_ProductionRefusesSoftProviderPolicy(t *testing.T) {
	for _, policy := range []string{"soft", " SOFT "} {
		t.Run(policy, func(t *testing.T) {
			defer withProdEnv(t)()
			ProviderFailurePolicy = policy
			if err := Validate(); err == nil {
				t.Fatalf("production with PROVIDER_FAILURE_POLICY=%q must fail Validate()", policy)
			}
		})
	}
}

func TestValidate_ProductionRefusesPasswordlessRedis(t *testing.T) {
	defer withProdEnv(t)()
	RedisPassword = ""
	RiskAckPasswordlessRedis = false
	if err := Validate(); err == nil {
		t.Fatal("production with passwordless Redis must fail Validate()")
	}
	// With explicit risk ack it should pass.
	RiskAckPasswordlessRedis = true
	if err := Validate(); err != nil {
		t.Fatalf("passwordless Redis with risk ack should pass, got: %v", err)
	}
}

func TestValidate_ProductionRefusesCORSWildcard(t *testing.T) {
	defer withProdEnv(t)()
	CORSAllowOrigins = "*"
	if err := Validate(); err == nil {
		t.Fatal("production with CORS wildcard must fail Validate()")
	}
}

func TestValidate_HalfTLSRefused(t *testing.T) {
	defer withProdEnv(t)()
	TLSCertFile = "/etc/herald/tls.crt"
	TLSKeyFile = ""
	if err := Validate(); err == nil {
		t.Fatal("half-configured TLS (cert without key) must fail Validate()")
	}
	// Also in development, half-TLS is a warning but does not abort.
	Env = EnvDevelopment
	if err := Validate(); err != nil {
		t.Fatalf("half-TLS in development should warn, not fail, got: %v", err)
	}
}

func TestValidate_ClientCAWithoutServerCertRefused(t *testing.T) {
	defer withProdEnv(t)()
	TLSCACertFile = "/etc/herald/ca.crt"
	TLSCertFile = ""
	TLSKeyFile = ""
	if err := Validate(); err == nil {
		t.Fatal("client CA without server cert/key must fail Validate()")
	}
}

func TestValidate_ProductionRefusesPlaintextProviderURL(t *testing.T) {
	defer withProdEnv(t)()
	HeraldSMTPAPIURL = "http://smtp.internal/send"
	if err := Validate(); err == nil {
		t.Fatal("production with plaintext provider URL must fail Validate()")
	}
	HeraldSMTPAPIURL = "https://smtp.internal/send"
	if err := Validate(); err != nil {
		t.Fatalf("https provider URL should pass, got: %v", err)
	}
}

func TestValidate_RefusesUnsupportedLokiAuditStorage(t *testing.T) {
	defer withProdEnv(t)()
	orig := AuditStorageType
	defer func() { AuditStorageType = orig }()

	AuditStorageType = "loki"
	if err := Validate(); err == nil {
		t.Fatal("AUDIT_STORAGE_TYPE=loki has no backend and must fail Validate() in production")
	}
	// Comma-separated list containing loki must also be rejected.
	AuditStorageType = "redis,loki"
	if err := Validate(); err == nil {
		t.Fatal("AUDIT_STORAGE_TYPE containing loki must fail Validate() in production")
	}
	// A supported type must pass.
	AuditStorageType = "redis"
	if err := Validate(); err != nil {
		t.Fatalf("AUDIT_STORAGE_TYPE=redis should pass, got: %v", err)
	}
}

func TestValidate_DevelopmentIsLenient(t *testing.T) {
	defer withProdEnv(t)()
	Env = EnvDevelopment
	APIKey = ""
	HMACSecret = ""
	hmacKeysMap = nil
	TestMode = true
	ProviderFailurePolicy = "soft"
	RedisPassword = ""
	if err := Validate(); err != nil {
		t.Fatalf("development should be lenient, got: %v", err)
	}
}

func TestTestCodeExposureEnabled(t *testing.T) {
	origEnv, origTest := Env, TestMode
	defer func() { Env, TestMode = origEnv, origTest }()

	cases := []struct {
		env  Environment
		test bool
		want bool
	}{
		{EnvProduction, true, false},
		{EnvProduction, false, false},
		{EnvDevelopment, true, false},
		{EnvTest, false, false},
		{EnvTest, true, true},
	}
	for _, c := range cases {
		Env, TestMode = c.env, c.test
		if got := TestCodeExposureEnabled(); got != c.want {
			t.Errorf("TestCodeExposureEnabled(env=%s,test=%v) = %v, want %v", c.env, c.test, got, c.want)
		}
	}
}

func TestParseEnvironment(t *testing.T) {
	cases := map[string]Environment{
		"production":  EnvProduction,
		"prod":        EnvProduction,
		"test":        EnvTest,
		"development": EnvDevelopment,
		"":            EnvDevelopment,
		"garbage":     EnvDevelopment,
	}
	for in, want := range cases {
		if got := parseEnvironment(in); got != want {
			t.Errorf("parseEnvironment(%q) = %v, want %v", in, got, want)
		}
	}
}
