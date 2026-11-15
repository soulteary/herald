package handlers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/soulteary/herald/internal/config"
)

// configSetting is one exported variable of the config package: how to read
// it for comparison and how to put a captured value back.
type configSetting struct {
	name    string
	capture func() (value string, restore func())
}

func setting[T any](name string, variable *T) configSetting {
	return configSetting{name: name, capture: func() (string, func()) {
		value := *variable
		return fmt.Sprintf("%#v", value), func() { *variable = value }
	}}
}

// listSetting copies the slice, so a test that changes an element in place
// does not change the captured value with it.
func listSetting(name string, variable *[]string) configSetting {
	return configSetting{name: name, capture: func() (string, func()) {
		value := *variable
		if value != nil {
			value = append([]string{}, value...)
		}
		return fmt.Sprintf("%#v", value), func() { *variable = value }
	}}
}

// configSettings lists every exported variable of the config package.
// TestConfigSettingsListIsComplete fails when one is missing.
var configSettings = []configSetting{
	setting("APIKey", &config.APIKey),
	listSetting("AllowedPurposes", &config.AllowedPurposes),
	setting("AuditDatabaseURL", &config.AuditDatabaseURL),
	setting("AuditEnabled", &config.AuditEnabled),
	setting("AuditFilePath", &config.AuditFilePath),
	setting("AuditMaskDestination", &config.AuditMaskDestination),
	setting("AuditSpoolDir", &config.AuditSpoolDir),
	setting("AuditSpoolMaxBytes", &config.AuditSpoolMaxBytes),
	setting("AuditStorageType", &config.AuditStorageType),
	setting("AuditTTL", &config.AuditTTL),
	setting("AuditTableName", &config.AuditTableName),
	setting("AuditWriterQueueSize", &config.AuditWriterQueueSize),
	setting("AuditWriterWorkers", &config.AuditWriterWorkers),
	setting("CORSAllowOrigins", &config.CORSAllowOrigins),
	setting("ChallengeExpiry", &config.ChallengeExpiry),
	setting("ClientCertMode", &config.ClientCertMode),
	setting("CodeHashKey", &config.CodeHashKey),
	setting("CodeHashVerifyOnly", &config.CodeHashVerifyOnly),
	setting("CodeHashKeyPrevious", &config.CodeHashKeyPrevious),
	setting("CodeLength", &config.CodeLength),
	setting("Env", &config.Env),
	setting("EnvironmentValue", &config.EnvironmentValue),
	setting("FailedAttemptLockThreshold", &config.FailedAttemptLockThreshold),
	setting("FailedAttemptLockWindow", &config.FailedAttemptLockWindow),
	setting("HMACDefaultKeyID", &config.HMACDefaultKeyID),
	setting("HMACKeysJSON", &config.HMACKeysJSON),
	setting("HMACMaxDrift", &config.HMACMaxDrift),
	setting("HMACSecret", &config.HMACSecret),
	setting("HMACV1Enabled", &config.HMACV1Enabled),
	setting("HealthcheckTLSCAFile", &config.HealthcheckTLSCAFile),
	setting("HealthcheckTLSClientCertFile", &config.HealthcheckTLSClientCertFile),
	setting("HealthcheckTLSClientKeyFile", &config.HealthcheckTLSClientKeyFile),
	setting("HealthcheckTLSServerName", &config.HealthcheckTLSServerName),
	setting("HeraldDingtalkAPIKey", &config.HeraldDingtalkAPIKey),
	setting("HeraldDingtalkAPIURL", &config.HeraldDingtalkAPIURL),
	setting("HeraldSMTPAPIKey", &config.HeraldSMTPAPIKey),
	setting("HeraldSMTPAPIURL", &config.HeraldSMTPAPIURL),
	setting("IdempotencyKeyTTL", &config.IdempotencyKeyTTL),
	setting("IdempotencySecret", &config.IdempotencySecret),
	setting("IdempotencyUnresolvedTTL", &config.IdempotencyUnresolvedTTL),
	setting("IdleTimeout", &config.IdleTimeout),
	setting("LockoutChallengeThreshold", &config.LockoutChallengeThreshold),
	setting("LockoutDuration", &config.LockoutDuration),
	setting("LogLevel", &config.LogLevel),
	setting("MaxAttempts", &config.MaxAttempts),
	setting("MaxBodyBytes", &config.MaxBodyBytes),
	setting("MaxConcurrentHashOperations", &config.MaxConcurrentHashOperations),
	setting("MetricsAPIKey", &config.MetricsAPIKey),
	setting("NonceStorePrefix", &config.NonceStorePrefix),
	setting("OTLPEnabled", &config.OTLPEnabled),
	setting("OTLPEndpoint", &config.OTLPEndpoint),
	setting("PIIPepper", &config.PIIPepper),
	setting("Port", &config.Port),
	setting("ProviderFailurePolicy", &config.ProviderFailurePolicy),
	setting("ProviderMaxResponseBytes", &config.ProviderMaxResponseBytes),
	setting("ProviderRedirectPolicy", &config.ProviderRedirectPolicy),
	setting("ProviderTimeout", &config.ProviderTimeout),
	setting("RateLimitPerDestination", &config.RateLimitPerDestination),
	setting("RateLimitPerIP", &config.RateLimitPerIP),
	setting("RateLimitPerUser", &config.RateLimitPerUser),
	setting("ReadTimeout", &config.ReadTimeout),
	setting("RedisAddr", &config.RedisAddr),
	setting("RedisDB", &config.RedisDB),
	setting("RedisPassword", &config.RedisPassword),
	setting("RedisTLSCAFile", &config.RedisTLSCAFile),
	setting("RedisTLSEnabled", &config.RedisTLSEnabled),
	setting("RedisTLSServerName", &config.RedisTLSServerName),
	setting("RedisUsername", &config.RedisUsername),
	setting("RequestAuthMode", &config.RequestAuthMode),
	setting("ResendCooldown", &config.ResendCooldown),
	setting("RiskAckPasswordlessRedis", &config.RiskAckPasswordlessRedis),
	setting("SMSAPIBaseURL", &config.SMSAPIBaseURL),
	setting("SMSAPIKey", &config.SMSAPIKey),
	setting("SMSProvider", &config.SMSProvider),
	setting("SMTPFrom", &config.SMTPFrom),
	setting("SMTPHost", &config.SMTPHost),
	setting("SMTPPassword", &config.SMTPPassword),
	setting("SMTPPort", &config.SMTPPort),
	setting("SMTPUser", &config.SMTPUser),
	setting("ServiceName", &config.ServiceName),
	setting("ServicePoliciesJSON", &config.ServicePoliciesJSON),
	setting("TLSCACertFile", &config.TLSCACertFile),
	setting("TLSCertFile", &config.TLSCertFile),
	setting("TLSClientCAFile", &config.TLSClientCAFile),
	setting("TLSKeyFile", &config.TLSKeyFile),
	setting("TOTPAPIKey", &config.TOTPAPIKey),
	setting("TOTPBaseURL", &config.TOTPBaseURL),
	setting("TOTPEnabled", &config.TOTPEnabled),
	setting("TOTPHMACKeyID", &config.TOTPHMACKeyID),
	setting("TOTPHMACSecret", &config.TOTPHMACSecret),
	setting("TOTPHMACService", &config.TOTPHMACService),
	setting("TemplateDir", &config.TemplateDir),
	setting("TestAPIKey", &config.TestAPIKey),
	setting("TestListenerAddr", &config.TestListenerAddr),
	setting("TestMode", &config.TestMode),
	listSetting("TrustedProxies", &config.TrustedProxies),
	setting("TrustedProxyHeader", &config.TrustedProxyHeader),
	setting("VerifyV1Enabled", &config.VerifyV1Enabled),
	setting("WriteTimeout", &config.WriteTimeout),
}

// globalState is the package-level state that the tests of this package
// change: the handlers keep none of their own, and read the variables of the
// config package when a Handlers value is built and on every request.
type globalState struct {
	values   map[string]string
	restores []func()
}

func captureGlobalState() globalState {
	state := globalState{values: make(map[string]string, len(configSettings))}
	for _, s := range configSettings {
		value, restore := s.capture()
		state.values[s.name] = value
		state.restores = append(state.restores, restore)
	}
	return state
}

func (state globalState) restore() {
	for _, restore := range state.restores {
		restore()
	}
}

// isolateGlobalState puts the configuration back when the test ends. Call it
// before the first assignment to a config variable. Cleanups run last in,
// first out, so the configuration comes back after the cleanups the test
// registers later have run, for example the one that stops an audit writer.
//
// A test must not rely on a value an earlier test assigned: it gets the
// configuration the package starts with, which is the environment defaults
// with audit logging off (see TestMain).
func isolateGlobalState(t testing.TB) {
	t.Helper()
	state := captureGlobalState()
	t.Cleanup(state.restore)
}

// globalStateChanges names the settings that differ between two states, in a
// fixed order.
func globalStateChanges(before, after globalState) []string {
	var changes []string
	for name, value := range after.values {
		if before.values[name] != value {
			changes = append(changes, fmt.Sprintf("config.%s: %s, was %s", name, value, before.values[name]))
		}
	}
	sort.Strings(changes)
	return changes
}

func TestIsolateGlobalStateRestoresConfiguration(t *testing.T) {
	before := captureGlobalState()
	t.Run("changes", func(t *testing.T) {
		isolateGlobalState(t)
		config.SMTPHost = "leaked.example.com"
		config.RateLimitPerIP++
		config.TestMode = !config.TestMode
		config.AllowedPurposes = append(config.AllowedPurposes, "leaked")
		if changes := globalStateChanges(before, captureGlobalState()); len(changes) != 4 {
			t.Fatalf("changes inside the test = %v, want the four assignments", changes)
		}
	})
	if changes := globalStateChanges(before, captureGlobalState()); len(changes) != 0 {
		t.Fatalf("state after the test differs: %v", changes)
	}
}

func TestListSettingIsNotChangedInPlace(t *testing.T) {
	isolateGlobalState(t)
	config.AllowedPurposes = []string{"login", "reset"}
	state := captureGlobalState()
	config.AllowedPurposes[0] = "changed"
	state.restore()
	if got := strings.Join(config.AllowedPurposes, ","); got != "login,reset" {
		t.Fatalf("AllowedPurposes after restore = %q, want login,reset", got)
	}
}

// TestConfigSettingsListIsComplete reads the config package's source and
// requires every exported package-level variable to be in configSettings, so
// a setting added later is restored and compared like the others.
func TestConfigSettingsListIsComplete(t *testing.T) {
	listed := make(map[string]bool, len(configSettings))
	for _, s := range configSettings {
		if listed[s.name] {
			t.Errorf("config.%s is listed twice", s.name)
		}
		listed[s.name] = true
	}

	files, err := filepath.Glob("../config/*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("find the config package's source: %v (%d files)", err, len(files))
	}
	declared := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					if !name.IsExported() {
						continue
					}
					declared++
					if !listed[name.Name] {
						t.Errorf("config.%s is not in configSettings", name.Name)
					}
				}
			}
		}
	}
	if declared != len(configSettings) {
		t.Errorf("config declares %d exported variables, configSettings lists %d", declared, len(configSettings))
	}
}
