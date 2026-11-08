# Herald - OTP 和验证码服务

[![Go Test Coverage](./.github/coverage.svg)](./.github/go-test-report.md) [![Go Report Card](./.github/goreportcard.svg)](./.github/goreportcard-report.md) [![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE) [![Go Version](https://img.shields.io/badge/go-1.27.3+-blue.svg)](https://golang.org)

Herald v1.5.0 新增 `CreateChallengeWithMetadata`，用于读取可选的创建验证码重试时间。既有 `CreateChallenge` 调用不变，不增加自动重试；参见[升级与回滚说明](docs/zhCN/DEPLOYMENT.md#重试时间元数据升级与回滚v150)。



> **📧 安全验证的网关**

## 🌐 多语言文档

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald 是一个生产就绪的独立 OTP 和验证码服务，可通过电子邮件和 SMS 发送验证码。它具有内置的速率限制、安全控制和审计日志记录功能。Herald 设计为可独立工作，也可以根据需要与其他服务集成。

HTTP 服务使用 Fiber v3；依赖及 Go 工具链以 `go.mod` 为准（Go 1.27.3 或以上）。
构建与验证使用 Linux，macOS、Windows 等宿主环境建议运行 Linux 容器。

## 核心特性

- 🔒 **安全设计**：基于挑战的验证，使用 Argon2 哈希存储，HMAC 或 API Key 请求认证，以及可选的 mTLS 传输保护
- 📊 **内置速率限制**：多维速率限制（按用户、按 IP、按目标），可配置阈值
- 📝 **完整审计跟踪**：所有操作的完整审计日志记录，包含提供者跟踪
- 🔌 **可插拔提供者**：可扩展的电子邮件、SMS 与 DingTalk 提供者架构（邮件通过 [herald-smtp](https://github.com/soulteary/herald-smtp)，DingTalk 通过 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)）
- ↩️ **作废 Challenge**：**POST /v1/otp/challenges/{id}/revoke** 可作废（解除/失效）不再需要的验证码 challenge
- 🔐 **TOTP 代理（可选）**：启用后可将 TOTP（Authenticator）请求代理到 [herald-totp](https://github.com/soulteary/herald-totp)，单一 Herald 地址即可同时支持 OTP 与 TOTP

v1.3.13 恢复创建流量前需遵循 [HMAC 身份升级](docs/zhCN/DEPLOYMENT.md#hmac-身份升级)。
单密钥 HMAC 只接受省略 Key ID 或 `default`，身份不明的旧记录需人工核对；生产 CORS 列表
不能含独立 `*` 项。本次变化见 [更新日志](CHANGELOG.md#1313---2026-11-02)。

## 快速开始

### 使用 Docker Compose

最简单的方式是使用 Docker Compose，它包含 Redis：

```bash
# 必须显式提供随机密钥，Compose 不再使用仓库中的公开占位值
export REDIS_PASSWORD="$(openssl rand -hex 32)"
export HERALD_API_KEY="$(openssl rand -hex 32)"

# 以 API Key 模式启动 Herald 和 Redis
docker compose up -d

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
# 复用启动 Compose 前设置的 API Key
export API_KEY="$HERALD_API_KEY"

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
docker compose logs -f herald
```

### 手动部署

项目在 Linux 上进行构建和验证。macOS、Windows 及其他环境推荐通过 Docker Compose 部署 Linux 容器。

有关手动部署和高级配置，请参阅 [部署指南](docs/zhCN/DEPLOYMENT.md)。

## 基本配置

Herald 需要最少的配置即可开始使用：

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | Server port | `:8082` | No |
| `REDIS_ADDR` | Redis address | `localhost:6379` | No |
| `API_KEY` | `REQUEST_AUTH_MODE=api_key` 使用的 API Key | - | 生产 API Key 模式必需 |

使用 herald-smtp 发邮件时，请设置 `HERALD_SMTP_API_URL`（可选 `HERALD_SMTP_API_KEY`）；参见 [部署指南](docs/zhCN/DEPLOYMENT.md#email-通道herald-smtp)。使用 DingTalk 通道时，请设置 `HERALD_DINGTALK_API_URL`（可选 `HERALD_DINGTALK_API_KEY`）；参见 [部署指南](docs/zhCN/DEPLOYMENT.md#dingtalk-通道herald-dingtalk)。

有关完整的配置选项，包括速率限制、挑战过期时间和提供者设置，请参阅 [部署指南](docs/zhCN/DEPLOYMENT.md#配置)。

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

## 当前安全升级

HMAC 创建验证码要求 v3；业务验证应使用 v2 接口并从可信状态提供预期用户、用途和渠道。
从 v1.3.10 之前的版本升级时，按 [激活索引升级](docs/zhCN/API.md#激活索引升级) 协调切换，
保留 Redis 和审计持久卷。日志字段变化与历史诊断数据处理见 [安全说明](docs/zhCN/SECURITY.md#发送渠道失败诊断)。
当前运维文档维护英文和中文，其余译文作为历史参考。
