# Herald - OTP- und Verifizierungscode-Service

> **📧 Ihr Gateway zur Sicheren Verifizierung**

## 🌐 Mehrsprachige Dokumentation

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald ist ein produktionsreifer, leichtgewichtiger Service zum Senden von Verifizierungscodes (OTP) per E-Mail (SMS-Unterstützung befindet sich derzeit in der Entwicklung) mit integrierter Rate-Limiting, Sicherheitskontrollen und Audit-Protokollierung.

## Funktionen

- 🚀 **Hohe Leistung** : Erstellt mit Go und Fiber
- 🔒 **Sicher** : Challenge-basierte Verifizierung mit Hash-Speicherung
- 📊 **Rate-Limiting** : Mehrdimensionales Rate-Limiting (pro Benutzer, pro IP, pro Ziel)
- 📝 **Audit-Protokollierung** : Vollständige Audit-Spur für alle Operationen
- 🔌 **Erweiterbare Anbieter** : Unterstützung für E-Mail-Anbieter (SMS-Anbieter sind Platzhalter-Implementierungen und noch nicht vollständig funktionsfähig)
- ⚡ **Redis-Backend** : Schneller, verteilter Speicher mit Redis

## Schnellstart

```bash
# Mit Docker Compose ausführen
docker-compose up -d

# Oder direkt ausführen
go run main.go
```

## Konfiguration

Umgebungsvariablen setzen :

- `PORT` : Server-Port (Standard : `:8082`)
- `REDIS_ADDR` : Redis-Adresse (Standard : `localhost:6379`)
- `REDIS_PASSWORD` : Redis-Passwort (optional)
- `REDIS_DB` : Redis-Datenbanknummer (Standard : `0`)
- `API_KEY` : API-Schlüssel für Service-zu-Service-Authentifizierung
- `LOG_LEVEL` : Protokollierungsstufe (Standard : `info`)

Für vollständige Konfigurationsoptionen siehe [DEPLOYMENT.md](docs/deDE/DEPLOYMENT.md).

## API-Dokumentation

Siehe [API.md](docs/deDE/API.md) für detaillierte API-Dokumentation.
