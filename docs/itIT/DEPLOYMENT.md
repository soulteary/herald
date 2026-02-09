# Guida al Deployment di Herald

## Avvio Rapido

### Utilizzo di Docker Compose

```bash
cd herald
docker-compose up -d
```

### Deployment Manuale

```bash
# Compilare
go build -o herald main.go

# Eseguire
./herald
```

## Configurazione

### Variabili d'Ambiente

**Elenco completo (allineato al codice):** [Inglese](enUS/DEPLOYMENT.md#environment-variables) | [中文](zhCN/DEPLOYMENT.md#环境变量)

| Variabile | Descrizione | Predefinito | Richiesto |
|-----------|-------------|-------------|-----------|
| `PORT` | Porta del server (può essere con o senza due punti, ad esempio `8082` o `:8082`) | `:8082` | No |
| `REDIS_ADDR` | Indirizzo Redis | `localhost:6379` | No |
| `REDIS_PASSWORD` | Password Redis | `` | No |
| `REDIS_DB` | Database Redis | `0` | No |
| `API_KEY` | Chiave API per l'autenticazione | `` | Consigliato |
| `HMAC_SECRET` | Segreto HMAC per l'autenticazione sicura | `` | Opzionale |
| `HERALD_HMAC_KEYS` | Chiavi HMAC multiple (JSON) | `` | Opzionale |
| `LOG_LEVEL` | Livello di log | `info` | No |
| `CHALLENGE_EXPIRY` | Scadenza della sfida | `5m` | No |
| `MAX_ATTEMPTS` | Numero massimo di tentativi di verifica | `5` | No |
| `RESEND_COOLDOWN` | Cooldown per il reinvio | `60s` | No |
| `CODE_LENGTH` | Lunghezza del codice di verifica | `6` | No |
| `RATE_LIMIT_PER_USER` | Limite di velocità per utente/ora | `10` | No |
| `RATE_LIMIT_PER_IP` | Limite di velocità per IP/minuto | `5` | No |
| `RATE_LIMIT_PER_DESTINATION` | Limite di velocità per destinazione/ora | `10` | No |
| `LOCKOUT_DURATION` | Durata del blocco utente dopo il numero massimo di tentativi | `10m` | No |
| `SERVICE_NAME` | Identificatore del servizio per l'autenticazione HMAC | `herald` | No |
| `SMTP_HOST` | Host del server SMTP | `` | Per e-mail (integrato) |
| `SMTP_PORT` | Porta del server SMTP | `587` | Per e-mail |
| `SMTP_USER` | Nome utente SMTP | `` | Per e-mail |
| `SMTP_PASSWORD` | Password SMTP | `` | Per e-mail |
| `SMTP_FROM` | Indirizzo mittente SMTP | `` | Per e-mail |
| `SMS_PROVIDER` | Fornitore SMS (es. nome per i log) | `` | Per SMS |
| `SMS_API_BASE_URL` | URL base API HTTP SMS | `` | Per SMS (API HTTP) |
| `SMS_API_KEY` | Chiave API SMS | `` | Per SMS (opzionale) |
| `HERALD_DINGTALK_API_URL` | URL base di [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (es. `http://herald-dingtalk:8083`) | `` | Per il canale DingTalk |
| `HERALD_DINGTALK_API_KEY` | Chiave API opzionale; deve corrispondere a `API_KEY` di herald-dingtalk se impostata | `` | No |
| `HERALD_SMTP_API_URL` | URL base di [herald-smtp](https://github.com/soulteary/herald-smtp) (es. `http://herald-smtp:8084`); se impostata, SMTP integrato non usato | `` | Per canale e-mail (opzionale) |
| `HERALD_SMTP_API_KEY` | Chiave API opzionale; deve corrispondere a `API_KEY` di herald-smtp se impostata | `` | No |
| `HERALD_TEST_MODE` | Se `true`: codice debug in Redis/risposta. **Solo per test; in produzione sempre `false`.** | `false` | No |

### Canale e-mail (herald-smtp)

Quando `HERALD_SMTP_API_URL` è impostata, Herald non utilizza l'SMTP integrato. Inoltra l'invio e-mail a [herald-smtp](https://github.com/soulteary/herald-smtp) via HTTP. Tutte le credenziali e la logica SMTP risiedono in herald-smtp; Herald non memorizza alcuna credenziale SMTP per il canale e-mail in questa modalità. Impostare `HERALD_SMTP_API_URL` sull'URL base del servizio herald-smtp. Se herald-smtp è configurato con `API_KEY`, impostare `HERALD_SMTP_API_KEY` sullo stesso valore. Quando `HERALD_SMTP_API_URL` è impostata, Herald ignora `SMTP_HOST` e le impostazioni SMTP integrate correlate.

### Canale DingTalk (herald-dingtalk)

Quando `channel` è `dingtalk`, Herald non invia i messaggi direttamente ma inoltra l'invio a [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) via HTTP. Tutte le credenziali e la logica DingTalk risiedono in herald-dingtalk; Herald non memorizza alcuna credenziale DingTalk. Impostare `HERALD_DINGTALK_API_URL` sull'URL base del servizio herald-dingtalk. Se herald-dingtalk è configurato con `API_KEY`, impostare `HERALD_DINGTALK_API_KEY` sullo stesso valore.

## Integrazione con altri servizi (Opzionale)

Herald è progettato per funzionare in modo indipendente e può essere integrato con altri servizi se necessario. Se si desidera integrare Herald con altri servizi di autenticazione o gateway, è possibile configurare quanto segue:

**Esempio di configurazione di integrazione:**
```bash
# URL del servizio dove Herald è accessibile
export HERALD_URL=http://herald:8082

# Chiave API per l'autenticazione tra servizi
export HERALD_API_KEY=your-secret-key

# Abilitare l'integrazione Herald (se il servizio lo supporta)
export HERALD_ENABLED=true
```

**Nota**: Herald può essere utilizzato in modo autonomo senza dipendenze da servizi esterni. L'integrazione con altri servizi è opzionale e dipende dal caso d'uso specifico.

## Sicurezza

- Utilizzare l'autenticazione HMAC per la produzione
- Impostare chiavi API forti
- Utilizzare TLS/HTTPS in produzione
- Configurare i limiti di velocità in modo appropriato
- Monitorare Redis per attività sospette
