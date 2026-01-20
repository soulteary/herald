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
| `EMAIL_API_URL` | Email provider API endpoint | `` | For email |
| `EMAIL_API_KEY` | Email provider API key | `` | Optional |
| `EMAIL_FROM` | Email sender address | `` | For email |
| `SMS_API_URL` | SMS provider API endpoint | `` | For SMS |
| `SMS_API_KEY` | SMS provider API key | `` | Optional |
| `PROVIDER_TIMEOUT` | External provider timeout | `5s` | No |

## Integration with Stargate

1. Set `HERALD_URL` in Stargate configuration
2. Set `HERALD_API_KEY` in Stargate configuration
3. Set `HERALD_ENABLED=true` in Stargate configuration

Example:
```bash
export HERALD_URL=http://herald:8082
export HERALD_API_KEY=your-secret-key
export HERALD_ENABLED=true
```

## Security

- Use HMAC authentication for production
- Set strong API keys
- Use TLS/HTTPS in production
- Configure rate limits appropriately
- Monitor Redis for suspicious activity
