# Herald - Servizio OTP e Codici di Verifica

> **📧 Il Tuo Gateway per la Verifica Sicura**

## 🌐 Documentazione Multilingue

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald è un servizio OTP e codici di verifica autonomo pronto per la produzione che invia codici di verifica tramite e-mail e SMS. Dispone di limitazione della velocità integrata, controlli di sicurezza e registrazione di audit. Herald è progettato per funzionare in modo indipendente e può essere integrato con altri servizi se necessario.

## Caratteristiche Principali

- 🔒 **Sicurezza per Progettazione** : Verifica basata su sfide con archiviazione hash Argon2, metodi di autenticazione multipli (mTLS, HMAC, API Key)
- 📊 **Limitazione della Velocità Integrata** : Limitazione della velocità multidimensionale (per utente, per IP, per destinazione) con soglie configurabili
- 📝 **Traccia di Audit Completa** : Registrazione di audit completa per tutte le operazioni con tracciamento del provider
- 🔌 **Provider Estendibili** : Architettura provider e-mail e SMS estendibile

## Avvio Rapido

### Utilizzo di Docker Compose

Il modo più semplice per iniziare è con Docker Compose, che include Redis:

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

Risposta attesa:
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Testare l'API

Creare una sfida di test (richiede autenticazione - vedere [Documentazione API](docs/itIT/API.md)):

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

### Visualizzare i Log

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### Distribuzione Manuale

Per la distribuzione manuale e la configurazione avanzata, vedere la [Guida alla Distribuzione](docs/itIT/DEPLOYMENT.md).

## Configurazione di Base

Herald richiede una configurazione minima per iniziare:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

Per le opzioni di configurazione complete, inclusi limiti di velocità, scadenza delle sfide e impostazioni del provider, vedere la [Guida alla Distribuzione](docs/itIT/DEPLOYMENT.md#configuration).

## Documentazione

### Per gli Sviluppatori

- **[Documentazione API](docs/itIT/API.md)** - Riferimento API completo con metodi di autenticazione, endpoint e codici di errore
- **[Guida alla Distribuzione](docs/itIT/DEPLOYMENT.md)** - Opzioni di configurazione, distribuzione Docker ed esempi di integrazione

### Per le Operazioni

- **[Guida al Monitoraggio](docs/itIT/MONITORING.md)** - Metriche Prometheus, dashboard Grafana e avvisi
- **[Guida alla Risoluzione dei Problemi](docs/itIT/TROUBLESHOOTING.md)** - Problemi comuni, passaggi diagnostici e soluzioni

### Indice della Documentazione

Per una panoramica completa di tutta la documentazione, vedere [docs/itIT/README.md](docs/itIT/README.md).

## License

See [LICENSE](LICENSE) for details.
