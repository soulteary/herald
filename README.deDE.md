# Herald - OTP- und Verifizierungscode-Service

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![codecov](https://codecov.io/gh/soulteary/herald/branch/main/graph/badge.svg)](https://codecov.io/gh/soulteary/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/herald)](https://goreportcard.com/report/github.com/soulteary/herald)

> **📧 Ihr Gateway zur Sicheren Verifizierung**

## 🌐 Mehrsprachige Dokumentation

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald ist ein produktionsreifer, eigenständiger OTP- und Verifizierungscode-Service, der Verifizierungscodes per E-Mail und SMS sendet. Er verfügt über integriertes Rate-Limiting, Sicherheitskontrollen und Audit-Protokollierung. Herald ist so konzipiert, dass es unabhängig funktioniert und bei Bedarf mit anderen Diensten integriert werden kann.

## Kernfunktionen

- 🔒 **Sicherheitsorientiert**: Challenge-basierte Verifizierung mit Argon2-Hash-Speicherung, mehrere Authentifizierungsmethoden (mTLS, HMAC, API Key)
- 📊 **Integriertes Rate-Limiting**: Mehrdimensionales Rate-Limiting (pro Benutzer, pro IP, pro Ziel) mit konfigurierbaren Schwellenwerten
- 📝 **Vollständige Audit-Spur**: Vollständige Audit-Protokollierung für alle Operationen mit Anbieter-Tracking
- 🔌 **Erweiterbare Anbieter**: Erweiterbare E-Mail- und SMS-Anbieter-Architektur

## Schnellstart

### Mit Docker Compose

Der einfachste Weg, um zu beginnen, ist mit Docker Compose, das Redis enthält:

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

Erwartete Antwort:
```json
{
  "status": "ok",
  "service": "herald"
}
```

### API testen

Erstellen Sie eine Test-Challenge (erfordert Authentifizierung - siehe [API-Dokumentation](docs/deDE/API.md)):

```bash
# Set your API key (from docker-compose.yml: your-secret-api-key-here)
export API_KEY="your-secret-api-key-here"

# Create a challenge
curl -X POST http://localhost:8082/v1/otp/challenges \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "channel": "email",
    "destination": "user@example.com",
    "purpose": "login"
  }'
```

### Logs anzeigen

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### Manuelle Bereitstellung

Für manuelle Bereitstellung und erweiterte Konfiguration siehe [Bereitstellungsanleitung](docs/deDE/DEPLOYMENT.md).

## Grundkonfiguration

Herald benötigt minimale Konfiguration, um zu beginnen:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

Für vollständige Konfigurationsoptionen einschließlich Rate-Limits, Challenge-Ablaufzeit und Anbieter-Einstellungen siehe [Bereitstellungsanleitung](docs/deDE/DEPLOYMENT.md#configuration).

## Dokumentation

### Für Entwickler

- **[API-Dokumentation](docs/deDE/API.md)** - Vollständige API-Referenz mit Authentifizierungsmethoden, Endpunkten und Fehlercodes
- **[Bereitstellungsanleitung](docs/deDE/DEPLOYMENT.md)** - Konfigurationsoptionen, Docker-Bereitstellung und Integrationsbeispiele

### Für Betrieb

- **[Überwachungsanleitung](docs/deDE/MONITORING.md)** - Prometheus-Metriken, Grafana-Dashboards und Alerting
- **[Fehlerbehebungsanleitung](docs/deDE/TROUBLESHOOTING.md)** - Häufige Probleme, Diagnoseschritte und Lösungen

### Dokumentationsindex

Für einen vollständigen Überblick über alle Dokumentationen siehe [docs/deDE/README.md](docs/deDE/README.md).

## License

See [LICENSE](LICENSE) for details.
