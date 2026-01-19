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
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
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
| `SMTP_HOST` | SMTP server host | `` | For email |
| `SMTP_PORT` | SMTP server port | `587` | For email |
| `SMTP_USER` | SMTP username | `` | For email |
| `SMTP_PASSWORD` | SMTP password | `` | For email |
| `SMTP_FROM` | SMTP from address | `` | For email |
| `SMS_PROVIDER` | SMS provider | `` | For SMS |
| `ALIYUN_ACCESS_KEY` | Aliyun access key | `` | For Aliyun SMS |
| `ALIYUN_SECRET_KEY` | Aliyun secret key | `` | For Aliyun SMS |

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
