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

### 配置项命名约定

**注意**：虽然规范建议所有配置变量使用 `HERALD_*` 前缀，但当前实现使用了混合命名约定：
- 部分变量使用 `HERALD_*` 前缀（例如：`HERALD_TEST_MODE`、`HERALD_SESSION_STORAGE_ENABLED`）
- 部分变量使用较短名称（例如：`REDIS_ADDR`、`API_KEY`、`HMAC_SECRET`）

为保持一致性并确保未来兼容性，建议尽可能使用 `HERALD_*` 前缀。服务支持两种命名约定。

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
| `SMTP_HOST` | SMTP 服务器主机 | `` | 用于电子邮件 |
| `SMTP_PORT` | SMTP 服务器端口 | `587` | 用于电子邮件 |
| `SMTP_USER` | SMTP 用户名 | `` | 用于电子邮件 |
| `SMTP_PASSWORD` | SMTP 密码 | `` | 用于电子邮件 |
| `SMTP_FROM` | SMTP 发件人地址 | `` | 用于电子邮件 |
| `SMS_PROVIDER` | SMS 提供商 | `` | 用于 SMS |
| `ALIYUN_ACCESS_KEY` | 阿里云访问密钥 | `` | 用于阿里云 SMS |
| `ALIYUN_SECRET_KEY` | 阿里云密钥 | `` | 用于阿里云 SMS |
| `ALIYUN_SIGN_NAME` | 阿里云 SMS 签名名称 | `` | 用于阿里云 SMS |
| `ALIYUN_TEMPLATE_CODE` | 阿里云 SMS 模板代码 | `` | 用于阿里云 SMS |

### Redis 配置

#### 独立 Redis 实例

对于生产部署，**强烈建议**为 Herald 使用专用的 Redis 实例或单独的数据库索引，以避免与其他服务的键空间冲突。

**键前缀：**
- `otp:ch:*` - 挑战数据（TTL：挑战过期时间）
- `otp:rate:*` - 速率限制计数器（TTL：速率限制窗口）
- `otp:idem:*` - 幂等性记录（TTL：幂等键 TTL）
- `otp:lock:*` - 用户锁定记录（TTL：锁定持续时间）

**容量规划：**

基础估算公式：
```
峰值挑战数/秒 × TTL（秒）× 每条平均大小 + 速率限制键 + 审计键
```

**示例：**
- 峰值：100 挑战/秒
- 挑战 TTL：300 秒（5 分钟）
- 平均挑战大小：~500 字节
- 估算：100 × 300 × 500 字节 ≈ 15 MB（挑战数据）
- 加上速率限制键（~1-2 MB）和审计键（~1-2 MB）
- **总估算：约 20 MB 最小值**

对于高可用性部署，请考虑使用 Redis Cluster 或 Redis Sentinel。

## 与其他服务集成（可选）

Herald 设计为可独立工作，也可以根据需要与其他服务集成。如果您想将 Herald 与其他认证或网关服务集成，可以配置以下内容：

**集成配置示例：**
```bash
# Herald 可访问的服务 URL
export HERALD_URL=http://herald:8082

# 用于服务间身份验证的 API 密钥
export HERALD_API_KEY=your-secret-key

# 启用 Herald 集成（如果您的服务支持）
export HERALD_ENABLED=true
```

**注意**：Herald 可以独立使用，无需任何外部服务依赖。与其他服务的集成是可选的，取决于您的具体使用场景。

## 监控

### Prometheus 指标

Herald 在 `/metrics` 端点暴露 Prometheus 指标。以下指标可用：

- `herald_otp_challenges_total{channel,purpose,result}` - 创建的 OTP 挑战总数
- `herald_otp_sends_total{channel,provider,result}` - 通过提供者发送的 OTP 总数
- `herald_otp_verifications_total{result,reason}` - OTP 验证总数
- `herald_otp_send_duration_seconds{provider}` - OTP 发送操作持续时间（直方图）
- `herald_rate_limit_hits_total{scope}` - 速率限制命中总数（scope：user、ip、destination、resend_cooldown）
- `herald_redis_latency_seconds{operation}` - Redis 操作延迟（operation：get、set、del、exists）

**Prometheus 抓取配置示例：**
```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
```

详细的监控文档，请参阅 [MONITORING.md](MONITORING.md)。

## 安全

- 在生产环境中使用 mTLS 或 HMAC 认证（推荐使用 mTLS 以获得最高安全性）
- 设置强 API 密钥
- 在生产环境中使用 TLS/HTTPS
- 适当配置速率限制
- 监控 Redis 的异常活动
