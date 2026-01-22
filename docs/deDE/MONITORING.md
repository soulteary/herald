# Herald Monitoring-Leitfaden

Herald stellt Prometheus-Metriken für Monitoring und Beobachtbarkeit bereit. Dieses Dokument beschreibt alle verfügbaren Metriken, wie Sie das Monitoring konfigurieren und Best Practices.

## Metriken-Endpunkt

Herald stellt Prometheus-Metriken am `/metrics`-Endpunkt bereit:

```
GET http://localhost:8082/metrics
```

## Verfügbare Metriken

### Challenge-Metriken

#### `herald_otp_challenges_total`

Zähler, der die Gesamtzahl der erstellten OTP-Challenges verfolgt.

**Labels:**
- `channel`: Kanaltyp (`sms` oder `email`)
- `purpose`: Zweck der Challenge (z.B. `login`, `reset`, `bind`)
- `result`: Ergebnis der Operation (`success` oder `failed`)

**Beispiel:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### Send-Metriken

#### `herald_otp_sends_total`

Zähler, der die Gesamtzahl der OTP-Sendungen über Provider verfolgt.

**Labels:**
- `channel`: Kanaltyp (`sms` oder `email`)
- `provider`: Provider-Name (z.B. `smtp`, `aliyun`, `placeholder`)
- `result`: Ergebnis der Send-Operation (`success` oder `failed`)

**Beispiel:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

Histogramm, das die Dauer von OTP-Send-Operationen verfolgt.

**Labels:**
- `provider`: Provider-Name

**Buckets:** Standard-Prometheus-Buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Beispiel:**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### Verifizierungs-Metriken

#### `herald_otp_verifications_total`

Zähler, der die Gesamtzahl der OTP-Verifizierungen verfolgt.

**Labels:**
- `result`: Verifizierungsergebnis (`success` oder `failed`)
- `reason`: Fehlergrund (z.B. `expired`, `invalid`, `locked`, `too_many_attempts`)

**Beispiel:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### Rate-Limiting-Metriken

#### `herald_rate_limit_hits_total`

Zähler, der die Gesamtzahl der Rate-Limit-Treffer verfolgt.

**Labels:**
- `scope`: Rate-Limit-Bereich (`user`, `ip`, `destination`, `resend_cooldown`)

**Beispiel:**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Redis-Metriken

#### `herald_redis_latency_seconds`

Histogramm, das die Redis-Operationslatenz verfolgt.

**Labels:**
- `operation`: Redis-Operationstyp (`get`, `set`, `del`, `exists`)

**Buckets:** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**Beispiel:**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Prometheus-Konfiguration

### Grundlegende Scrape-Konfiguration

Fügen Sie Folgendes zu Ihrer `prometheus.yml` hinzu:

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

Für Kubernetes-Bereitstellungen verwenden Sie Service Discovery:

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

## Wichtige zu überwachende Metriken

### Geschäftsmetriken

1. **Challenge-Erstellungsrate**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **Challenge-Erfolgsrate**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **Verifizierungs-Erfolgsrate**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **Send-Erfolgsrate nach Provider**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### Leistungsmetriken

1. **Durchschnittliche Send-Dauer**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **P95 Send-Dauer**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Redis P99 Latenz**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### Fehlermetriken

1. **Rate-Limit-Trefferrate**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **Verifizierungs-Fehlergründe**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **Send-Fehlerrate**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## Alerting-Regeln

### Beispiel-Alert-Regeln

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # Hohe Fehlerrate
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald-Verifizierungsfehlerrate ist hoch"
          description: "Verifizierungsfehlerrate ist {{ $value | humanizePercentage }}"

      # Hohe Send-Fehlerrate
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald-Send-Fehlerrate ist hoch"
          description: "Send-Fehlerrate ist {{ $value | humanizePercentage }}"

      # Hohe Redis-Latenz
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald-Redis-Latenz ist hoch"
          description: "P99 Redis-Latenz ist {{ $value }}s"

      # Hohe Rate-Limit-Treffer
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Herald-Rate-Limit-Treffer sind hoch"
          description: "Rate-Limit-Trefferrate ist {{ $value }} Treffer/Sekunde"
```

## Grafana-Dashboards

### Empfohlene Dashboard-Panels

1. **Übersichtspanel**
   - Challenge-Erstellungsrate
   - Verifizierungs-Erfolgsrate
   - Send-Erfolgsrate
   - Aktuelle aktive Challenges (falls verfolgt)

2. **Leistungspanel**
   - Send-Dauer (P50, P95, P99)
   - Redis-Latenz (P50, P95, P99)
   - Anforderungsrate

3. **Fehlerpanel**
   - Aufschlüsselung der Verifizierungs-Fehlergründe
   - Send-Fehlerrate nach Provider
   - Rate-Limit-Treffer nach Bereich

4. **Provider-Panel**
   - Send-Erfolgsrate nach Provider
   - Send-Dauer nach Provider
   - Send-Volumen nach Provider

## OpenTelemetry-Unterstützung

**Hinweis**: OpenTelemetry-Unterstützung für verteilte Tracing ist geplant, aber noch nicht implementiert. Dies umfasst:
- `traceparent` / `tracestate` Header-Propagierung
- Kern-Spans: `otp.challenge.create`, `otp.provider.send`, `otp.verify`
- Span-Tags: `channel`, `purpose`, `provider`, `result`, `reason`

Siehe die Projekt-Roadmap für den Implementierungszeitplan.

## Best Practices

1. **Überwachen Sie wichtige Geschäftsmetriken**: Verfolgen Sie Challenge-Erstellung, Verifizierungs-Erfolgsraten und Send-Erfolgsraten
2. **Richten Sie Alerts ein**: Konfigurieren Sie Alerts für hohe Fehlerraten und Leistungsverschlechterung
3. **Verfolgen Sie Provider-Leistung**: Überwachen Sie Send-Dauer und Erfolgsraten nach Provider
4. **Überwachen Sie Redis-Gesundheit**: Verfolgen Sie Redis-Latenz und Verbindungsprobleme
5. **Rate-Limit-Monitoring**: Überwachen Sie Rate-Limit-Treffer, um Nutzungsmuster und potenziellen Missbrauch zu verstehen

## Verwandte Dokumentation

- [API-Dokumentation](API.md) - API-Endpunkt-Details
- [Bereitstellungsleitfaden](DEPLOYMENT.md) - Bereitstellung und Konfiguration
- [Fehlerbehebungsleitfaden](TROUBLESHOOTING.md) - Häufige Probleme und Lösungen
