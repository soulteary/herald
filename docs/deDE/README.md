# Dokumentationsverzeichnis

> **Veraltete Übersetzung / Historical translation.** This page predates current security changes.
> Do not use its commands or configuration as current instructions. See the
> [current English guide](../enUS/README.md) or [中文](../zhCN/README.md)
> and [translation status](STALE.md). HMAC creation requires v3; builds and validation use Linux.

Willkommen zur Dokumentation des Herald OTP- und Verifizierungscode-Services.

## 🌐 Mehrsprachige Dokumentation

- [English](../enUS/README.md) | [中文](../zhCN/README.md) | [Français](../frFR/README.md) | [Italiano](../itIT/README.md) | [日本語](../jaJP/README.md) | [Deutsch](README.md) | [한국어](../koKR/README.md)

## 📚 Dokumentenliste

### Kerndokumente

- **[README.md](../../README.deDE.md)** - Projektübersicht und Schnellstartanleitung

### Detaillierte Dokumente

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Full-Stack-Architektur
  - Stargate / Warden / Herald / Redis / Provider
  - Datenfluss und zentrale Schnittstellen (Challenge, Verify, Provider-Send)
  - Sicherheitsgrenzen (HMAC/mTLS, PII)

- **[API.md](API.md)** - Vollständige API-Endpunkt-Dokumentation
  - Authentifizierungsmethoden
  - Health-Check-Endpunkte
  - Challenge-Erstellung und -Verifizierung
  - Rate-Limiting
  - Fehlercodes und Antworten

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Bereitstellungsanleitung
  - Docker Compose-Bereitstellung
  - Manuelle Bereitstellung
  - Konfigurationsoptionen
  - Optionale Integration mit anderen Diensten
  - Sicherheitsbest Practices

- **[MONITORING.md](MONITORING.md)** - Monitoring-Leitfaden
  - Prometheus-Metriken
  - Grafana-Dashboards
  - Alerting-Regeln
  - Best Practices

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Fehlerbehebungsleitfaden
  - Häufige Probleme und Lösungen
  - Diagnoseschritte
  - Leistungsoptimierung

## 🚀 Schnellnavigation

### Erste Schritte

1. Lesen Sie [README.deDE.md](../../README.deDE.md), um das Projekt zu verstehen
2. Überprüfen Sie den Abschnitt [Schnellstart](../../README.deDE.md#schnellstart)
3. Beziehen Sie sich auf [Konfiguration](../../README.deDE.md#grundkonfiguration), um den Service zu konfigurieren

### Entwickler

1. Lesen Sie [ARCHITECTURE.md](ARCHITECTURE.md) für die Full-Stack-Übersicht
2. Überprüfen Sie [API.md](API.md), um die API-Schnittstellen zu verstehen
3. Prüfen Sie [DEPLOYMENT.md](DEPLOYMENT.md) für Bereitstellungsoptionen

### Betrieb

1. Lesen Sie [DEPLOYMENT.md](DEPLOYMENT.md), um Bereitstellungsmethoden zu verstehen
2. Überprüfen Sie [API.md](API.md) für API-Endpunkt-Details
3. Beziehen Sie sich auf [Sicherheit](DEPLOYMENT.md#sicherheit) für Sicherheitsbest Practices
4. Service-Gesundheit überwachen: [MONITORING.md](MONITORING.md)
5. Probleme beheben: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 Dokumentenstruktur

```
herald/
├── README.md              # Hauptprojektdokument (Englisch)
├── README.deDE.md         # Hauptprojektdokument (Deutsch)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # Dokumentationsverzeichnis (Englisch)
│   │   ├── ARCHITECTURE.md # Full-Stack-Architektur (Englisch)
│   │   ├── API.md          # API-Dokument (Englisch)
│   │   ├── DEPLOYMENT.md   # Bereitstellungsanleitung (Englisch)
│   │   ├── MONITORING.md   # Monitoring-Leitfaden (Englisch)
│   │   └── TROUBLESHOOTING.md # Fehlerbehebungsleitfaden (Englisch)
│   └── deDE/
│       ├── README.md       # Dokumentationsverzeichnis (Deutsch, diese Datei)
│       ├── ARCHITECTURE.md # Full-Stack-Architektur (Deutsch)
│       ├── API.md          # API-Dokument (Deutsch)
│       ├── DEPLOYMENT.md   # Bereitstellungsanleitung (Deutsch)
│       ├── MONITORING.md   # Monitoring-Leitfaden (Deutsch)
│       └── TROUBLESHOOTING.md # Fehlerbehebungsleitfaden (Deutsch)
└── ...
```

## 🔍 Nach Thema Suchen

### API-bezogen

- API-Endpunktliste: [API.md](API.md)
- Authentifizierungsmethoden: [API.md#authentifizierung](API.md#authentifizierung)
- Fehlerbehandlung: [API.md#fehlercodes](API.md#fehlercodes)
- Rate-Limiting: [API.md#rate-limiting](API.md#rate-limiting)

### Bereitstellungsbezogen

- Docker-Bereitstellung: [DEPLOYMENT.md#schnellstart](DEPLOYMENT.md#schnellstart)
- Konfigurationsoptionen: [DEPLOYMENT.md#konfiguration](DEPLOYMENT.md#konfiguration)
- Dienstintegration: [DEPLOYMENT.md#integration-mit-anderen-diensten-optional](DEPLOYMENT.md#integration-mit-anderen-diensten-optional)
- Sicherheit: [DEPLOYMENT.md#sicherheit](DEPLOYMENT.md#sicherheit)

### Monitoring und Betrieb

- Prometheus-Metriken: [MONITORING.md](MONITORING.md)
- Grafana-Dashboards: [MONITORING.md#grafana-dashboards](MONITORING.md#grafana-dashboards)
- Fehlerbehebung: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 Verwendungsempfehlungen

1. **Erstmalige Benutzer**: Beginnen Sie mit [README.deDE.md](../../README.deDE.md) und folgen Sie der Schnellstartanleitung
2. **Service konfigurieren**: Beziehen Sie sich auf [DEPLOYMENT.md](DEPLOYMENT.md), um alle Konfigurationsoptionen zu verstehen
3. **Mit Services integrieren**: Überprüfen Sie den Integrationsabschnitt in [DEPLOYMENT.md](DEPLOYMENT.md)
4. **API-Integration**: Lesen Sie [API.md](API.md), um die API-Schnittstellen zu verstehen
5. **Service überwachen**: Richten Sie Monitoring mit [MONITORING.md](MONITORING.md) ein
6. **Probleme beheben**: Beziehen Sie sich auf [TROUBLESHOOTING.md](TROUBLESHOOTING.md) für häufige Probleme

## 📝 Dokumentationsaktualisierungen

Die Dokumentation wird kontinuierlich aktualisiert, während sich das Projekt weiterentwickelt. Wenn Sie Fehler finden oder Ergänzungen benötigen, senden Sie bitte ein Issue oder einen Pull Request.

## 🤝 Beitragen

Verbesserungen der Dokumentation sind willkommen:

1. Finden Sie Fehler oder Bereiche, die verbessert werden müssen
2. Senden Sie ein Issue, das das Problem beschreibt
3. Oder senden Sie direkt einen Pull Request
