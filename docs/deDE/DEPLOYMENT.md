# Herald Bereitstellungsanleitung

## Schnellstart

### Verwendung von Docker Compose

```bash
cd herald
docker-compose up -d
```

### Manuelle Bereitstellung

```bash
# Erstellen
go build -o herald main.go

# Ausführen
./herald
```

## Konfiguration

### Umgebungsvariablen

**Vollständige Liste (mit Code abgeglichen):** [Englisch](enUS/DEPLOYMENT.md#environment-variables) | [中文](zhCN/DEPLOYMENT.md#环境变量)

| Variable | Beschreibung | Standard | Erforderlich |
|----------|-------------|----------|--------------|
| `PORT` | Server-Port (kann mit oder ohne führenden Doppelpunkt sein, z. B. `8082` oder `:8082`) | `:8082` | Nein |
| `REDIS_ADDR` | Redis-Adresse | `localhost:6379` | Nein |
| `REDIS_PASSWORD` | Redis-Passwort | `` | Nein |
| `REDIS_DB` | Redis-Datenbank | `0` | Nein |
| `API_KEY` | API-Schlüssel für die Authentifizierung | `` | Empfohlen |
| `HMAC_SECRET` | HMAC-Geheimnis für sichere Authentifizierung | `` | Optional |
| `HERALD_HMAC_KEYS` | Mehrere HMAC-Keys (JSON) | `` | Optional |
| `LOG_LEVEL` | Protokollierungsstufe | `info` | Nein |
| `CHALLENGE_EXPIRY` | Challenge-Ablaufzeit | `5m` | Nein |
| `MAX_ATTEMPTS` | Maximale Verifizierungsversuche | `5` | Nein |
| `RESEND_COOLDOWN` | Wartezeit für erneutes Senden | `60s` | Nein |
| `CODE_LENGTH` | Verifizierungscode-Länge | `6` | Nein |
| `RATE_LIMIT_PER_USER` | Rate-Limit pro Benutzer/Stunde | `10` | Nein |
| `RATE_LIMIT_PER_IP` | Rate-Limit pro IP/Minute | `5` | Nein |
| `RATE_LIMIT_PER_DESTINATION` | Rate-Limit pro Ziel/Stunde | `10` | Nein |
| `LOCKOUT_DURATION` | Benutzer-Sperrdauer nach maximalen Versuchen | `10m` | Nein |
| `SERVICE_NAME` | Service-Identifikator für HMAC-Authentifizierung | `herald` | Nein |
| `SMTP_HOST` | SMTP-Server-Host | `` | Für E-Mail (eingebaut) |
| `SMTP_PORT` | SMTP-Server-Port | `587` | Für E-Mail |
| `SMTP_USER` | SMTP-Benutzername | `` | Für E-Mail |
| `SMTP_PASSWORD` | SMTP-Passwort | `` | Für E-Mail |
| `SMTP_FROM` | SMTP-Absenderadresse | `` | Für E-Mail |
| `SMS_PROVIDER` | SMS-Anbieter (z. B. Name für Logs) | `` | Für SMS |
| `SMS_API_BASE_URL` | SMS-HTTP-API Basis-URL | `` | Für SMS (HTTP-API) |
| `SMS_API_KEY` | SMS-API-Schlüssel | `` | Für SMS (optional) |
| `HERALD_DINGTALK_API_URL` | Basis-URL von [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (z. B. `http://herald-dingtalk:8083`) | `` | Für DingTalk-Kanal |
| `HERALD_DINGTALK_API_KEY` | Optionaler API-Schlüssel; muss mit herald-dingtalk `API_KEY` übereinstimmen, falls gesetzt | `` | Nein |
| `HERALD_SMTP_API_URL` | Basis-URL von [herald-smtp](https://github.com/soulteary/herald-smtp) (z. B. `http://herald-smtp:8084`); wenn gesetzt, wird eingebautes SMTP nicht verwendet | `` | Für E-Mail-Kanal (optional) |
| `HERALD_SMTP_API_KEY` | Optionaler API-Schlüssel; muss mit herald-smtp `API_KEY` übereinstimmen, falls gesetzt | `` | Nein |
| `HERALD_TEST_MODE` | Wenn `true`: Debug-Code in Redis/Response. **Nur für Tests; in Produktion immer `false`.** | `false` | Nein |

### E-Mail-Kanal (herald-smtp)

Wenn `HERALD_SMTP_API_URL` gesetzt ist, verwendet Herald kein eingebautes SMTP. Der E-Mail-Versand wird per HTTP an [herald-smtp](https://github.com/soulteary/herald-smtp) weitergeleitet. Alle SMTP-Zugangsdaten und -logik liegen in herald-smtp; Herald speichert in diesem Modus keine SMTP-Zugangsdaten für den E-Mail-Kanal. Setzen Sie `HERALD_SMTP_API_URL` auf die Basis-URL Ihres herald-smtp-Dienstes. Wenn herald-smtp mit `API_KEY` konfiguriert ist, setzen Sie `HERALD_SMTP_API_KEY` auf denselben Wert. Wenn `HERALD_SMTP_API_URL` gesetzt ist, ignoriert Herald `SMTP_HOST` und zugehörige eingebaute SMTP-Einstellungen.

### DingTalk-Kanal (herald-dingtalk)

Wenn `channel` `dingtalk` ist, sendet Herald keine Nachrichten selbst, sondern leitet den Versand per HTTP an [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) weiter. Alle DingTalk-Zugangsdaten und -logik liegen in herald-dingtalk; Herald speichert keine DingTalk-Zugangsdaten. Setzen Sie `HERALD_DINGTALK_API_URL` auf die Basis-URL Ihres herald-dingtalk-Dienstes. Wenn herald-dingtalk mit `API_KEY` konfiguriert ist, setzen Sie `HERALD_DINGTALK_API_KEY` auf denselben Wert.

## Integration mit anderen Diensten (Optional)

Herald ist so konzipiert, dass es unabhängig funktioniert und bei Bedarf mit anderen Diensten integriert werden kann. Wenn Sie Herald mit anderen Authentifizierungs- oder Gateway-Diensten integrieren möchten, können Sie Folgendes konfigurieren:

**Beispiel-Integrationskonfiguration:**
```bash
# Service-URL, unter der Herald erreichbar ist
export HERALD_URL=http://herald:8082

# API-Schlüssel für die Dienst-zu-Dienst-Authentifizierung
export HERALD_API_KEY=your-secret-key

# Herald-Integration aktivieren (wenn Ihr Dienst dies unterstützt)
export HERALD_ENABLED=true
```

**Hinweis**: Herald kann eigenständig ohne externe Dienstabhängigkeiten verwendet werden. Die Integration mit anderen Diensten ist optional und hängt von Ihrem spezifischen Anwendungsfall ab.

## Sicherheit

- HMAC-Authentifizierung für die Produktion verwenden
- Starke API-Schlüssel setzen
- TLS/HTTPS in der Produktion verwenden
- Rate-Limits angemessen konfigurieren
- Redis auf verdächtige Aktivitäten überwachen
