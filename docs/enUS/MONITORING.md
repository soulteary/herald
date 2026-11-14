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
- `channel`: Channel type (`sms`, `email`, or `dingtalk`)
- `purpose`: Purpose of the challenge (e.g., `login`, `reset`, `bind`)
- `result`: Currently emitted as `success` after challenge storage, before delivery.
  This counter does not measure successful HTTP creation responses.

**Example:**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
```

### Send Metrics

#### `herald_otp_sends_total`

Counter tracking the total number of OTP sends via providers.

**Labels:**
- `channel`: Channel type (`sms`, `email`, or `dingtalk`)
- `provider`: Provider name (e.g., `smtp`, `aliyun`, `placeholder`)
- `result`: Result of the send operation (`success` or `failure`)

**Example:**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failure"} 10
```

#### `herald_otp_send_duration_seconds`

Histogram tracking the duration of OTP send operations.

**Labels:**
- `provider`: Provider name

**Buckets:** External API buckets (0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30)

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
- `result`: Verification result (`success` or `failure`)
- `reason`: Failure reason (e.g., `expired`, `invalid`, `locked`)

**Example:**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failure",reason="expired"} 50
herald_otp_verifications_total{result="failure",reason="invalid"} 30
herald_otp_verifications_total{result="failure",reason="locked"} 5
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

#### `herald_redis_operation_duration_seconds`

Histogram tracking Redis operation latency.

**Labels:**
- `operation`: Instrumented wrapper; not every Redis command is instrumented. Challenge creation applies its limits and cooldowns in one call reported as `ratelimit_create`. The per-limiter wrappers (`ratelimit`, `ratelimit_user`, `ratelimit_ip`, `ratelimit_dest`, `cooldown`) are no longer used by challenge creation. `cooldown_release` is the call that gives back the resend cooldowns after a send that provably reached no provider

**Buckets:** [0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0]

**Example:**
```
herald_redis_operation_duration_seconds_bucket{operation="ratelimit_user",le="0.01"} 5000
herald_redis_operation_duration_seconds_sum{operation="ratelimit_user"} 12.5
herald_redis_operation_duration_seconds_count{operation="ratelimit_user"} 10000
```

## Prometheus Configuration

### Basic Scrape Configuration

Provision a separate `HERALD_METRICS_API_KEY` and mount its value at the scraper credential file below. Production rejects anonymous scraping; business API/HMAC keys must not be reused.

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
    scrape_interval: 15s
    scrape_timeout: 10s
```

### Service Discovery

For Kubernetes deployments, use service discovery:

```yaml
scrape_configs:
  - job_name: 'herald'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
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

2. **Created Challenge Rate by Channel and Purpose**
   ```
   sum by (channel, purpose) (rate(herald_otp_challenges_total{result="success"}[5m]))
   ```

3. **Verification Success Rate**
   ```
   sum(rate(herald_otp_verifications_total{result="success"}[5m])) / sum(rate(herald_otp_verifications_total[5m]))
   ```

4. **Send Success Rate by Provider**
   ```
   sum by (provider) (rate(herald_otp_sends_total{result="success"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m]))
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
   histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m]))
   ```

### Error Metrics

1. **Rate Limit Hit Rate**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **Verification Failure Reasons**
   ```
   sum by (reason) (rate(herald_otp_verifications_total{result="failure"}[5m]))
   ```

3. **Send Failure Rate**
   ```
   sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m]))
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
          sum(rate(herald_otp_verifications_total{result="failure"}[5m])) / sum(rate(herald_otp_verifications_total[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald verification failure rate is high"
          description: "Verification failure rate is {{ $value | humanizePercentage }}"

      # High send failure rate
      - alert: HeraldHighSendFailureRate
        expr: |
          sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m])) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald send failure rate is high"
          description: "Send failure rate is {{ $value | humanizePercentage }}"

      # High Redis latency
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m])) > 0.1
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

When `OTLP_ENABLED=true` and `OTLP_ENDPOINT` is set, Herald enables OpenTelemetry tracing middleware. Trace context is propagated via `traceparent` and `tracestate` headers. Core spans include challenge create, provider send, and verify operations; span tags include `channel`, `purpose`, `provider`, `result`, and `reason`. Users are identified on spans by `user_id_digest`, a peppered digest (keyed by `HERALD_PII_PEPPER`); the raw `user_id` is not exported. The digest is stable for a given pepper, so it can still be used to correlate spans for one user. The creation span carries the destination as `destination_digest` in the same way, and no span carries the client address. Configure `OTLP_ENDPOINT` as `host:port` (for example `localhost:4318`) or as an `http://` or `https://` URL (for example `http://localhost:4318`), as described in [DEPLOYMENT.md](DEPLOYMENT.md); spans are sent with OTLP over HTTP. Herald logs `OpenTelemetry tracing initialized` with the resolved endpoint only when an exporter was set up. With an empty endpoint, or one it cannot parse, it logs that no spans are exported.

The request span of each HTTP request is named `<METHOD> <route template>`, for
example `POST /v1/otp/challenges/:id/revoke`, and carries the template as
`http.route`. A request that matches no route is named `<METHOD>
unmatched_route` and has no `http.route`. The request path itself is not
exported, neither in the span name nor as `http.target`, because it contains
caller-supplied segments such as a challenge ID.

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

## Audit delivery

Alert on `herald_audit_write_failure_total`: backend or cleanup writes failed,
but undelivered records remain in the local buffer for retry. Monitor pending
`*.json` files and free space in `AUDIT_SPOOL_DIR`; sustained growth can reach
`AUDIT_SPOOL_MAX_BYTES` and reject new audit events.

`herald_audit_dropped_total` now counts events rejected before durable local
acceptance. Such failures return `503/audit_unavailable`, except the
post-action failures described below; it no longer counts
recoverable backend delivery failures. Verify that retries drain the buffer
after storage recovery. Never delete pending files to silence an alert.

Alert on any increase of `herald_audit_quarantined_total`. It counts pending
files that were read and could not be decoded in 5 consecutive attempts, for
example a file truncated by disk corruption. Such a record cannot be delivered. Herald renames
it from `<id>.json` to `<id>.json.quarantined` in `AUDIT_SPOOL_DIR`, logs an
error naming the file, and stops retrying it; the file no longer counts towards
`AUDIT_SPOOL_MAX_BYTES`. The counter restarts at zero with the process, so a
startup warning also reports how many quarantined files the directory holds.
Inspect and archive these files; they are the only copy of those audit events.

A pending file that cannot be read at all is never quarantined. It is retried
with a delay that doubles from 1 second up to 5 minutes and is delivered once
it can be read. To keep a lasting fault from flooding the log, only the 1st,
2nd, 4th, 8th (and so on) consecutive failed read of a file writes an error
line and increments `herald_audit_write_failure_total`; the line names the
file and the next retry delay. Such a file still counts towards
`AUDIT_SPOOL_MAX_BYTES`.

Alert on `herald_audit_post_action_failure_total{operation}` (`create`,
`verify`, `revoke`): a result record could not be accepted after the message
was sent, the code was consumed or the revoke completed. The request was
answered normally, so the event is missing from the audit trail. Each
occurrence also increments `herald_audit_dropped_total` and writes one error
log line with `stage="post_action"`, the event type, challenge ID, result,
reason and `user_id_digest`; use that line to reconstruct the record.

## Provider failure diagnostics

Delivery failures are logged with a bounded `reason` field, not the upstream
message. Span status and exception events use the same safe reason. Do not
enable raw provider-body logging to investigate failures. Review historical
logs and exported spans for echoed sensitive data and rotate exposed credentials.
See [security](SECURITY.md#provider-failure-diagnostics) for compatibility details.

## Authentication and idempotency outcomes

- `herald_idempotency_total{result}` emits `replay`, `conflict`, `in_flight`, `delivery_unknown`, `ownership_lost`, or `backend_error`. Both fingerprint conflicts and legacy namespace conflicts use `conflict`; distinguish them by the API reason, without adding identity labels.
- `herald_nonce_replay_total` counts rejected replayed nonces.
- `herald_auth_failure_total{reason}` counts HMAC requests answered `401/invalid_signature`. `reason` is `invalid_key_id` (the key ID is not configured) or `invalid_signature` (the key is known and the signature is wrong). The HTTP response is the same for both. Requests on the legacy v1 scheme (`HMAC_V1_ENABLED=true`) that name an unconfigured key ID are counted as `invalid_key_id` and answered like a wrong signature as well; wrong signatures for a configured key are not counted on that scheme.
- `herald_provider_timeout_total{channel}` counts provider timeouts.
- `herald_verification_contention_total{result}` records `contended` or `acquired` where emitted by the verification path.
- `herald_failed_attempt_lock_total` counts users locked because their wrong codes reached `LOCKOUT_FAILED_ATTEMPT_THRESHOLD`. Each lock also writes a warning with `user_id_digest`. A steady rate for one digest indicates code guessing against that user.
- `herald_challenge_state_total{state}` records emitted transitions such as `activated`, `superseded`, and `revoked`.
- `herald_redis_operations_total{operation,result}` uses `success` or `failure`; the duration histogram's count includes both outcomes and is not an error counter.

There is no separate hash-budget occupancy metric. Observe overload through
`503/backend_unavailable`, retry headers, runtime memory and request outcomes.
Do not add raw service/key IDs, destinations or client keys as metric labels.

### Metrics authentication

In production, `/metrics` requires a separate `HERALD_METRICS_API_KEY` of at
least 32 bytes. If no metrics key is configured, the endpoint returns 401.
Business API keys do not grant scraping access, and the metrics key does not
grant OTP or TOTP access. Generate independent random credentials for each.
Startup rejects reuse of the metrics key as an API key or any configured HMAC
secret, including in development and test environments.

Development and test environments allow anonymous scraping only when the metrics
key is unset. Configuring a key protects scraping in every environment. Acceptable
headers are `Authorization: Bearer <key>` and `X-Metrics-API-Key: <key>`; query
parameters are not accepted.

Before upgrading production, provision the key and update the scraper together:

```yaml
scrape_configs:
  - job_name: herald
    static_configs:
      - targets: ['herald:8082']
    metrics_path: /metrics
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
```

Keep the endpoint on a private network or restrict it at the reverse proxy even
when authentication is enabled. Liveness and readiness probes are unchanged.
