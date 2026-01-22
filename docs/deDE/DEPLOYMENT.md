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

| Variable | Beschreibung | Standard | Erforderlich |
|----------|-------------|----------|--------------|
| `PORT` | Server-Port (kann mit oder ohne führenden Doppelpunkt sein, z. B. `8082` oder `:8082`) | `:8082` | Nein |
| `REDIS_ADDR` | Redis-Adresse | `localhost:6379` | Nein |
| `REDIS_PASSWORD` | Redis-Passwort | `` | Nein |
| `REDIS_DB` | Redis-Datenbank | `0` | Nein |
| `API_KEY` | API-Schlüssel für die Authentifizierung | `` | Empfohlen |
| `HMAC_SECRET` | HMAC-Geheimnis für sichere Authentifizierung | `` | Optional |
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
| `SMTP_HOST` | SMTP-Server-Host | `` | Für E-Mail |
| `SMTP_PORT` | SMTP-Server-Port | `587` | Für E-Mail |
| `SMTP_USER` | SMTP-Benutzername | `` | Für E-Mail |
| `SMTP_PASSWORD` | SMTP-Passwort | `` | Für E-Mail |
| `SMTP_FROM` | SMTP-Absenderadresse | `` | Für E-Mail |
| `SMS_PROVIDER` | SMS-Anbieter | `` | Für SMS |
| `ALIYUN_ACCESS_KEY` | Aliyun-Zugriffsschlüssel | `` | Für Aliyun SMS |
| `ALIYUN_SECRET_KEY` | Aliyun-Geheimschlüssel | `` | Für Aliyun SMS |
| `ALIYUN_SIGN_NAME` | Aliyun SMS-Signaturname | `` | Für Aliyun SMS |
| `ALIYUN_TEMPLATE_CODE` | Aliyun SMS-Vorlagencode | `` | Für Aliyun SMS |

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
