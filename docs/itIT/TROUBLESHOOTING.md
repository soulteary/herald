# Guida alla Risoluzione dei Problemi Herald

Questa guida ti aiuta a diagnosticare e risolvere problemi comuni con il servizio Herald OTP e codici di verifica.

## Indice

- [Codice di Verifica Non Ricevuto](#codice-di-verifica-non-ricevuto)
- [Errori Codice di Verifica](#errori-codice-di-verifica)
- [Errori 401 Non Autorizzato](#errori-401-non-autorizzato)
- [Problemi di Limitazione della Velocità](#problemi-di-limitazione-della-velocità)
- [Problemi di Connessione Redis](#problemi-di-connessione-redis)
- [Fallimenti Invio Provider](#fallimenti-invio-provider)
- [Problemi di Prestazioni](#problemi-di-prestazioni)

## Codice di Verifica Non Ricevuto

### Sintomi
- L'utente segnala di non aver ricevuto il codice di verifica tramite SMS o e-mail
- La creazione della challenge ha successo ma nessun codice viene consegnato

### Passaggi di Diagnostica

1. **Verificare la Connessione Provider**
   ```bash
   # Controllare i log Herald per errori del provider
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **Verificare la Configurazione Provider**
   - Controllare le impostazioni SMTP (per e-mail): `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - Controllare le impostazioni del provider SMS: `SMS_PROVIDER`, `ALIYUN_ACCESS_KEY`, ecc.
   - Verificare che le credenziali del provider siano corrette

3. **Controllare le Metriche Prometheus**
   ```promql
   # Controllare il tasso di fallimento invio
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # Controllare il successo invio per provider
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **Controllare i Log di Audit**
   - Esaminare i log di audit per eventi `send_failed`
   - Controllare i codici di errore e i messaggi del provider
   - Verificare che gli indirizzi di destinazione siano validi

5. **Testare il Provider Direttamente**
   - Testare la connessione SMTP manualmente
   - Testare l'API del provider SMS direttamente
   - Verificare la connettività di rete agli endpoint del provider

### Soluzioni

- **Problemi di Configurazione Provider**: Aggiornare le credenziali e le impostazioni del provider
- **Problemi di Rete**: Controllare le regole del firewall e la connettività di rete
- **Limiti di Velocità Provider**: Controllare se il provider ha limiti di velocità che vengono superati
- **Destinazioni Non Valide**: Verificare che gli indirizzi e-mail e i numeri di telefono siano validi

## Errori Codice di Verifica

### Sintomi
- L'utente segnala errori "codice non valido"
- La verifica fallisce anche con il codice corretto
- La challenge appare scaduta o bloccata

### Passaggi di Diagnostica

1. **Controllare lo Stato della Challenge**
   ```bash
   # Controllare Redis per i dati della challenge
   redis-cli GET "otp:ch:{challenge_id}"
   ```

2. **Verificare la Scadenza della Challenge**
   - Controllare la configurazione `CHALLENGE_EXPIRY` (predefinito: 5 minuti)
   - Verificare che l'ora di sistema sia sincronizzata (NTP)
   - Controllare se la challenge è scaduta

3. **Controllare il Conteggio dei Tentativi**
   - Verificare la configurazione `MAX_ATTEMPTS` (predefinito: 5)
   - Controllare se la challenge è bloccata a causa di troppi tentativi
   - Esaminare i log di audit per la cronologia dei tentativi

4. **Controllare le Metriche Prometheus**
   ```promql
   # Controllare i motivi di fallimento della verifica
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # Controllare le challenge bloccate
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
   ```

5. **Esaminare i Log di Audit**
   - Controllare i tentativi e i risultati della verifica
   - Verificare che il formato del codice corrisponda al formato previsto
   - Controllare problemi di timing

### Soluzioni

- **Challenge Scaduta**: L'utente deve richiedere un nuovo codice di verifica
- **Troppi Tentativi**: La challenge è bloccata, attendere la durata del blocco o richiedere una nuova challenge
- **Incompatibilità Formato Codice**: Verificare che la lunghezza del codice corrisponda alla configurazione `CODE_LENGTH`
- **Sincronizzazione Temporale**: Assicurarsi che gli orologi di sistema siano sincronizzati

## Errori 401 Non Autorizzato

### Sintomi
- Le richieste API restituiscono 401 Non Autorizzato
- Fallimenti di autenticazione nei log
- La comunicazione servizio-servizio fallisce

### Passaggi di Diagnostica

1. **Controllare il Metodo di Autenticazione**
   - Verificare quale metodo di autenticazione viene utilizzato (mTLS, HMAC, Chiave API)
   - Controllare se sono presenti gli header di autenticazione

2. **Per Autenticazione HMAC**
   ```bash
   # Verificare che HMAC_SECRET sia configurato
   echo $HMAC_SECRET
   
   # Controllare che il timestamp sia entro la finestra di 5 minuti
   # Verificare che il calcolo della firma corrisponda all'implementazione Herald
   ```

3. **Per Autenticazione Chiave API**
   ```bash
   # Verificare che API_KEY sia impostato
   echo $API_KEY
   
   # Controllare che l'header X-API-Key corrisponda alla chiave configurata
   ```

4. **Per Autenticazione mTLS**
   - Verificare che il certificato client sia valido
   - Controllare che la catena di certificati sia attendibile
   - Verificare che la connessione TLS sia stabilita

5. **Controllare i Log Herald**
   ```bash
   # Controllare errori di autenticazione
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### Soluzioni

- **Credenziali Mancanti**: Impostare le variabili d'ambiente `API_KEY` o `HMAC_SECRET`
- **Firma Non Valida**: Verificare che il calcolo della firma HMAC corrisponda all'implementazione Herald
- **Timestamp Scaduto**: Assicurarsi che gli orologi client e server siano sincronizzati (entro 5 minuti)
- **Problemi di Certificato**: Verificare che i certificati mTLS siano validi e attendibili

## Problemi di Limitazione della Velocità

### Sintomi
- Gli utenti segnalano errori "limite di velocità superato"
- Le richieste legittime vengono bloccate
- Metriche elevate di hit di limitazione della velocità

### Passaggi di Diagnostica

1. **Controllare la Configurazione di Limitazione della Velocità**
   ```bash
   # Verificare le impostazioni di limitazione della velocità
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **Controllare le Metriche Prometheus**
   ```promql
   # Controllare gli hit di limitazione della velocità per ambito
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # Controllare il tasso di hit di limitazione della velocità
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Esaminare le Chiavi Redis**
   ```bash
   # Controllare le chiavi di limitazione della velocità
   redis-cli KEYS "otp:rate:*"
   
   # Controllare una chiave di limitazione della velocità specifica
   redis-cli GET "otp:rate:user:{user_id}"
   ```

4. **Analizzare i Modelli di Utilizzo**
   - Controllare se gli utenti legittimi raggiungono i limiti
   - Identificare potenziali abusi o attività di bot
   - Esaminare i modelli di creazione delle challenge

### Soluzioni

- **Regolare i Limiti di Velocità**: Aumentare i limiti se gli utenti legittimi sono interessati
- **Educazione Utenti**: Informare gli utenti sui limiti di velocità e i periodi di cooldown
- **Prevenzione Abusi**: Implementare misure di sicurezza aggiuntive per sospetti abusi
- **Cooldown Reinvio**: Gli utenti devono attendere il periodo `RESEND_COOLDOWN` (predefinito: 60 secondi)

## Problemi di Connessione Redis

### Sintomi
- Il servizio non si avvia
- Il controllo dello stato di salute restituisce non sano
- La creazione/verifica della challenge fallisce
- Metriche di latenza Redis elevate

### Passaggi di Diagnostica

1. **Controllare la Connettività Redis**
   ```bash
   # Testare la connessione Redis
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **Verificare la Configurazione**
   ```bash
   # Controllare la configurazione Redis
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   echo $REDIS_DB
   ```

3. **Controllare la Salute Redis**
   ```bash
   # Controllare le informazioni Redis
   redis-cli INFO
   
   # Controllare l'utilizzo della memoria
   redis-cli INFO memory
   ```

4. **Controllare le Metriche Prometheus**
   ```promql
   # Controllare la latenza Redis
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # Controllare gli errori delle operazioni Redis
   rate(herald_redis_latency_seconds_count[5m])
   ```

5. **Esaminare i Log Herald**
   ```bash
   # Controllare errori Redis
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### Soluzioni

- **Problemi di Connessione**: Verificare che `REDIS_ADDR` sia corretto e che Redis sia accessibile
- **Problemi di Autenticazione**: Controllare che `REDIS_PASSWORD` sia corretto
- **Selezione Database**: Verificare che `REDIS_DB` sia corretto (predefinito: 0)
- **Problemi di Prestazioni**: Controllare l'utilizzo della memoria Redis e considerare il ridimensionamento
- **Problemi di Rete**: Verificare la connettività di rete e le regole del firewall

## Fallimenti Invio Provider

### Sintomi
- Tasso di fallimento invio elevato nelle metriche
- Errori `send_failed` nei log
- Messaggi di errore specifici del provider

### Passaggi di Diagnostica

1. **Controllare le Metriche Provider**
   ```promql
   # Controllare il tasso di fallimento invio per provider
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # Controllare la durata invio per provider
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **Esaminare i Log Provider**
   - Controllare i messaggi di errore specifici del provider
   - Esaminare i log di audit per errori del provider
   - Controllare i codici di risposta dell'API del provider

3. **Testare la Connessione Provider**
   - Testare la connessione SMTP manualmente
   - Testare l'API del provider SMS direttamente
   - Verificare le credenziali del provider

4. **Controllare lo Stato Provider**
   - Controllare la pagina di stato del provider
   - Verificare che l'API del provider sia operativa
   - Controllare i limiti di velocità del provider

### Soluzioni

- **Interruzione Provider**: Attendere che il provider ripristini il servizio
- **Problemi di Credenziali**: Aggiornare le credenziali del provider
- **Limiti di Velocità**: Controllare se i limiti di velocità del provider vengono superati
- **Problemi di Configurazione**: Verificare che la configurazione del provider sia corretta
- **Problemi di Rete**: Controllare la connettività di rete al provider

## Problemi di Prestazioni

### Sintomi
- Tempi di risposta lenti
- Metriche di latenza elevate
- Errori di timeout
- Utilizzo elevato delle risorse

### Passaggi di Diagnostica

1. **Controllare i Tempi di Risposta**
   ```promql
   # Controllare la durata invio
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # Controllare la latenza Redis
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **Controllare l'Utilizzo delle Risorse**
   ```bash
   # Controllare CPU e memoria
   top -p $(pgrep herald)
   
   # Controllare il conteggio Goroutine
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
   ```

3. **Esaminare i Modelli di Richiesta**
   - Controllare il tasso di richiesta
   - Identificare i tempi di utilizzo di picco
   - Controllare i picchi di traffico

4. **Controllare le Prestazioni Redis**
   ```bash
   # Controllare il log lento Redis
   redis-cli SLOWLOG GET 10
   
   # Controllare la memoria Redis
   redis-cli INFO memory
   ```

### Soluzioni

- **Ridimensionare le Risorse**: Aumentare l'allocazione CPU/memoria
- **Ottimizzare Redis**: Utilizzare Redis Cluster o ottimizzare le query
- **Ottimizzazione Provider**: Utilizzare provider più veloci o ottimizzare le chiamate al provider
- **Caching**: Implementare la cache per i dati frequentemente accessibili
- **Bilanciamento del Carico**: Distribuire il carico su più istanze

## Ottenere Aiuto

Se non riesci a risolvere un problema:

1. **Controllare i Log**: Esaminare i log Herald per messaggi di errore dettagliati
2. **Controllare le Metriche**: Esaminare le metriche Prometheus per modelli
3. **Esaminare la Documentazione**: Controllare [API.md](API.md) e [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Aprire un Problema**: Creare un problema con:
   - Messaggi di errore e log
   - Configurazione (senza segreti)
   - Passaggi per riprodurre
   - Comportamento atteso vs reale

## Documentazione Correlata

- [Documentazione API](API.md) - Dettagli endpoint API
- [Guida al Deployment](DEPLOYMENT.md) - Deployment e configurazione
- [Guida al Monitoraggio](MONITORING.md) - Monitoraggio e metriche
