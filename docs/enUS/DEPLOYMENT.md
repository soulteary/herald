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

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port (can be with or without leading colon, e.g., `8082` or `:8082`) | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | No |
| `REDIS_PASSWORD` | Redis password | `` | No |
| `REDIS_DB` | Redis database | `0` | No |
| `API_KEY` | API key for authentication | `` | Recommended |
| `HMAC_SECRET` | HMAC secret for secure auth | `` | Optional |
| `LOG_LEVEL` | Log level | `info` | No |
| `CHALLENGE_EXPIRY` | Challenge expiration | `5m` | No |
| `MAX_ATTEMPTS` | Max verification attempts | `5` | No |
| `RESEND_COOLDOWN` | Resend cooldown | `60s` | No |
| `CODE_LENGTH` | Verification code length | `6` | No |
| `RATE_LIMIT_PER_USER` | Rate limit per user/hour | `10` | No |
| `RATE_LIMIT_PER_IP` | Rate limit per IP/minute | `5` | No |
| `RATE_LIMIT_PER_DESTINATION` | Rate limit per destination/hour | `10` | No |
| `LOCKOUT_DURATION` | User lockout duration after max attempts | `10m` | No |
| `SERVICE_NAME` | Service identifier for HMAC auth | `herald` | No |
| `SMTP_HOST` | SMTP server host | `` | For email |
| `SMTP_PORT` | SMTP server port | `587` | For email |
| `SMTP_USER` | SMTP username | `` | For email |
| `SMTP_PASSWORD` | SMTP password | `` | For email |
| `SMTP_FROM` | SMTP from address | `` | For email |
| `SMS_PROVIDER` | SMS provider | `` | For SMS |
| `ALIYUN_ACCESS_KEY` | Aliyun access key | `` | For Aliyun SMS |
| `ALIYUN_SECRET_KEY` | Aliyun secret key | `` | For Aliyun SMS |
| `ALIYUN_SIGN_NAME` | Aliyun SMS sign name | `` | For Aliyun SMS |
| `ALIYUN_TEMPLATE_CODE` | Aliyun SMS template code | `` | For Aliyun SMS |
| `HERALD_DINGTALK_API_URL` | Base URL of [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (e.g. `http://herald-dingtalk:8083`) | `` | For DingTalk channel |
| `HERALD_DINGTALK_API_KEY` | Optional API key; must match herald-dingtalk `API_KEY` when set | `` | No |

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
