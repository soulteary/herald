# Documentation Index

Welcome to the Herald OTP and Verification Code Service documentation.

## 🌐 Multi-language Documentation

- [English](README.md) | [中文](../zhCN/README.md) | [Français](../frFR/README.md) | [Italiano](../itIT/README.md) | [日本語](../jaJP/README.md) | [Deutsch](../deDE/README.md) | [한국어](../koKR/README.md)

## 📚 Document List

### Core Documents

- **[README.md](../../README.md)** - Project overview and quick start guide

### Detailed Documents

- **[API.md](API.md)** - Complete API endpoint documentation
  - Authentication methods
  - Health check endpoints
  - Challenge creation and verification
  - Rate limiting
  - Error codes and responses

- **[DEPLOYMENT.md](DEPLOYMENT.md)** - Deployment guide
  - Docker Compose deployment
  - Manual deployment
  - Configuration options
  - Optional integration with other services
  - Security best practices

- **[MONITORING.md](MONITORING.md)** - Monitoring guide
  - Prometheus metrics
  - Grafana dashboards
  - Alerting rules
  - Best practices

- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Troubleshooting guide
  - Common issues and solutions
  - Diagnostic steps
  - Performance optimization

## 🚀 Quick Navigation

### Getting Started

1. Read [README.md](../../README.md) to understand the project
2. Check the [Quick Start](../../README.md#quick-start) section
3. Refer to [Configuration](../../README.md#configuration) to configure the service

### Developers

1. Check [API.md](API.md) to understand the API interfaces
2. Review [DEPLOYMENT.md](DEPLOYMENT.md) for deployment options

### Operations

1. Read [DEPLOYMENT.md](DEPLOYMENT.md) to understand deployment methods
2. Check [API.md](API.md) for API endpoint details
3. Refer to [Security](DEPLOYMENT.md#security) for security best practices
4. Monitor service health: [MONITORING.md](MONITORING.md)
5. Troubleshoot issues: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 📖 Document Structure

```
herald/
├── README.md              # Main project document (English)
├── README.zhCN.md         # Main project document (Chinese)
├── docs/
│   ├── enUS/
│   │   ├── README.md       # Documentation index (English, this file)
│   │   ├── API.md          # API document (English)
│   │   ├── DEPLOYMENT.md   # Deployment guide (English)
│   │   ├── MONITORING.md   # Monitoring guide (English)
│   │   └── TROUBLESHOOTING.md # Troubleshooting guide (English)
│   └── zhCN/
│       ├── README.md       # Documentation index (Chinese)
│       ├── API.md          # API document (Chinese)
│       ├── DEPLOYMENT.md   # Deployment guide (Chinese)
│       ├── MONITORING.md   # Monitoring guide (Chinese)
│       └── TROUBLESHOOTING.md # Troubleshooting guide (Chinese)
└── ...
```

## 🔍 Find by Topic

### API Related

- API endpoint list: [API.md](API.md)
- Authentication methods: [API.md#authentication](API.md#authentication)
- Error handling: [API.md#error-codes](API.md#error-codes)
- Rate limiting: [API.md#rate-limiting](API.md#rate-limiting)

### Deployment Related

- Docker deployment: [DEPLOYMENT.md#quick-start](DEPLOYMENT.md#quick-start)
- Configuration options: [DEPLOYMENT.md#configuration](DEPLOYMENT.md#configuration)
- Service integration: [DEPLOYMENT.md#integration-with-other-services-optional](DEPLOYMENT.md#integration-with-other-services-optional)
- Security: [DEPLOYMENT.md#security](DEPLOYMENT.md#security)

### Monitoring and Operations

- Prometheus metrics: [MONITORING.md](MONITORING.md)
- Grafana dashboards: [MONITORING.md#grafana-dashboards](MONITORING.md#grafana-dashboards)
- Troubleshooting: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

## 💡 Usage Recommendations

1. **First-time users**: Start with [README.md](../../README.md) and follow the quick start guide
2. **Configure service**: Refer to [DEPLOYMENT.md](DEPLOYMENT.md) to understand all configuration options
3. **Integrate with services**: Check the integration section in [DEPLOYMENT.md](DEPLOYMENT.md)
4. **API integration**: Read [API.md](API.md) to understand the API interfaces
5. **Monitor service**: Set up monitoring with [MONITORING.md](MONITORING.md)
6. **Troubleshoot issues**: Refer to [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common problems

## 📝 Document Updates

Documentation is continuously updated as the project evolves. If you find errors or need additions, please submit an Issue or Pull Request.

## 🤝 Contributing

Documentation improvements are welcome:

1. Find errors or areas that need improvement
2. Submit an Issue describing the problem
3. Or directly submit a Pull Request
