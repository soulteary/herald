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

| Variabile | Descrizione | Predefinito | Richiesto |
|-----------|-------------|-------------|-----------|
| `PORT` | Porta del server (può essere con o senza due punti, ad esempio `8082` o `:8082`) | `:8082` | No |
| `REDIS_ADDR` | Indirizzo Redis | `localhost:6379` | No |
| `REDIS_PASSWORD` | Password Redis | `` | No |
| `REDIS_DB` | Database Redis | `0` | No |
| `API_KEY` | Chiave API per l'autenticazione | `` | Consigliato |
| `HMAC_SECRET` | Segreto HMAC per l'autenticazione sicura | `` | Opzionale |
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
| `SMTP_HOST` | Host del server SMTP | `` | Per e-mail |
| `SMTP_PORT` | Porta del server SMTP | `587` | Per e-mail |
| `SMTP_USER` | Nome utente SMTP | `` | Per e-mail |
| `SMTP_PASSWORD` | Password SMTP | `` | Per e-mail |
| `SMTP_FROM` | Indirizzo mittente SMTP | `` | Per e-mail |
| `SMS_PROVIDER` | Fornitore SMS | `` | Per SMS |
| `ALIYUN_ACCESS_KEY` | Chiave di accesso Aliyun | `` | Per Aliyun SMS |
| `ALIYUN_SECRET_KEY` | Chiave segreta Aliyun | `` | Per Aliyun SMS |
| `ALIYUN_SIGN_NAME` | Nome della firma SMS Aliyun | `` | Per Aliyun SMS |
| `ALIYUN_TEMPLATE_CODE` | Codice del modello SMS Aliyun | `` | Per Aliyun SMS |

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
