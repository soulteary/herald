# Herald - OTP 和验证码服务

> **📧 安全验证的网关**

## 🌐 多语言文档 / Multi-language Documentation

- [English](README.md) | [中文](README.zhCN.md) | [Français](README.frFR.md) | [Italiano](README.itIT.md) | [日本語](README.jaJP.md) | [Deutsch](README.deDE.md) | [한국어](README.koKR.md)

![Herald](.github/assets/banner.jpg)

Herald 是一个生产就绪的轻量级服务，用于通过电子邮件发送验证码（OTP）（SMS 支持目前正在开发中），具有内置的速率限制、安全控制和审计日志记录。

## 特性

- 🚀 **高性能**：使用 Go 和 Fiber 构建
- 🔒 **安全**：基于挑战的验证，使用哈希存储
- 📊 **速率限制**：多维速率限制（按用户、按 IP、按目标）
- ♻️ **幂等支持**：避免重复创建挑战与重复发送
- 📈 **指标**：Prometheus 兼容指标端点
- 📝 **运行日志**：关键事件与错误排查信息
- 🔌 **可插拔提供者**：支持电子邮件提供者（SMS 提供者是占位符实现，尚未完全 functional）
- ⚡ **Redis 后端**：快速、分布式存储，使用 Redis

## 快速开始

```bash
# 使用 Docker Compose 运行
docker-compose up -d

# 或直接运行
go run main.go
```

## 配置

设置环境变量：

- `PORT`：服务器端口（默认：`:8082`）
- `REDIS_ADDR`：Redis 地址（默认：`localhost:6379`）
- `REDIS_PASSWORD`：Redis 密码（可选）
- `REDIS_DB`：Redis 数据库编号（默认：`0`）
- `API_KEY`：用于服务间身份验证的 API 密钥
- `LOG_LEVEL`：日志级别（默认：`info`）

有关完整的配置选项，请参阅 [DEPLOYMENT.md](docs/zhCN/DEPLOYMENT.md)。

## API 文档

有关详细的 API 文档，请参阅 [API.md](docs/zhCN/API.md)。
