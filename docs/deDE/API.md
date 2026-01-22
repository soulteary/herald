# Herald API-Dokumentation

Herald ist ein Verifizierungscode- und OTP-Service, der das Senden von Verifizierungscodes über SMS und E-Mail mit integriertem Rate-Limiting und Sicherheitskontrollen verwaltet.

## Basis-URL

```
http://localhost:8082
```

## Authentifizierung

Herald unterstützt drei Authentifizierungsmethoden in folgender Prioritätsreihenfolge :

1. **mTLS** (Am sichersten) : Gegenseitiges TLS mit Client-Zertifikat-Verifizierung (höchste Priorität)
2. **HMAC-Signatur** (Sicher) : `X-Signature`, `X-Timestamp` und `X-Service`-Header setzen
3. **API-Schlüssel** (Einfach) : `X-API-Key`-Header setzen (niedrigste Priorität)

### mTLS-Authentifizierung

Bei Verwendung von HTTPS mit einem verifizierten Client-Zertifikat authentifiziert Herald die Anfrage automatisch über mTLS. Dies ist die sicherste Methode und hat Vorrang vor anderen Authentifizierungsmethoden.

### HMAC-Signatur

Die HMAC-Signatur wird wie folgt berechnet :
```
HMAC-SHA256(timestamp:service:body, secret)
```

Wobei :
- `timestamp` : Unix-Zeitstempel (Sekunden)
- `service` : Service-Identifikator (z. B. "my-service", "api-gateway")
- `body` : Anfragekörper (JSON-String)
- `secret` : HMAC-Geheimschlüssel

**Hinweis** : Der Zeitstempel muss innerhalb von 5 Minuten (300 Sekunden) der Serverzeit liegen, um Replay-Angriffe zu verhindern. Das Zeitstempel-Fenster ist konfigurierbar, standardmäßig jedoch 5 Minuten.

**Hinweis** : Derzeit wird der `X-Key-Id`-Header für Schlüsselrotation nicht unterstützt. Diese Funktion ist für zukünftige Versionen geplant.

## Endpunkte

### Gesundheitsprüfung

**GET /healthz**

Service-Gesundheit prüfen.

**Antwort :**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Challenge Erstellen

**POST /v1/otp/challenges**

Eine neue Verifizierungs-Challenge erstellen und Verifizierungscode senden.

**Anfrage :**
```json
{
  "user_id": "u_123",
  "channel": "sms",
  "destination": "+8613800138000",
  "purpose": "login",
  "locale": "zh-CN",
  "client_ip": "192.168.1.1",
  "ua": "Mozilla/5.0..."
}
```

**Antwort :**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

**Fehlerantworten :**

Alle Fehlerantworten folgen diesem Format :
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "optionale Fehlermeldung"
}
```

Mögliche Fehlercodes :
- `invalid_request` : Anfragekörper-Parsing fehlgeschlagen
- `user_id_required` : Erforderliches Feld `user_id` fehlt
- `invalid_channel` : Ungültiger Kanaltyp (muss "sms" oder "email" sein)
- `destination_required` : Erforderliches Feld `destination` fehlt
- `rate_limit_exceeded` : Rate-Limit überschritten
- `resend_cooldown` : Wartezeit für erneutes Senden noch nicht abgelaufen
- `user_locked` : Benutzer ist vorübergehend gesperrt
- `internal_error` : Interner Serverfehler

HTTP-Statuscodes :
- `400 Bad Request` : Ungültige Anfrageparameter
- `401 Unauthorized` : Authentifizierung fehlgeschlagen
- `403 Forbidden` : Benutzer gesperrt
- `429 Too Many Requests` : Rate-Limit überschritten
- `500 Internal Server Error` : Interner Serverfehler

### Challenge Verifizieren

**POST /v1/otp/verifications**

Einen Challenge-Code verifizieren.

**Anfrage :**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**Antwort (Erfolg) :**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**Antwort (Fehler) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Fehlerantworten :**

Mögliche Fehlercodes :
- `invalid_request` : Anfragekörper-Parsing fehlgeschlagen
- `challenge_id_required` : Erforderliches Feld `challenge_id` fehlt
- `code_required` : Erforderliches Feld `code` fehlt
- `invalid_code_format` : Verifizierungscode-Format ist ungültig
- `expired` : Challenge ist abgelaufen
- `invalid` : Ungültiger Verifizierungscode
- `locked` : Challenge aufgrund zu vieler Versuche gesperrt
- `verification_failed` : Allgemeiner Verifizierungsfehler
- `internal_error` : Interner Serverfehler

HTTP-Statuscodes :
- `400 Bad Request` : Ungültige Anfrageparameter
- `401 Unauthorized` : Verifizierung fehlgeschlagen
- `403 Forbidden` : Benutzer gesperrt
- `500 Internal Server Error` : Interner Serverfehler

### Challenge Widerrufen

**POST /v1/otp/challenges/{id}/revoke**

Eine Challenge widerrufen (optional).

**Antwort (Erfolg) :**
```json
{
  "ok": true
}
```

**Antwort (Fehler) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Fehlerantworten :**

Mögliche Fehlercodes :
- `challenge_id_required` : Challenge-ID fehlt im URL-Parameter
- `internal_error` : Interner Serverfehler

HTTP-Statuscodes :
- `400 Bad Request` : Ungültige Anfrage
- `500 Internal Server Error` : Interner Serverfehler

## Rate-Limiting

Herald implementiert mehrdimensionales Rate-Limiting :

- **Pro Benutzer** : 10 Anfragen pro Stunde (konfigurierbar)
- **Pro IP** : 5 Anfragen pro Minute (konfigurierbar)
- **Pro Ziel** : 10 Anfragen pro Stunde (konfigurierbar)
- **Wartezeit für erneutes Senden** : 60 Sekunden zwischen erneuten Sendungen

## Fehlercodes

Dieser Abschnitt listet alle möglichen Fehlercodes auf, die von der API zurückgegeben werden.

### Anfragevalidierungsfehler
- `invalid_request` : Anfragekörper-Parsing fehlgeschlagen oder ungültiges JSON
- `user_id_required` : Erforderliches Feld `user_id` fehlt
- `invalid_channel` : Ungültiger Kanaltyp (muss "sms" oder "email" sein)
- `destination_required` : Erforderliches Feld `destination` fehlt
- `challenge_id_required` : Erforderliches Feld `challenge_id` fehlt
- `code_required` : Erforderliches Feld `code` fehlt
- `invalid_code_format` : Verifizierungscode-Format ist ungültig

### Authentifizierungsfehler
- `authentication_required` : Keine gültige Authentifizierung bereitgestellt
- `invalid_timestamp` : Ungültiges Zeitstempel-Format
- `timestamp_expired` : Zeitstempel liegt außerhalb des erlaubten Fensters (5 Minuten)
- `invalid_signature` : HMAC-Signatur-Verifizierung fehlgeschlagen

### Challenge-Fehler
- `expired` : Challenge ist abgelaufen
- `invalid` : Ungültiger Verifizierungscode
- `locked` : Challenge aufgrund zu vieler Versuche gesperrt
- `too_many_attempts` : Zu viele fehlgeschlagene Versuche (kann in `locked` enthalten sein)
- `verification_failed` : Allgemeiner Verifizierungsfehler

### Rate-Limiting-Fehler
- `rate_limit_exceeded` : Rate-Limit überschritten
- `resend_cooldown` : Wartezeit für erneutes Senden noch nicht abgelaufen

### Benutzerstatusfehler
- `user_locked` : Benutzer ist vorübergehend gesperrt

### Systemfehler
- `internal_error` : Interner Serverfehler
