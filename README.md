# Herald - OTP and Verification Code Service

> **📧 Your Gateway to Secure Verification**

![Herald](.github/assets/banner.jpg)

Herald is a production-ready, lightweight service for sending verification codes (OTP) via SMS and email, with built-in rate limiting, security controls, and audit logging.

## Features

- 🚀 **High Performance**: Built with Go and Fiber
- 🔒 **Secure**: Challenge-based verification with hash storage
- 📊 **Rate Limiting**: Multi-dimensional rate limiting (per user, per IP, per destination)
- 📝 **Audit Logging**: Complete audit trail for all operations
- 🔌 **Pluggable Providers**: Support for multiple SMS and email providers
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

## API Documentation

See [API.md](docs/API.md) for detailed API documentation.
