# Herald - OTP and Verification Code Service

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![codecov](https://codecov.io/gh/soulteary/herald/branch/main/graph/badge.svg)](https://codecov.io/gh/soulteary/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/herald)](https://goreportcard.com/report/github.com/soulteary/herald)

> **📧 Your Gateway to Secure Verification**

## 🌐 Multi-language Documentation

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald is a production-ready, standalone OTP and verification code service that sends verification codes via email and SMS. It features built-in rate limiting, security controls, and audit logging. Herald is designed to work independently and can be integrated with other services as needed.

## Core Features

- 🔒 **Secure by Design**: Challenge-based verification with Argon2 hash storage, multiple authentication methods (mTLS, HMAC, API Key)
- 📊 **Built-in Rate Limiting**: Multi-dimensional rate limiting (per user, per IP, per destination) with configurable thresholds
- 📝 **Complete Audit Trail**: Full audit logging for all operations with provider tracking
- 🔌 **Pluggable Providers**: Extensible email and SMS provider architecture

## Quick Start

### Using Docker Compose

The easiest way to get started is with Docker Compose, which includes Redis:

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

Expected response:
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Test the API

Create a test challenge (requires authentication - see [API Documentation](docs/enUS/API.md)):

```bash
# Set your API key (from docker-compose.yml: your-secret-api-key-here)
export API_KEY="your-secret-api-key-here"

# Create a challenge
curl -X POST http://localhost:8082/v1/otp/challenges \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "test_user",
    "channel": "email",
    "destination": "user@example.com",
    "purpose": "login"
  }'
```

### View Logs

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### Manual Deployment

For manual deployment and advanced configuration, see the [Deployment Guide](docs/enUS/DEPLOYMENT.md).

## Basic Configuration

Herald requires minimal configuration to get started:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | Yes |
| `API_KEY` | API key for authentication | - | Recommended |

For complete configuration options including rate limits, challenge expiry, and provider settings, see the [Deployment Guide](docs/enUS/DEPLOYMENT.md#configuration).

## Documentation

### For Developers

- **[API Documentation](docs/enUS/API.md)** - Complete API reference with authentication methods, endpoints, and error codes
- **[Deployment Guide](docs/enUS/DEPLOYMENT.md)** - Configuration options, Docker deployment, and integration examples

### For Operations

- **[Monitoring Guide](docs/enUS/MONITORING.md)** - Prometheus metrics, Grafana dashboards, and alerting
- **[Troubleshooting Guide](docs/enUS/TROUBLESHOOTING.md)** - Common issues, diagnostic steps, and solutions

### Documentation Index

For a complete overview of all documentation, see [docs/enUS/README.md](docs/enUS/README.md).

## License

See [LICENSE](LICENSE) for details.
