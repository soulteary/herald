# Documentazione API Herald

Herald è un servizio di codici di verifica e OTP che gestisce l'invio di codici di verifica tramite SMS e e-mail, con limitazione della velocità integrata e controlli di sicurezza.

## URL di Base

```
http://localhost:8082
```

## Autenticazione

Herald supporta due metodi di autenticazione :

1. **Chiave API** (Semplice) : Impostare l'intestazione `X-API-Key`
2. **Firma HMAC** (Sicuro) : Impostare le intestazioni `X-Signature`, `X-Timestamp` e `X-Service`

### Firma HMAC

La firma HMAC viene calcolata come :
```
HMAC-SHA256(timestamp:service:body, secret)
```

Dove :
- `timestamp` : Timestamp Unix (secondi)
- `service` : Identificatore del servizio (ad esempio, "stargate")
- `body` : Corpo della richiesta (stringa JSON)
- `secret` : Chiave segreta HMAC

## Endpoint

### Controllo dello Stato

**GET /health**

Verificare lo stato di salute del servizio.

**Risposta :**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Creare una Sfida

**POST /v1/otp/challenges**

Creare una nuova sfida di verifica e inviare un codice di verifica.

**Richiesta :**
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

**Risposta :**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

**Risposte di Errore :**

Tutte le risposte di errore seguono questo formato :
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "messaggio di errore opzionale"
}
```

Codici di errore possibili :
- `invalid_request` : Parsing del corpo della richiesta fallito
- `user_id_required` : Campo richiesto `user_id` mancante
- `invalid_channel` : Tipo di canale non valido (deve essere "sms" o "email")
- `destination_required` : Campo richiesto `destination` mancante
- `rate_limit_exceeded` : Limite di velocità superato
- `resend_cooldown` : Periodo di cooldown per il reinvio non scaduto
- `user_locked` : L'utente è temporaneamente bloccato
- `internal_error` : Errore interno del server

Codici di Stato HTTP :
- `400 Bad Request` : Parametri della richiesta non validi
- `401 Unauthorized` : Autenticazione fallita
- `403 Forbidden` : Utente bloccato
- `429 Too Many Requests` : Limite di velocità superato
- `500 Internal Server Error` : Errore interno del server

### Verificare una Sfida

**POST /v1/otp/verifications**

Verificare un codice di sfida.

**Richiesta :**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**Risposta (Successo) :**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**Risposta (Fallimento) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Risposte di Errore :**

Codici di errore possibili :
- `invalid_request` : Parsing del corpo della richiesta fallito
- `challenge_id_required` : Campo richiesto `challenge_id` mancante
- `code_required` : Campo richiesto `code` mancante
- `invalid_code_format` : Il formato del codice di verifica non è valido
- `expired` : La sfida è scaduta
- `invalid` : Codice di verifica non valido
- `locked` : Sfida bloccata a causa di troppi tentativi
- `verification_failed` : Fallimento generale della verifica
- `internal_error` : Errore interno del server

Codici di Stato HTTP :
- `400 Bad Request` : Parametri della richiesta non validi
- `401 Unauthorized` : Verifica fallita
- `403 Forbidden` : Utente bloccato
- `500 Internal Server Error` : Errore interno del server

### Revocare una Sfida

**POST /v1/otp/challenges/{id}/revoke**

Revocare una sfida (opzionale).

**Risposta (Successo) :**
```json
{
  "ok": true
}
```

**Risposta (Fallimento) :**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Risposte di Errore :**

Codici di errore possibili :
- `challenge_id_required` : ID della sfida mancante nel parametro URL
- `internal_error` : Errore interno del server

Codici di Stato HTTP :
- `400 Bad Request` : Richiesta non valida
- `500 Internal Server Error` : Errore interno del server

## Limitazione della Velocità

Herald implementa una limitazione della velocità multidimensionale :

- **Per Utente** : 10 richieste all'ora (configurabile)
- **Per IP** : 5 richieste al minuto (configurabile)
- **Per Destinazione** : 10 richieste all'ora (configurabile)
- **Cooldown per Reinvio** : 60 secondi tra i reinvii

## Codici di Errore

Questa sezione elenca tutti i possibili codici di errore restituiti dall'API.

### Errori di Validazione della Richiesta
- `invalid_request` : Parsing del corpo della richiesta fallito o JSON non valido
- `user_id_required` : Campo richiesto `user_id` mancante
- `invalid_channel` : Tipo di canale non valido (deve essere "sms" o "email")
- `destination_required` : Campo richiesto `destination` mancante
- `challenge_id_required` : Campo richiesto `challenge_id` mancante
- `code_required` : Campo richiesto `code` mancante
- `invalid_code_format` : Il formato del codice di verifica non è valido

### Errori di Autenticazione
- `authentication_required` : Nessuna autenticazione valida fornita
- `invalid_timestamp` : Formato del timestamp non valido
- `timestamp_expired` : Il timestamp è fuori dalla finestra consentita (5 minuti)
- `invalid_signature` : Verifica della firma HMAC fallita

### Errori di Sfida
- `expired` : La sfida è scaduta
- `invalid` : Codice di verifica non valido
- `locked` : Sfida bloccata a causa di troppi tentativi
- `too_many_attempts` : Troppi tentativi falliti (può essere incluso in `locked`)
- `verification_failed` : Fallimento generale della verifica

### Errori di Limitazione della Velocità
- `rate_limit_exceeded` : Limite di velocità superato
- `resend_cooldown` : Periodo di cooldown per il reinvio non scaduto

### Errori di Stato Utente
- `user_locked` : L'utente è temporaneamente bloccato

### Errori di Sistema
- `internal_error` : Errore interno del server
