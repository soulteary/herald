# Herald Monitoring Guide

Herald exposes Prometheus metrics for monitoring and observability. This document describes all available metrics, how to configure monitoring, and best practices.

## Metrics Endpoint

Herald exposes Prometheus metrics at the `/metrics` endpoint:

```
GET http://localhost:8082/metrics
```

## Available Metrics

### Challenge Metrics

#### `herald_otp_challenges_total`

Counter tracking the total number of OTP challenges created.

**Labels:**
- `channel`: Channel type (`sms` or `email`)
- `purpose`: Purpose of the challenge (e.g., `login`, `reset`, `bind`)
- `result`: Result of the operation (`success` or `failed`)

**Example:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### Send Metrics

#### `herald_otp_sends_total`

Counter tracking the total number of OTP sends via providers.

**Labels:**
- `channel`: Channel type (`sms` or `email`)
- `provider`: Provider name (e.g., `smtp`, `aliyun`, `placeholder`)
- `result`: Result of the send operation (`success` or `failed`)

**Example:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

Histogram tracking the duration of OTP send operations.

**Labels:**
- `provider`: Provider name

**Buckets:** Default Prometheus buckets (0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)

**Example:**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### Verification Metrics

#### `herald_otp_verifications_total`

Counter tracking the total number of OTP verifications.

**Labels:**
- `result`: Verification result (`success` or `failed`)
- `reason`: Failure reason (e.g., `expired`, `invalid`, `locked`, `too_many_attempts`)

**Example:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
```

### Rate Limiting Metrics

#### `herald_rate_limit_hits_total`

Counter tracking the total number of rate limit hits.

**Labels:**
- `scope`: Rate limit scope (`user`, `ip`, `destination`, `resend_cooldown`)

**Example:**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Redis Metrics

#### `herald_redis_latency_seconds`

Histogram tracking Redis operation latency.

**Labels:**
- `operation`: Redis operation type (`get`, `set`, `del`, `exists`)

**Buckets:** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**Example:**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Prometheus Configuration

### Basic Scrape Configuration

Add the following to your `prometheus.yml`:

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

For Kubernetes deployments, use service discovery:

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

## Key Metrics to Monitor

### Business Metrics

1. **Challenge Creation Rate**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **Challenge Success Rate**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **Verification Success Rate**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **Send Success Rate by Provider**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

### Performance Metrics

1. **Average Send Duration**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **P95 Send Duration**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Redis P99 Latency**
   ```
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### Error Metrics

1. **Rate Limit Hit Rate**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **Verification Failure Reasons**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **Send Failure Rate**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   ```

## Alerting Rules

### Example Alert Rules

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # High failure rate
      - alert: HeraldHighVerificationFailureRate
        expr: |
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald verification failure rate is high"
          description: "Verification failure rate is {{ $value | humanizePercentage }}"

      # High send failure rate
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald send failure rate is high"
          description: "Send failure rate is {{ $value | humanizePercentage }}"

      # High Redis latency
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald Redis latency is high"
          description: "P99 Redis latency is {{ $value }}s"

      # High rate limit hits
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Herald rate limit hits are high"
          description: "Rate limit hit rate is {{ $value }} hits/second"
```

## Grafana Dashboards

### Recommended Dashboard Panels

1. **Overview Panel**
   - Challenge creation rate
   - Verification success rate
   - Send success rate
   - Current active challenges (if tracked)

2. **Performance Panel**
   - Send duration (P50, P95, P99)
   - Redis latency (P50, P95, P99)
   - Request rate

3. **Error Panel**
   - Verification failure reasons breakdown
   - Send failure rate by provider
   - Rate limit hits by scope

4. **Provider Panel**
   - Send success rate by provider
   - Send duration by provider
   - Send volume by provider

## OpenTelemetry Support

**Note**: OpenTelemetry support for distributed tracing is planned but not yet implemented. This includes:
- `traceparent` / `tracestate` header propagation
- Core spans: `otp.challenge.create`, `otp.provider.send`, `otp.verify`
- Span tags: `channel`, `purpose`, `provider`, `result`, `reason`

See the project roadmap for implementation timeline.

## Best Practices

1. **Monitor Key Business Metrics**: Track challenge creation, verification success rates, and send success rates
2. **Set Up Alerts**: Configure alerts for high failure rates and performance degradation
3. **Track Provider Performance**: Monitor send duration and success rates by provider
4. **Monitor Redis Health**: Track Redis latency and connection issues
5. **Rate Limit Monitoring**: Monitor rate limit hits to understand usage patterns and potential abuse

## Related Documentation

- [API Documentation](API.md) - API endpoint details
- [Deployment Guide](DEPLOYMENT.md) - Deployment and configuration
- [Troubleshooting Guide](TROUBLESHOOTING.md) - Common issues and solutions
