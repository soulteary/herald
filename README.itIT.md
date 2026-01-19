# Herald - Servizio OTP e Codici di Verifica

> **📧 Il Tuo Gateway per la Verifica Sicura**

## 🌐 Documentazione Multilingue

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald è un servizio leggero pronto per la produzione per l'invio di codici di verifica (OTP) tramite e-mail (il supporto SMS è attualmente in sviluppo), con limitazione della velocità integrata, controlli di sicurezza e registrazione di audit.

## Caratteristiche

- 🚀 **Alte Prestazioni** : Costruito con Go e Fiber
- 🔒 **Sicuro** : Verifica basata su sfide con archiviazione hash
- 📊 **Limitazione della Velocità** : Limitazione della velocità multidimensionale (per utente, per IP, per destinazione)
- 📝 **Registrazione di Audit** : Traccia di audit completa per tutte le operazioni
- 🔌 **Provider Estendibili** : Supporto per provider di posta elettronica (i provider SMS sono implementazioni segnaposto e non sono ancora completamente funzionali)
- ⚡ **Backend Redis** : Archiviazione rapida e distribuita con Redis

## Avvio Rapido

```bash
# Eseguire con Docker Compose
docker-compose up -d

# Oppure eseguire direttamente
go run main.go
```

## Configurazione

Impostare le variabili d'ambiente :

- `PORT` : Porta del server (predefinito : `:8082`)
- `REDIS_ADDR` : Indirizzo Redis (predefinito : `localhost:6379`)
- `REDIS_PASSWORD` : Password Redis (opzionale)
- `REDIS_DB` : Numero del database Redis (predefinito : `0`)
- `API_KEY` : Chiave API per l'autenticazione tra servizi
- `LOG_LEVEL` : Livello di log (predefinito : `info`)

Per le opzioni di configurazione complete, vedere [DEPLOYMENT.md](docs/itIT/DEPLOYMENT.md).

## Documentazione API

Vedere [API.md](docs/itIT/API.md) per la documentazione API dettagliata.
