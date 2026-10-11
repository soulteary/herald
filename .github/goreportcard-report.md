# Go Report Card

**Grade: A+** (97.9%)

| Metric | Value |
| ------ | ----- |
| Files | 51 |
| Issues | 30 |

## Checks

| Check | Score |
| ----- | ----- |
| gofmt | 100% |
| go_vet | 100% |
| gocyclo | 82% |
| ineffassign | 100% |
| license | 100% |
| misspell | 58% |

## Issues

### gocyclo

- `internal/handlers/handlers.go`
  - Line 236: cyclomatic complexity 66 for function (*Handlers).CreateChallenge
  - Line 759: cyclomatic complexity 24 for function (*Handlers).VerifyChallenge
  - Line 903: cyclomatic complexity 21 for function (*Handlers).VerifyChallengeV2
  - Line 89: cyclomatic complexity 19 for function NewHandlersWithError
- `internal/config/strict.go`
  - Line 34: cyclomatic complexity 36 for function validateStrictSettings
- `internal/config/config.go`
  - Line 325: cyclomatic complexity 35 for function Validate
  - Line 225: cyclomatic complexity 21 for function Initialize
- `pkg/herald/client_totp_test.go`
  - Line 21: cyclomatic complexity 26 for function TestTOTPClientHappyPaths
- `internal/tracing/middleware_test.go`
  - Line 10: cyclomatic complexity 19 for function TestTracingMiddleware
- `internal/auth/middleware.go`
  - Line 118: cyclomatic complexity 19 for function verifyHMACV2
- `internal/handlers/handlers_test.go`
  - Line 1595: cyclomatic complexity 18 for function TestHandlers_VerifyChallenge_FailureReasons
- `internal/destination/destination.go`
  - Line 67: cyclomatic complexity 18 for function Validate
- `internal/server/server.go`
  - Line 60: cyclomatic complexity 17 for function New

### misspell

- `README.deDE.md`
  - Line 76: "Konfiguration" is a misspelling of "Configuration"
  - Line 80: "Konfiguration" is a misspelling of "Configuration"
- `docs/frFR/CONTRIBUTING.md`
  - Line 9: "Contribuer" is a misspelling of "Contribute"
  - Line 11: "contribuer" is a misspelling of "contribute"
- `docs/frFR/API.md`
  - Line 32: "exemple" is a misspelling of "example"
  - Line 40: "Terminaison" is a misspelling of "Termination"
- `docs/frFR/DEPLOYMENT.md`
  - Line 3: "marrage" is a misspelling of "marriage"
  - Line 30: "exemple" is a misspelling of "example"
  - Line 81: "Exemple" is a misspelling of "Example"
  - Line 101: "activit" is a misspelling of "activist"
  - Line 101: "suspectes" is a misspelling of "suspects"
- `docs/frFR/TROUBLESHOOTING.md`
  - Line 52: "terminaison" is a misspelling of "termination"
  - Line 139: "certificats" is a misspelling of "certificates"
  - Line 153: "certificats" is a misspelling of "certificates"
  - Line 331: "trafic" is a misspelling of "traffic"
  - Line 348: "Distribuer" is a misspelling of "Distribute"
  - Line 365: "terminaison" is a misspelling of "termination"
- `docs/deDE/README.md`
  - Line 53: "Konfiguration" is a misspelling of "Configuration"
  - Line 53: "konfiguration" is a misspelling of "configuration"
  - Line 105: "konfiguration" is a misspelling of "configuration"
  - Line 105: "konfiguration" is a misspelling of "configuration"
- `docs/deDE/TROUBLESHOOTING.md`
  - Line 29: "Konfiguration" is a misspelling of "Configuration"
  - Line 31: "Konfiguration" is a misspelling of "Configuration"
  - Line 77: "Konfiguration" is a misspelling of "Configuration"
  - Line 82: "Konfiguration" is a misspelling of "Configuration"
  - Line 104: "Konfiguration" is a misspelling of "Configuration"
  - Line 164: "Konfiguration" is a misspelling of "Configuration"
  - Line 219: "Konfiguration" is a misspelling of "Configuration"
  - Line 221: "Konfiguration" is a misspelling of "Configuration"
  - Line 297: "Konfiguration" is a misspelling of "Configuration"
  - Line 359: "Konfiguration" is a misspelling of "Configuration"
  - Line 360: "Reproduktion" is a misspelling of "Reproduction"
  - Line 366: "Konfiguration" is a misspelling of "Configuration"
- `docs/frFR/README.md`
  - Line 13: "marrage" is a misspelling of "marriage"
  - Line 22: "terminaison" is a misspelling of "termination"
  - Line 24: "terminaison" is a misspelling of "termination"
  - Line 52: "marrage" is a misspelling of "marriage"
  - Line 52: "marrage" is a misspelling of "marriage"
  - Line 64: "terminaison" is a misspelling of "termination"
  - Line 97: "terminaison" is a misspelling of "termination"
  - Line 104: "marrage" is a misspelling of "marriage"
  - Line 104: "marrage" is a misspelling of "marriage"
  - Line 115: "Recommandations" is a misspelling of "Recommendations"
  - Line 117: "marrage" is a misspelling of "marriage"
- `docs/itIT/ARCHITECTURE.md`
  - Line 53: "interfacce" is a misspelling of "interfaces"
  - Line 59: "integrato" is a misspelling of "integration"
  - Line 65: "Interfacce" is a misspelling of "Interfaces"
  - Line 73: "identificativo" is a misspelling of "identification"
- `docs/itIT/CONTRIBUTING.md`
  - Line 24: "contributo" is a misspelling of "contribution"
- `docs/itIT/README.md`
  - Line 19: "interfacce" is a misspelling of "interfaces"
  - Line 53: "configurare" is a misspelling of "configure"
  - Line 58: "interfacce" is a misspelling of "interfaces"
  - Line 118: "Configurare" is a misspelling of "Configure"
  - Line 120: "interfacce" is a misspelling of "interfaces"
- `README.frFR.md`
  - Line 18: "Principales" is a misspelling of "Principles"
  - Line 25: "marrage" is a misspelling of "marriage"
  - Line 94: "terminaison" is a misspelling of "termination"
- `README.itIT.md`
  - Line 16: "integrato" is a misspelling of "integration"
- `docs/deDE/ARCHITECTURE.md`
  - Line 90: "Konfiguration" is a misspelling of "Configuration"
- `docs/deDE/DEPLOYMENT.md`
  - Line 22: "Konfiguration" is a misspelling of "Configuration"
  - Line 63: "Produktion" is a misspelling of "Production"
  - Line 97: "Produktion" is a misspelling of "Production"
  - Line 99: "Produktion" is a misspelling of "Production"
- `docs/deDE/MONITORING.md`
  - Line 118: "Konfiguration" is a misspelling of "Configuration"
  - Line 120: "Konfiguration" is a misspelling of "Configuration"
  - Line 296: "Konfiguration" is a misspelling of "Configuration"
  - Line 309: "Konfiguration" is a misspelling of "Configuration"
- `docs/frFR/MONITORING.md`
  - Line 5: "Terminaison" is a misspelling of "Termination"
  - Line 7: "terminaison" is a misspelling of "termination"
  - Line 26: "Exemple" is a misspelling of "Example"
  - Line 43: "Exemple" is a misspelling of "Example"
  - Line 58: "Exemple" is a misspelling of "Example"
  - Line 75: "Exemple" is a misspelling of "Example"
  - Line 92: "Exemple" is a misspelling of "Example"
  - Line 111: "Exemple" is a misspelling of "Example"
  - Line 308: "terminaison" is a misspelling of "termination"
- `docs/itIT/DEPLOYMENT.md`
  - Line 15: "Compilare" is a misspelling of "Compiler"
  - Line 47: "integrato" is a misspelling of "integration"
  - Line 57: "integrato" is a misspelling of "integration"
  - Line 67: "integrato" is a misspelling of "integration"
  - Line 67: "configurato" is a misspelling of "configuration"
  - Line 71: "configurato" is a misspelling of "configuration"
  - Line 79: "integrato" is a misspelling of "integration"
  - Line 79: "possibile" is a misspelling of "possible"
  - Line 79: "configurare" is a misspelling of "configure"
  - Line 83: "accessibile" is a misspelling of "accessible"
  - Line 100: "Configurare" is a misspelling of "Configure"
  - Line 100: "appropriato" is a misspelling of "appropriation"
- `docs/itIT/MONITORING.md`
  - Line 3: "configurare" is a misspelling of "configure"
  - Line 277: "correnti" is a misspelling of "correcting"
  - Line 301: "Configurare" is a misspelling of "Configure"
  - Line 301: "Configurare" is a misspelling of "Configure"
- `docs/itIT/API.md`
  - Line 15: "certificato" is a misspelling of "certification"
  - Line 21: "certificato" is a misspelling of "certification"
  - Line 21: "verificato" is a misspelling of "verification"
- `docs/itIT/TROUBLESHOOTING.md`
  - Line 30: "integrato" is a misspelling of "integration"
  - Line 122: "configurato" is a misspelling of "configuration"
  - Line 138: "certificato" is a misspelling of "certification"
  - Line 153: "Certificato" is a misspelling of "Certification"
  - Line 253: "accessibile" is a misspelling of "accessible"

---

_Generated by [Go Report Card](https://github.com/soulteary/goreportcard-action)._
