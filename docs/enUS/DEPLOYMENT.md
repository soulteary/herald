# Herald Deployment Guide

## Quick Start

### Using Docker Compose

```bash
cd herald
docker-compose up -d
```

### Manual Deployment

```bash
# Build
go build -o herald main.go

# Run
./herald
```

## Configuration

### Configuration Naming Convention

**Note**: While the specification recommends using `HERALD_*` prefix for all configuration variables, the current implementation uses a mix of naming conventions:
- Some variables use `HERALD_*` prefix (e.g., `HERALD_TEST_MODE`, `HERALD_SESSION_STORAGE_ENABLED`)
- Some variables use shorter names (e.g., `REDIS_ADDR`, `API_KEY`, `HMAC_SECRET`)

For consistency and future compatibility, consider using `HERALD_*` prefix when possible. The service supports both naming conventions.

### Environment Variables

The following match the implementation in `internal/config/config.go`.

#### Server and Redis

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server listen port (with or without leading colon, e.g. `8082` or `:8082`) | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | No |
| `REDIS_PASSWORD` | Redis password | (empty) | No |
| `REDIS_DB` | Redis database index | `0` | No |
| `LOG_LEVEL` | Log level | `info` | No |
| `SERVICE_NAME` | Service identifier (HMAC, logging, health) | `herald` | No |

#### Service-to-service authentication

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `API_KEY` | Simple API key auth (via header) | (empty) | One recommended |
| `HMAC_SECRET` | Single HMAC secret for request signing | (empty) | One recommended |
| `HERALD_HMAC_KEYS` | Multiple HMAC keys, JSON: `{"key-id-1":"secret-1","key-id-2":"secret-2"}`; supports key rotation | (empty) | One recommended |

If none are set, the service logs a warning and allows unauthenticated requests (dev/test only).

#### OTP / Challenge

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CHALLENGE_EXPIRY` | Challenge expiry (e.g. `5m`, `300s`) | `5m` | No |
| `MAX_ATTEMPTS` | Max verify failures per challenge before lockout | `5` | No |
| `LOCKOUT_DURATION` | Lockout duration (e.g. `10m`) | `10m` | No |
| `RESEND_COOLDOWN` | Resend cooldown for same challenge | `60s` | No |
| `CODE_LENGTH` | Verification code length (digits) | `6` | No |
| `IDEMPOTENCY_KEY_TTL` | Idempotency key cache TTL; `0` = use `CHALLENGE_EXPIRY` | `0` | No |
| `ALLOWED_PURPOSES` | Allowed purposes, comma-separated (e.g. `login,reset,bind,stepup`) | `login` | No |

#### Rate limiting

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `RATE_LIMIT_PER_USER` | Challenges per user_id per hour | `10` | No |
| `RATE_LIMIT_PER_IP` | Challenges per IP per minute | `5` | No |
| `RATE_LIMIT_PER_DESTINATION` | Challenges per destination (email/phone) per hour | `10` | No |

#### Email channel

**Built-in SMTP** (used when `HERALD_SMTP_API_URL` is not set):

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SMTP_HOST` | SMTP server host | (empty) | When using built-in |
| `SMTP_PORT` | SMTP port | `587` | No |
| `SMTP_USER` | SMTP username | (empty) | No |
| `SMTP_PASSWORD` | SMTP password | (empty) | No |
| `SMTP_FROM` | From address | (empty) | Recommended |
| `PROVIDER_FAILURE_POLICY` | On send failure: `soft` (still create challenge) or `strict` (do not create) | `soft` | No |

**herald-smtp plugin** (when set, built-in SMTP is not used):

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_SMTP_API_URL` | [herald-smtp](https://github.com/soulteary/herald-smtp) base URL (e.g. `http://herald-smtp:8084`) | (empty) | When using plugin |
| `HERALD_SMTP_API_KEY` | Must match herald-smtp `API_KEY` if set | (empty) | No |

#### SMS channel (HTTP API mode)

The implementation calls an external SMS HTTP API; Aliyun/etc. credentials are held by that gateway, not Herald.

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SMS_PROVIDER` | Provider name (e.g. `aliyun`, `tencent`, `http`) for logging | (empty) | When using SMS |
| `SMS_API_BASE_URL` | SMS HTTP API base URL | (empty) | When using SMS |
| `SMS_API_KEY` | SMS API auth key if required by gateway | (empty) | As needed |

#### DingTalk channel (herald-dingtalk plugin)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) base URL (e.g. `http://herald-dingtalk:8083`) | (empty) | When using DingTalk |
| `HERALD_DINGTALK_API_KEY` | Must match herald-dingtalk `API_KEY` if set | (empty) | No |

#### TLS / mTLS

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `TLS_CERT_FILE` | Server certificate path | (empty) | When TLS enabled |
| `TLS_KEY_FILE` | Server private key path | (empty) | When TLS enabled |
| `TLS_CA_CERT_FILE` | Client CA cert for mTLS verification | (empty) | Optional |
| `TLS_CLIENT_CA_FILE` | Alias for `TLS_CA_CERT_FILE` | (empty) | Optional |

#### Session storage (optional)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_SESSION_STORAGE_ENABLED` | Enable Redis session storage | `false` | No |
| `HERALD_SESSION_DEFAULT_TTL` | Default session TTL (e.g. `1h`) | `1h` | No |
| `HERALD_SESSION_KEY_PREFIX` | Redis session key prefix | `session:` | No |

#### Audit logging

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `AUDIT_ENABLED` | Enable audit | `true` | No |
| `AUDIT_MASK_DESTINATION` | Mask destination in audit | `false` | No |
| `AUDIT_TTL` | Audit record TTL in Redis (e.g. `168h` = 7 days) | `168h` | No |
| `AUDIT_STORAGE_TYPE` | Persistent storage: `database`, `file`, `loki`, or comma-separated | (empty) | No |
| `AUDIT_DATABASE_URL` | DB URL when `AUDIT_STORAGE_TYPE` includes database | (empty) | No |
| `AUDIT_TABLE_NAME` | Audit table name | `audit_logs` | No |
| `AUDIT_FILE_PATH` | Audit file path when using file | (empty) | No |
| `AUDIT_LOKI_URL` | Loki URL when using loki | (empty) | No |
| `AUDIT_WRITER_QUEUE_SIZE` | Audit writer queue size | `1000` | No |
| `AUDIT_WRITER_WORKERS` | Audit writer workers | `2` | No |

#### Templates and observability

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `TEMPLATE_DIR` | Optional path to email/SMS template directory | (empty) | No |
| `OTLP_ENABLED` | Enable OpenTelemetry | `false` | No |
| `OTLP_ENDPOINT` | OTLP endpoint (e.g. `http://localhost:4318`) | (empty) | When OTLP enabled |

#### Test and debug

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_TEST_MODE` | When `true`, store code in Redis for `GET /v1/test/code/:id` and optional `debug_code` in create response; **must be false in production** | `false` | No |

### Test mode and debugging

When `HERALD_TEST_MODE=true`:

- The create-challenge response may include a `debug_code` field (plain verification code) so callers (e.g. Stargate with `DEBUG=true`) can display it on the login page.
- The endpoint `GET /v1/test/code/:challenge_id` is enabled and returns the verification code for the given challenge (for integration tests or as a fallback when the client does not receive `debug_code` in the create response).

**Security:** Always set `HERALD_TEST_MODE=false` in production. Test mode exposes verification codes and must not be used in production environments.

### Email channel (herald-smtp)

When `HERALD_SMTP_API_URL` is set, Herald does not use built-in SMTP. It forwards email sends to [herald-smtp](https://github.com/soulteary/herald-smtp) over HTTP. All SMTP credentials and sending logic live in herald-smtp; Herald does not store any SMTP credentials for the email channel in this mode.

- Set `HERALD_SMTP_API_URL` to the base URL of your herald-smtp service (e.g. `http://herald-smtp:8084`).
- If herald-smtp is configured with `API_KEY`, set `HERALD_SMTP_API_KEY` to the same value so Herald can authenticate when calling herald-smtp.
- When `HERALD_SMTP_API_URL` is set, Herald ignores `SMTP_HOST` and related built-in SMTP settings (they are not used for the email channel).

### DingTalk channel (herald-dingtalk)

When `channel` is `dingtalk`, Herald does not send messages itself. It forwards the send to [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) over HTTP. All DingTalk credentials and business logic live in herald-dingtalk; Herald does not store any DingTalk credentials.

- Set `HERALD_DINGTALK_API_URL` to the base URL of your herald-dingtalk service (e.g. `http://herald-dingtalk:8083`).
- If herald-dingtalk is configured with `API_KEY`, set `HERALD_DINGTALK_API_KEY` to the same value so Herald can authenticate when calling herald-dingtalk.

### Redis Configuration

#### Independent Redis Instance

For production deployments, it is **strongly recommended** to use a dedicated Redis instance or a separate database index for Herald to avoid key space conflicts with other services.

**Key Prefixes:**
- `otp:ch:*` - Challenge data (TTL: challenge expiry time)
- `otp:rate:*` - Rate limiting counters (TTL: rate limit window)
- `otp:idem:*` - Idempotency records (TTL: idempotency key TTL)
- `otp:lock:*` - User lockout records (TTL: lockout duration)

**Capacity Planning:**

Basic estimation formula:
```
Peak challenges/second × TTL (seconds) × Average size per entry + Rate limit keys + Audit keys
```

**Example:**
- Peak: 100 challenges/second
- Challenge TTL: 300 seconds (5 minutes)
- Average challenge size: ~500 bytes
- Estimated: 100 × 300 × 500 bytes ≈ 15 MB for challenges
- Plus rate limit keys (~1-2 MB) and audit keys (~1-2 MB)
- **Total estimated: ~20 MB minimum**

For high-availability deployments, consider Redis Cluster or Redis Sentinel.

## Integration with Other Services (Optional)

Herald is designed to work independently and can be integrated with other services as needed. If you want to integrate Herald with other authentication or gateway services, you can configure the following:

**Example integration configuration:**
```bash
# Service URL where Herald is accessible
export HERALD_URL=http://herald:8082

# API key for service-to-service authentication
export HERALD_API_KEY=your-secret-key

# Enable Herald integration (if your service supports it)
export HERALD_ENABLED=true
```

**Note**: Herald can be used standalone without any external service dependencies. Integration with other services is optional and depends on your specific use case.

## Monitoring

### Prometheus Metrics

Herald exposes Prometheus metrics at the `/metrics` endpoint. The following metrics are available:

- `herald_otp_challenges_total{channel,purpose,result}` - Total number of OTP challenges created
- `herald_otp_sends_total{channel,provider,result}` - Total number of OTP sends via providers
- `herald_otp_verifications_total{result,reason}` - Total number of OTP verifications
- `herald_otp_send_duration_seconds{provider}` - Duration of OTP send operations (Histogram)
- `herald_rate_limit_hits_total{scope}` - Total number of rate limit hits (scope: user, ip, destination, resend_cooldown)
- `herald_redis_latency_seconds{operation}` - Redis operation latency (operation: get, set, del, exists)

**Example Prometheus scrape configuration:**
```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
```

For detailed monitoring documentation, see [MONITORING.md](MONITORING.md).

## Security

- Use mTLS or HMAC authentication for production (mTLS is recommended for highest security)
- Set strong API keys
- Use TLS/HTTPS in production
- Configure rate limits appropriately
- Monitor Redis for suspicious activity
