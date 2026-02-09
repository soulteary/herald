# Herald - OTP 和验证码服务

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org)
[![codecov](https://codecov.io/gh/soulteary/herald/branch/main/graph/badge.svg)](https://codecov.io/gh/soulteary/herald)
[![Go Report Card](https://goreportcard.com/badge/github.com/soulteary/herald)](https://goreportcard.com/report/github.com/soulteary/herald)

> **📧 安全验证的网关**

## 🌐 多语言文档

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald 是一个生产就绪的独立 OTP 和验证码服务，可通过电子邮件和 SMS 发送验证码。它具有内置的速率限制、安全控制和审计日志记录功能。Herald 设计为可独立工作，也可以根据需要与其他服务集成。

## 核心特性

- 🔒 **安全设计**：基于挑战的验证，使用 Argon2 哈希存储，多种认证方法（mTLS、HMAC、API Key）
- 📊 **内置速率限制**：多维速率限制（按用户、按 IP、按目标），可配置阈值
- 📝 **完整审计跟踪**：所有操作的完整审计日志记录，包含提供者跟踪
- 🔌 **可插拔提供者**：可扩展的电子邮件、SMS 与 DingTalk 提供者架构（邮件通过 [herald-smtp](https://github.com/soulteary/herald-smtp)，DingTalk 通过 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)）

## 快速开始

### 使用 Docker Compose

最简单的方式是使用 Docker Compose，它包含 Redis：

```bash
# Start Herald and Redis
docker-compose up -d

# Verify the service is running
curl http://localhost:8082/healthz
```

预期响应：
```json
{
  "status": "ok",
  "service": "herald"
}
```

### 测试 API

创建测试挑战（需要身份验证 - 请参阅 [API 文档](docs/zhCN/API.md)）：

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

### 查看日志

```bash
# Docker Compose logs
docker-compose logs -f herald
```

### 手动部署

有关手动部署和高级配置，请参阅 [部署指南](docs/zhCN/DEPLOYMENT.md)。

## 基本配置

Herald 需要最少的配置即可开始使用：

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | No |
| `API_KEY` | API key for authentication | - | Recommended |

使用 herald-smtp 发邮件时，请设置 `HERALD_SMTP_API_URL`（可选 `HERALD_SMTP_API_KEY`）；参见 [部署指南](docs/zhCN/DEPLOYMENT.md#email-通道herald-smtp)。使用 DingTalk 通道时，请设置 `HERALD_DINGTALK_API_URL`（可选 `HERALD_DINGTALK_API_KEY`）；参见 [部署指南](docs/zhCN/DEPLOYMENT.md#dingtalk-通道herald-dingtalk)。

有关完整的配置选项，包括速率限制、挑战过期时间和提供者设置，请参阅 [部署指南](docs/zhCN/DEPLOYMENT.md#configuration)。

## 文档

### 开发者文档

- **[API 文档](docs/zhCN/API.md)** - 完整的 API 参考，包含认证方法、端点和错误代码
- **[部署指南](docs/zhCN/DEPLOYMENT.md)** - 配置选项、Docker 部署和集成示例

### 运维文档

- **[监控指南](docs/zhCN/MONITORING.md)** - Prometheus 指标、Grafana 仪表板和告警
- **[故障排查指南](docs/zhCN/TROUBLESHOOTING.md)** - 常见问题、诊断步骤和解决方案

### 文档索引

有关所有文档的完整概述，请参阅 [docs/zhCN/README.md](docs/zhCN/README.md)。

## License

See [LICENSE](LICENSE) for details.
