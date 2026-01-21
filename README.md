# Herald - OTP and Verification Code Service

> **📧 Your Gateway to Secure Verification**

## 🌐 Multi-language Documentation

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald is a production-ready, lightweight service for sending verification codes (OTP) via email (SMS support is currently in development), with built-in rate limiting, security controls, and audit logging.

## Features

- 🚀 **High Performance**: Built with Go and Fiber
- 🔒 **Secure**: Challenge-based verification with hash storage
- 📊 **Rate Limiting**: Multi-dimensional rate limiting (per user, per IP, per destination)
- ♻️ **Idempotency**: Prevent duplicate challenge creation and sends
- 📈 **Metrics**: Prometheus-compatible metrics endpoint
- 📝 **Operational Logs**: Key events and errors for troubleshooting
- 🔌 **Pluggable Providers**: Support for email providers (SMS providers are placeholder implementations and not yet fully functional)
- ⚡ **Redis Backend**: Fast, distributed storage with Redis

## Quick Start

```bash
# Run with Docker Compose
docker-compose up -d

# Or run directly
go run main.go
```

## Configuration

Set environment variables:

- `PORT`: Server port (default: `:8082`)
- `REDIS_ADDR`: Redis address (default: `localhost:6379`)
- `REDIS_PASSWORD`: Redis password (optional)
- `REDIS_DB`: Redis database number (default: `0`)
- `API_KEY`: API key for service-to-service authentication
- `LOG_LEVEL`: Log level (default: `info`)

For complete configuration options, see [DEPLOYMENT.md](docs/enUS/DEPLOYMENT.md).

## API Documentation

See [API.md](docs/enUS/API.md) for detailed API documentation.
