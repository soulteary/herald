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

以下与代码实现一致（参见 `internal/config/config.go`）。

#### 服务与 Redis

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `PORT` | 服务器监听端口（可带或不带前导冒号，如 `8082` 或 `:8082`） | `:8082` | 否 |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` | 否 |
| `REDIS_PASSWORD` | Redis 密码 | （空） | 否 |
| `REDIS_DB` | Redis 数据库索引 | `0` | 否 |
| `LOG_LEVEL` | 日志级别 | `info` | 否 |
| `SERVICE_NAME` | 服务标识（用于 HMAC/日志/健康检查） | `herald` | 否 |

#### 服务间认证

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `API_KEY` | 简单 API Key 认证（请求头携带） | （空） | 推荐其一 |
| `HMAC_SECRET` | 单密钥 HMAC 签名认证 | （空） | 推荐其一 |
| `HERALD_HMAC_KEYS` | 多密钥 HMAC，JSON 格式：`{"key-id-1":"secret-1","key-id-2":"secret-2"}`，支持密钥轮换 | （空） | 推荐其一 |

未设置任一认证时，服务会记录警告并允许未认证请求（仅适合开发/测试）。

#### OTP/Challenge

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `CHALLENGE_EXPIRY` | Challenge 过期时间（如 `5m`、`300s`） | `5m` | 否 |
| `MAX_ATTEMPTS` | 单 challenge 最大验证失败次数，超过后锁定 | `5` | 否 |
| `LOCKOUT_DURATION` | 锁定持续时间（如 `10m`） | `10m` | 否 |
| `RESEND_COOLDOWN` | 同一 challenge 重发冷却时间 | `60s` | 否 |
| `CODE_LENGTH` | 验证码位数 | `6` | 否 |
| `IDEMPOTENCY_KEY_TTL` | 幂等键缓存 TTL；`0` 表示使用 `CHALLENGE_EXPIRY` | `0` | 否 |
| `ALLOWED_PURPOSES` | 允许的 purpose，逗号分隔，如 `login,reset,bind,stepup` | `login` | 否 |

#### 限流

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `RATE_LIMIT_PER_USER` | 每 user_id 每小时可创建 challenge 数 | `10` | 否 |
| `RATE_LIMIT_PER_IP` | 每 IP 每分钟可创建 challenge 数 | `5` | 否 |
| `RATE_LIMIT_PER_DESTINATION` | 每 destination（邮箱/手机）每小时可创建数 | `10` | 否 |

#### 邮件通道

**内置 SMTP**（当未设置 `HERALD_SMTP_API_URL` 时使用）：

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `SMTP_HOST` | SMTP 服务器主机 | （空） | 使用内置时 |
| `SMTP_PORT` | SMTP 端口 | `587` | 否 |
| `SMTP_USER` | SMTP 用户名 | （空） | 否 |
| `SMTP_PASSWORD` | SMTP 密码 | （空） | 否 |
| `SMTP_FROM` | 发件人地址 | （空） | 建议 |
| `PROVIDER_FAILURE_POLICY` | 发送失败策略：`soft`（仍创建 challenge）或 `strict`（失败则不创建） | `soft` | 否 |

**herald-smtp 插件**（设置后不再使用内置 SMTP）：

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_SMTP_API_URL` | [herald-smtp](https://github.com/soulteary/herald-smtp) 服务 Base URL（如 `http://herald-smtp:8084`） | （空） | 使用插件时 |
| `HERALD_SMTP_API_KEY` | 若 herald-smtp 启用 `API_KEY`，需与此一致 | （空） | 否 |

#### 短信通道（HTTP API 模式）

当前实现通过 HTTP 调用外部 SMS API，不直接使用阿里云等密钥；密钥由外部 SMS 服务或网关保管。

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `SMS_PROVIDER` | 提供商名称（如 `aliyun`、`tencent`、`http`），用于标识与日志 | （空） | 使用 SMS 时 |
| `SMS_API_BASE_URL` | SMS HTTP API 的 Base URL | （空） | 使用 SMS 时 |
| `SMS_API_KEY` | SMS API 认证密钥（若需要） | （空） | 视网关要求 |

#### DingTalk 通道（herald-dingtalk 插件）

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) 服务 Base URL（如 `http://herald-dingtalk:8083`） | （空） | 使用 DingTalk 时 |
| `HERALD_DINGTALK_API_KEY` | 若 herald-dingtalk 启用 `API_KEY`，需与此一致 | （空） | 否 |

#### TLS / mTLS

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `TLS_CERT_FILE` | 服务端证书文件路径 | （空） | 启用 TLS 时 |
| `TLS_KEY_FILE` | 服务端私钥文件路径 | （空） | 启用 TLS 时 |
| `TLS_CA_CERT_FILE` | 客户端 CA 证书（mTLS 校验用） | （空） | 可选 |
| `TLS_CLIENT_CA_FILE` | 与 `TLS_CA_CERT_FILE` 同义 | （空） | 可选 |

#### 会话存储（可选）

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_SESSION_STORAGE_ENABLED` | 是否启用 Redis 会话存储 | `false` | 否 |
| `HERALD_SESSION_DEFAULT_TTL` | 会话默认 TTL（如 `1h`） | `1h` | 否 |
| `HERALD_SESSION_KEY_PREFIX` | Redis 会话键前缀 | `session:` | 否 |

#### 审计日志

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `AUDIT_ENABLED` | 是否启用审计 | `true` | 否 |
| `AUDIT_MASK_DESTINATION` | 是否对 destination 脱敏 | `false` | 否 |
| `AUDIT_TTL` | 审计记录在 Redis 中的 TTL（如 `168h` 即 7 天） | `168h` | 否 |
| `AUDIT_STORAGE_TYPE` | 持久化类型：`database`、`file`、`loki` 或逗号分隔多类型 | （空） | 否 |
| `AUDIT_DATABASE_URL` | 数据库连接串（当 `AUDIT_STORAGE_TYPE` 含 database） | （空） | 否 |
| `AUDIT_TABLE_NAME` | 审计表名 | `audit_logs` | 否 |
| `AUDIT_FILE_PATH` | 审计文件路径（当含 file） | （空） | 否 |
| `AUDIT_LOKI_URL` | Loki 地址（当含 loki） | （空） | 否 |
| `AUDIT_WRITER_QUEUE_SIZE` | 审计写入队列大小 | `1000` | 否 |
| `AUDIT_WRITER_WORKERS` | 审计写入 worker 数 | `2` | 否 |

#### 模板与可观测性

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `TEMPLATE_DIR` | 邮件/短信模板目录（可选） | （空） | 否 |
| `OTLP_ENABLED` | 是否启用 OpenTelemetry | `false` | 否 |
| `OTLP_ENDPOINT` | OTLP 端点（如 `http://localhost:4318`） | （空） | 启用 OTLP 时 |

#### 测试与调试

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_TEST_MODE` | 为 `true` 时写入 Redis 可查码、创建 challenge 响应可含 `debug_code`；**生产必须为 false** | `false` | 否 |

### 测试模式与调试

当 `HERALD_TEST_MODE=true` 时：

- 创建 challenge 的响应中会包含 `debug_code` 字段（明文验证码），便于调用方（如 Stargate 在 `DEBUG=true` 时）在登录页直接展示。
- 会开放接口 `GET /v1/test/code/:challenge_id`，用于根据 challenge_id 查询验证码（供集成测试或 Stargate 在创建响应未带 `debug_code` 时回退使用）。

**安全：** 生产环境必须设置 `HERALD_TEST_MODE=false`。测试模式会暴露验证码，不得在生产环境使用。

### Email 通道（herald-smtp）

当设置 `HERALD_SMTP_API_URL` 时，Herald 不再使用内置 SMTP，而是通过 HTTP 将邮件发送请求转发给 [herald-smtp](https://github.com/soulteary/herald-smtp)。所有 SMTP 凭证与发送逻辑均在 herald-smtp 中；此模式下 Herald 不保存任何 email 通道的 SMTP 凭证。

- 将 `HERALD_SMTP_API_URL` 设置为 herald-smtp 服务的基础 URL（例如 `http://herald-smtp:8084`）。
- 若 herald-smtp 配置了 `API_KEY`，请将 `HERALD_SMTP_API_KEY` 设为相同值，以便 Herald 调用 herald-smtp 时通过认证。
- 设置 `HERALD_SMTP_API_URL` 后，Herald 会忽略 `SMTP_HOST` 等内置 SMTP 配置（email 通道不再使用它们）。

### DingTalk 通道（herald-dingtalk）

当 `channel` 为 `dingtalk` 时，Herald 不直接发送消息，而是通过 HTTP 将发送请求转发给 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)。所有钉钉凭证与业务逻辑均在 herald-dingtalk 中；Herald 不保存任何钉钉凭证。

- 将 `HERALD_DINGTALK_API_URL` 设置为 herald-dingtalk 服务的基础 URL（例如 `http://herald-dingtalk:8083`）。
- 若 herald-dingtalk 配置了 `API_KEY`，请将 `HERALD_DINGTALK_API_KEY` 设为相同值，以便 Herald 调用 herald-dingtalk 时通过认证。

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
