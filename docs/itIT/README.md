# Indice della Documentazione

> **Traduzione non aggiornata / Historical translation.** This page predates current security changes.
> Do not use its commands or configuration as current instructions. See the
> [current English guide](../enUS/README.md) or [中文](../zhCN/README.md)
> and [translation status](STALE.md). HMAC creation requires v3; builds and validation use Linux.

Benvenuti nella documentazione del servizio Herald OTP e codici di verifica.

## 🌐 Documentazione Multilingue

- [English](../enUS/README.md) | [中文](../zhCN/README.md) | [Français](../frFR/README.md) | [Italiano](README.md) | [日本語](../jaJP/README.md) | [Deutsch](../deDE/README.md) | [한국어](../koKR/README.md)

## 📚 Elenco Documenti

### Documenti Principali

- **[README.md](../../README.itIT.md)** - Panoramica del progetto e guida rapida

### Documenti Dettagliati

- **[ARCHITECTURE.md](ARCHITECTURE.md)** - Architettura full-stack
  - Stargate / Warden / Herald / Redis / Provider
  - Flusso dati e interfacce chiave (challenge, verify, invio provider)
  - Confini di sicurezza (HMAC/mTLS, PII)

- **[API.md](API.md)** - Documentazione completa degli endpoint API
  - Metodi di autenticazione
  - Endpoint di controllo dello stato
  - Creazione e verifica delle sfide
  - Limitazione della velocità
  - Codici di errore e risposte

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Guida al deployment
  - Deployment con Docker Compose
  - Deployment manuale
  - Opzioni di configurazione
  - Integrazione opzionale con altri servizi
  - Best practice di sicurezza

- **[MONITORING.md](MONITORING.md)** - Guida al monitoraggio
  - Metriche Prometheus
  - Dashboard Grafana
  - Regole di alerting
  - Migliori pratiche

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Guida alla risoluzione dei problemi
  - Problemi comuni e soluzioni
  - Passaggi di diagnostica
  - Ottimizzazione delle prestazioni

## 🚀 Navigazione Rapida

### Per Iniziare

1. Leggi [README.itIT.md](../../README.itIT.md) per comprendere il progetto
2. Controlla la sezione [Avvio Rapido](../../README.itIT.md#avvio-rapido)
3. Fai riferimento a [Configurazione](../../README.itIT.md#configurazione-di-base) per configurare il servizio

### Sviluppatori

1. Leggi [ARCHITECTURE.md](ARCHITECTURE.md) per la panoramica full-stack
2. Controlla [API.md](API.md) per comprendere le interfacce API
3. Esamina [DEPLOYMENT.md](DEPLOYMENT.md) per le opzioni di deployment

### Operazioni

1. Leggi [DEPLOYMENT.md](DEPLOYMENT.md) per comprendere i metodi di deployment
2. Controlla [API.md](API.md) per i dettagli degli endpoint API
3. Fai riferimento a [Sicurezza](DEPLOYMENT.md#sicurezza) per le best practice di sicurezza
4. Monitorare lo stato del servizio: [MONITORING.md](MONITORING.md)
5. Risolvere i problemi: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 Struttura dei Documenti

```
herald/
├── README.md              # Documento principale del progetto (Inglese)
├── README.itIT.md         # Documento principale del progetto (Italiano)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # Indice della documentazione (Inglese)
│   │   ├── ARCHITECTURE.md # Architettura full-stack (Inglese)
│   │   ├── API.md          # Documento API (Inglese)
│   │   ├── DEPLOYMENT.md   # Guida al deployment (Inglese)
│   │   ├── MONITORING.md   # Guida al monitoraggio (Inglese)
│   │   └── TROUBLESHOOTING.md # Guida alla risoluzione dei problemi (Inglese)
│   └── itIT/
│       ├── README.md       # Indice della documentazione (Italiano, questo file)
│       ├── ARCHITECTURE.md # Architettura full-stack (Italiano)
│       ├── API.md          # Documento API (Italiano)
│       ├── DEPLOYMENT.md   # Guida al deployment (Italiano)
│       ├── MONITORING.md   # Guida al monitoraggio (Italiano)
│       └── TROUBLESHOOTING.md # Guida alla risoluzione dei problemi (Italiano)
└── ...
```

## 🔍 Cerca per Argomento

### Relativo all'API

- Elenco endpoint API : [API.md](API.md)
- Metodi di autenticazione : [API.md#autenticazione](API.md#autenticazione)
- Gestione degli errori : [API.md#codici-di-errore](API.md#codici-di-errore)
- Limitazione della velocità : [API.md#limitazione-della-velocità](API.md#limitazione-della-velocità)

### Relativo al Deployment

- Deployment Docker : [DEPLOYMENT.md#avvio-rapido](DEPLOYMENT.md#avvio-rapido)
- Opzioni di configurazione : [DEPLOYMENT.md#configurazione](DEPLOYMENT.md#configurazione)
- Integrazione servizi : [DEPLOYMENT.md#integrazione-con-altri-servizi-opzionale](DEPLOYMENT.md#integrazione-con-altri-servizi-opzionale)
- Sicurezza : [DEPLOYMENT.md#sicurezza](DEPLOYMENT.md#sicurezza)

### Monitoraggio e Operazioni

- Metriche Prometheus : [MONITORING.md](MONITORING.md)
- Dashboard Grafana : [MONITORING.md#dashboard-grafana](MONITORING.md#dashboard-grafana)
- Risoluzione problemi : [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 Raccomandazioni d'Uso

1. **Utenti per la prima volta** : Inizia con [README.itIT.md](../../README.itIT.md) e segui la guida di avvio rapido
2. **Configurare il servizio** : Fai riferimento a [DEPLOYMENT.md](DEPLOYMENT.md) per comprendere tutte le opzioni di configurazione
3. **Integrare con i servizi** : Controlla la sezione di integrazione in [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Integrazione API** : Leggi [API.md](API.md) per comprendere le interfacce API
5. **Monitorare il servizio** : Configura il monitoraggio con [MONITORING.md](MONITORING.md)
6. **Risolvere i problemi** : Fai riferimento a [TROUBLESHOOTING.md](TROUBLESHOOTING.md) per problemi comuni

## 📝 Aggiornamenti della Documentazione

La documentazione viene continuamente aggiornata man mano che il progetto evolve. Se trovi errori o hai bisogno di aggiunte, invia un Issue o una Pull Request.

## 🤝 Contribuire

Sono benvenuti i miglioramenti alla documentazione :

1. Trova errori o aree che necessitano di miglioramento
2. Invia un Issue che descriva il problema
3. Oppure invia direttamente una Pull Request
