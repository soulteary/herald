# Herald 部署指南

## 快速开始

### 使用 Docker Compose

```bash
cd herald
docker-compose up -d
```

### 手动部署

```bash
# 构建
go build -o herald main.go

# 运行
./herald
```

## 配置

### 环境变量

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `PORT` | 服务器端口（可以带或不带前导冒号，例如 `8082` 或 `:8082`） | `:8082` | 否 |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` | 否 |
| `REDIS_PASSWORD` | Redis 密码 | `` | 否 |
| `REDIS_DB` | Redis 数据库 | `0` | 否 |
| `API_KEY` | 用于身份验证的 API 密钥 | `` | 推荐 |
| `HMAC_SECRET` | 用于安全认证的 HMAC 密钥 | `` | 可选 |
| `LOG_LEVEL` | 日志级别 | `info` | 否 |
| `CHALLENGE_EXPIRY` | 挑战过期时间 | `5m` | 否 |
| `MAX_ATTEMPTS` | 最大验证尝试次数 | `5` | 否 |
| `RESEND_COOLDOWN` | 重发冷却时间 | `60s` | 否 |
| `CODE_LENGTH` | 验证码长度 | `6` | 否 |
| `RATE_LIMIT_PER_USER` | 每个用户/小时的速率限制 | `10` | 否 |
| `RATE_LIMIT_PER_IP` | 每个 IP/分钟的速率限制 | `5` | 否 |
| `RATE_LIMIT_PER_DESTINATION` | 每个目标/小时的速率限制 | `10` | 否 |
| `LOCKOUT_DURATION` | 达到最大尝试次数后的用户锁定持续时间 | `10m` | 否 |
| `SERVICE_NAME` | HMAC 认证的服务标识符 | `herald` | 否 |
| `EMAIL_API_URL` | 邮件服务 API 地址 | `` | 用于邮件 |
| `EMAIL_API_KEY` | 邮件服务 API Key | `` | 可选 |
| `EMAIL_FROM` | 邮件发件人地址 | `` | 用于邮件 |
| `SMS_API_URL` | 短信服务 API 地址 | `` | 用于短信 |
| `SMS_API_KEY` | 短信服务 API Key | `` | 可选 |
| `PROVIDER_TIMEOUT` | 外部服务超时时间 | `5s` | 否 |

## 与 Stargate 集成

1. 在 Stargate 配置中设置 `HERALD_URL`
2. 在 Stargate 配置中设置 `HERALD_API_KEY`
3. 在 Stargate 配置中设置 `HERALD_ENABLED=true`

示例：
```bash
export HERALD_URL=http://herald:8082
export HERALD_API_KEY=your-secret-key
export HERALD_ENABLED=true
```

## 安全

- 在生产环境中使用 HMAC 认证
- 设置强 API 密钥
- 在生产环境中使用 TLS/HTTPS
- 适当配置速率限制
- 监控 Redis 的异常活动
