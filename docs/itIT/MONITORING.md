# Guida al Monitoraggio Herald

Herald espone metriche Prometheus per il monitoraggio e l'osservabilità. Questo documento descrive tutte le metriche disponibili, come configurare il monitoraggio e le migliori pratiche.

## Endpoint delle Metriche

Herald espone le metriche Prometheus all'endpoint `/metrics`:

```
GET http://localhost:8082/metrics
```

## Metriche Disponibili

### Metriche delle Challenge

#### `herald_otp_challenges_total`

Contatore che traccia il numero totale di challenge OTP create.

**Labels:**
- `channel`: Tipo di canale (`sms` o `email`)
- `purpose`: Scopo della challenge (ad es. `login`, `reset`, `bind`)
- `result`: Risultato dell'operazione (`success` o `failed`)

**Esempio:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### Metriche di Invio

#### `herald_otp_sends_total`

Contatore che traccia il numero totale di invii OTP tramite provider.

**Labels:**
- `channel`: Tipo di canale (`sms` o `email`)
- `provider`: Nome del provider (ad es. `smtp`, `aliyun`, `placeholder`)
- `result`: Risultato dell'operazione di invio (`success` o `failed`)

**Esempio:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

Istogramma che traccia la durata delle operazioni di invio OTP.

**Labels:**
- `provider`: Nome del provider

**Buckets:** Bucket Prometheus predefiniti (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Esempio:**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### Metriche di Verifica

#### `herald_otp_verifications_total`

Contatore che traccia il numero totale di verifiche OTP.

**Labels:**
- `result`: Risultato della verifica (`success` o `failed`)
- `reason`: Motivo del fallimento (ad es. `expired`, `invalid`, `locked`, `too_many_attempts`)

**Esempio:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### Metriche di Limitazione della Velocità

#### `herald_rate_limit_hits_total`

Contatore che traccia il numero totale di hit di limitazione della velocità.

**Labels:**
- `scope`: Ambito di limitazione della velocità (`user`, `ip`, `destination`, `resend_cooldown`)

**Esempio:**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Metriche Redis

#### `herald_redis_latency_seconds`

Istogramma che traccia la latenza delle operazioni Redis.

**Labels:**
- `operation`: Tipo di operazione Redis (`get`, `set`, `del`, `exists`)

**Buckets:** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**Esempio:**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Configurazione Prometheus

### Configurazione di Scrape Base

Aggiungere quanto segue al file `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
```

### Service Discovery

Per i deployment Kubernetes, utilizzare il service discovery:

```yaml
scrape_configs:
  - job_name: 'herald'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - default
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: herald
        action: keep
      - source_labels: [__meta_kubernetes_pod_ip]
        target_label: __address__
        replacement: '${1}:8082'
```

## Metriche Chiave da Monitorare

### Metriche di Business

1. **Tasso di Creazione Challenge**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **Tasso di Successo Challenge**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **Tasso di Successo Verifica**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **Tasso di Successo Invio per Provider**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### Metriche di Prestazioni

1. **Durata Media Invio**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **Durata Invio P95**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Latenza Redis P99**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### Metriche di Errore

1. **Tasso di Hit Limitazione Velocità**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **Motivi di Fallimento Verifica**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **Tasso di Fallimento Invio**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## Regole di Alerting

### Esempi di Regole di Alert

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # Alto tasso di fallimento
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Il tasso di fallimento della verifica Herald è alto"
          description: "Il tasso di fallimento della verifica è {{ $value | humanizePercentage }}"

      # Alto tasso di fallimento invio
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Il tasso di fallimento dell'invio Herald è alto"
          description: "Il tasso di fallimento dell'invio è {{ $value | humanizePercentage }}"

      # Alta latenza Redis
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "La latenza Redis Herald è alta"
          description: "La latenza Redis P99 è {{ $value }}s"

      # Alto numero di hit limitazione velocità
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Gli hit di limitazione della velocità Herald sono alti"
          description: "Il tasso di hit di limitazione della velocità è {{ $value }} hit/secondo"
```

## Dashboard Grafana

### Pannelli Dashboard Consigliati

1. **Pannello Panoramica**
   - Tasso di creazione challenge
   - Tasso di successo verifica
   - Tasso di successo invio
   - Challenge attive correnti (se tracciate)

2. **Pannello Prestazioni**
   - Durata invio (P50, P95, P99)
   - Latenza Redis (P50, P95, P99)
   - Tasso di richiesta

3. **Pannello Errori**
   - Suddivisione motivi di fallimento verifica
   - Tasso di fallimento invio per provider
   - Hit di limitazione velocità per ambito

4. **Pannello Provider**
   - Tasso di successo invio per provider
   - Durata invio per provider
   - Volume invio per provider

## Supporto OpenTelemetry

**Nota**: Il supporto OpenTelemetry per il tracciamento distribuito è pianificato ma non ancora implementato. Questo include:
- Propagazione header `traceparent` / `tracestate`
- Span principali: `otp.challenge.create`, `otp.provider.send`, `otp.verify`
- Tag span: `channel`, `purpose`, `provider`, `result`, `reason`

Vedere la roadmap del progetto per la timeline di implementazione.

## Migliori Pratiche

1. **Monitorare Metriche di Business Chiave**: Tracciare la creazione challenge, i tassi di successo verifica e i tassi di successo invio
2. **Configurare Alert**: Configurare alert per alti tassi di fallimento e degrado delle prestazioni
3. **Tracciare Prestazioni Provider**: Monitorare durata invio e tassi di successo per provider
4. **Monitorare Salute Redis**: Tracciare latenza Redis e problemi di connessione
5. **Monitoraggio Limitazione Velocità**: Monitorare hit di limitazione velocità per comprendere pattern di utilizzo e potenziali abusi

## Documentazione Correlata

- [Documentazione API](API.md) - Dettagli endpoint API
- [Guida al Deployment](DEPLOYMENT.md) - Deployment e configurazione
- [Guida alla Risoluzione dei Problemi](TROUBLESHOOTING.md) - Problemi comuni e soluzioni
