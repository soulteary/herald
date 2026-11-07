# Herald 部署指南

## 快速开始

构建和验证统一在 Linux 环境进行。macOS、Windows 及其他环境推荐使用下方 Docker Compose 方案部署 Linux 容器；CI 不验证这些平台上的原生构建。

### 使用 Docker Compose

```bash
cd herald
export REDIS_PASSWORD="$(openssl rand -hex 32)"
export HERALD_API_KEY="$(openssl rand -hex 32)"
docker compose up -d
```

仓库中的 Compose 服务已设置生产启动所必需的 `PROVIDER_FAILURE_POLICY=strict`。

### Linux 手动部署

```bash
# 构建
go build -o herald main.go

# 先配置受密码保护的 Redis 端点
export ENVIRONMENT=production REQUEST_AUTH_MODE=api_key PROVIDER_FAILURE_POLICY=strict
: "${API_KEY:?提供至少 32 字节密钥}"
: "${REDIS_PASSWORD:?提供 Redis 密码}"
: "${AUDIT_SPOOL_DIR:?提供私有持久审计目录}"
./herald
```

## 配置

### 配置项命名约定

**注意**：虽然规范建议所有配置变量使用 `HERALD_*` 前缀，但当前实现使用了混合命名约定：
- 部分变量使用 `HERALD_*` 前缀（例如：`HERALD_TEST_MODE`、`HERALD_HMAC_KEYS`）
- 部分变量使用较短名称（例如：`REDIS_ADDR`、`API_KEY`、`HMAC_SECRET`）

必须使用下表中的准确名称，不能任意添加或删除 `HERALD_` 前缀。
`HERALD_API_KEY` 是 Compose 输入，会映射为进程的 `API_KEY`；仅明确列出的别名有效。

### `REDIS_ADDR` 与 `HERALD_REDIS_*` 约定

当前 Herald 进程实际读取的是：

- `REDIS_ADDR`
- `REDIS_PASSWORD`
- `REDIS_DB`

在 `stargate-suite` 或 compose 模板中，可能会看到 `HERALD_REDIS_ADDR` 等变量名。这类变量通常是上层编排层的统一命名，最终会映射为 Herald 实际识别的 `REDIS_*`。  
部署时请以 Herald 本仓库 `internal/config/config.go` 的 `REDIS_*` 为准。

### 环境变量

以下与代码实现一致（参见 `internal/config/config.go`）。

#### 服务与 Redis

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `ENVIRONMENT` | 运行策略：`development`/`dev`、`test`/`testing`、`production`/`prod`；未知值拒绝启动 | `development` | 生产设置 `production` |
| `PORT` | 服务器监听端口（可带或不带前导冒号，如 `8082` 或 `:8082`） | `:8082` | 否 |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` | 否 |
| `REDIS_PASSWORD` | Redis 密码 | （空） | 生产必需，除非显式风险确认 |
| `REDIS_DB` | Redis 数据库索引 | `0` | 否 |
| `LOG_LEVEL` | 日志级别 | `info` | 否 |
| `SERVICE_NAME` | 服务标识（用于 HMAC/日志/健康检查） | `herald` | 否 |
| `HERALD_READ_TIMEOUT` | 请求读取超时 | `15s` | 否 |
| `HERALD_WRITE_TIMEOUT` | 响应写入超时 | `15s` | 否 |
| `HERALD_IDLE_TIMEOUT` | 空闲连接超时 | `60s` | 否 |
| `HERALD_CORS_ALLOW_ORIGINS` | 逗号分隔源允许列表；空值关闭 CORS，生产禁止通配符 | （空） | 否 |
| `HERALD_RISK_ACK_PASSWORDLESS_REDIS` | 生产无密码 Redis 的显式风险确认；应优先使用密码 | `false` | 仅此例外 |
| `HERALD_NONCE_PREFIX` | Redis nonce 键前缀，实例间与升级时保持一致 | `otp:nonce:` | 否 |
| `PROVIDER_TIMEOUT` | 正值渠道请求超时 | `10s` | 否 |
| `PROVIDER_REDIRECT_POLICY` | HTTP 渠道重定向：`deny` 或 `same-origin`；TOTP 始终禁止 | `deny` | 否 |
| `HERALD_TRUSTED_PROXY_HEADER` | 转发客户端 IP 的请求头；仅信任允许列表中的代理 | （空） | 配置可信代理时 |
| `HERALD_TRUSTED_PROXIES` | 可信代理 IP/CIDR 允许列表，逗号分隔 | （空） | 配置代理请求头时 |

Herald 从右向左解析转发 IP 链，仅跳过明确配置为可信的代理节点，并将遇到的第一个不可信地址作为客户端 IP；不会自动信任调用方可控的最左侧前缀。

#### 服务间认证

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `REQUEST_AUTH_MODE` | 请求认证：`hmac_v2`、`api_key` 或 `none`（仅开发） | `hmac_v2` | 生产必需 |
| `API_KEY` | `api_key` 模式密钥；生产至少 32 字节 | （空） | API Key 模式 |
| `HMAC_SECRET` | 单 HMAC 密钥；生产至少 32 字节 | （空） | HMAC 单密钥模式 |
| `HERALD_HMAC_KEYS` | HMAC 密钥映射；ID/密钥不得为空，生产密钥至少 32 字节 | （空） | HMAC 多密钥模式 |
| `HERALD_HMAC_DEFAULT_KEY_ID` | 配置查找默认项；密钥 map 的 v2/v3 请求仍必须显式发送 `X-Key-Id` | （空） | 可选 |
| `HMAC_MAX_DRIFT` | HMAC v2/v3 时间戳允许偏差 | `60s` | 否 |
| `HMAC_V1_ENABLED` | 临时启用旧版 HMAC v1 | `false` | 否 |
| `HERALD_IDEMPOTENCY_SECRET` | 幂等记录密钥；生产仅配置多 HMAC 密钥时必需 | （空） | 条件必需 |
| `HERALD_PII_PEPPER` | PII 派生 Redis/指标键的 pepper；生产至少 32 字节 | （空） | 建议 |

`hmac_v2` 和 `api_key` 模式缺少可用凭据时会拒绝请求。只有显式选择 `REQUEST_AUTH_MODE=none`
才会跳过请求认证，且生产环境禁止此模式。

#### OTP/Challenge

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_MAX_CONCURRENT_HASH_OPERATIONS` | 每个 handler 实例中创建与 v1/v2 验证共享的并发预算，必须为正整数 | `4` | 否 |
| `CHALLENGE_EXPIRY` | Challenge 过期时间（如 `5m`、`300s`） | `5m` | 否 |
| `MAX_ATTEMPTS` | 单 challenge 最大验证失败次数，超过后锁定 | `5` | 否 |
| `LOCKOUT_DURATION` | 锁定持续时间（如 `10m`） | `10m` | 否 |
| `RESEND_COOLDOWN` | 同一 challenge 重发冷却时间 | `60s` | 否 |
| `CODE_LENGTH` | 验证码位数（最小 `4`；还须在扣除紧凑的上下文绑定 V2 请求封装后不超过 `HERALD_MAX_BODY_BYTES`，当前封装为 136 字节） | `6` | 否 |
| `IDEMPOTENCY_KEY_TTL` | 发送前占位和成功响应的 TTL；不确定发送记录不过期；`0` 表示使用 `CHALLENGE_EXPIRY` | `0` | 否 |
| `ALLOWED_PURPOSES` | 允许的 purpose，逗号分隔，如 `login,reset,bind,stepup` | `login` | 否 |

#### 限流

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `RATE_LIMIT_PER_USER` | 每 user_id 每小时可创建 challenge 数 | `10` | 否 |
| `RATE_LIMIT_PER_IP` | 每 IP 每分钟可创建 challenge 数 | `5` | 否 |
| `RATE_LIMIT_PER_DESTINATION` | 每 destination（邮箱/手机）每小时可创建数 | `10` | 否 |

#### Provider 部署版本

固定使用 `ghcr.io/soulteary/herald-dingtalk:1.2.6`、
`ghcr.io/soulteary/herald-smtp:1.2.6`、
`ghcr.io/soulteary/herald-totp:2.0.2`，或经过核验的不可变摘要。
Herald v1.4.2 导入 `github.com/soulteary/herald-totp/v2` v2.0.2。SMTP 和钉钉
保留补丁版本接口。TOTP 保留 `/v1` HTTP 路由与凭据存储格式，但 HMAC 协议
升级为绑定操作并带 nonce 防重放的 v2；必须协调升级 TOTP 服务和 Herald
代理，旧 HMAC 签名不再接受。仅使用 API key 的模式仍可用。
保留 provider 凭据、TOTP 加密密钥和 Redis 数据。恢复流量前检查健康/就绪状态
及所用认证模式下的真实操作，并遵循[安全要求](SECURITY.md)与 OTP pending
协议迁移说明。邮件和钉钉发送有外部副作用，演练应使用隔离目标。

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

#### TOTP（herald-totp 代理）

启用后，Herald 将 TOTP（Authenticator）请求代理到 [herald-totp](https://github.com/soulteary/herald-totp)。TOTP 路由位于 `/v1/totp/*`（status、verify、enroll/start、enroll/confirm、revoke）。

TOTP 调用使用 10 秒超时，校验 TLS 证书并要求 TLS 1.2 或更高版本，生产环境必须使用
HTTPS。请配置最终服务地址：包括同源跳转在内的所有重定向均被拒绝。Base URL 可以
包含路径前缀，但不能包含用户凭据、查询参数或 fragment。

`PROVIDER_MAX_RESPONSE_BYTES` 同时限制 TOTP 响应体，默认 1 MiB。正常响应必须是完整
的 JSON 对象且 HTTP 状态为 200。重定向、其他 HTTP 状态、超限、格式错误或传输中断
均返回 `502/proxy_failed`，上游错误响应体不能被转为验证成功。正常的 HTTP 200 验证
失败仍返回 `200/ok:false`；enrollment confirm 收到完整的上游 HTTP 400 时，仍返回
`400/invalid`。API Key 和 HMAC 的传输格式保持不变。

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_TOTP_ENABLED` | 是否启用 TOTP 代理 | `false` | 否 |
| `HERALD_TOTP_BASE_URL` | herald-totp 服务 Base URL（如 `http://herald-totp:8085`） | （空） | 启用 TOTP 时 |
| `HERALD_TOTP_API_KEY` | Herald 调用 herald-totp 时使用的 API Key（若 herald-totp 需要） | （空） | 否 |
| `HERALD_TOTP_HMAC_SECRET` | Herald 调用 herald-totp 时使用的 HMAC 密钥（若 herald-totp 使用 HMAC） | （空） | 否 |

#### TLS / mTLS

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `TLS_CERT_FILE` | 服务端证书文件路径 | （空） | 启用 TLS 时 |
| `TLS_KEY_FILE` | 服务端私钥文件路径 | （空） | 启用 TLS 时 |
| `TLS_CA_CERT_FILE` | 客户端 CA 证书（mTLS 校验用） | （空） | optional/require 模式 |
| `TLS_CLIENT_CA_FILE` | 与 `TLS_CA_CERT_FILE` 同义 | （空） | 可选 |
| `CLIENT_CERT_MODE` | 客户端证书策略：`off`、`optional` 或 `require` | `off` | 否 |
| `HERALD_HEALTHCHECK_TLS_CA_FILE` | 二进制回环 HTTPS 探针使用的 CA | （空） | 私有服务端 CA 时 |
| `HERALD_HEALTHCHECK_TLS_SERVER_NAME` | 回环 HTTPS 探针校验的服务端名称 | `localhost` | 否 |
| `HERALD_HEALTHCHECK_TLS_CLIENT_CERT_FILE` | 回环探针使用的客户端证书 | （空） | 要求客户端证书时 |
| `HERALD_HEALTHCHECK_TLS_CLIENT_KEY_FILE` | 回环探针客户端私钥，必须与证书同时配置 | （空） | 要求客户端证书时 |

#### 审计日志

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `AUDIT_ENABLED` | 是否启用审计 | `true` | 否 |
| `AUDIT_MASK_DESTINATION` | 是否对 destination 脱敏 | 生产未显式配置时为 `true`，否则为 `false` | 否 |
| `AUDIT_TTL` | 审计记录在 Redis 中的 TTL（如 `168h` 即 7 天） | `168h` | 否 |
| `AUDIT_STORAGE_TYPE` | 单一持久化后端：`database`（别名 `db`）、`file` 或 `redis`；空值/`none` 使用传入的 Redis 客户端 | （空） | 否 |
| `AUDIT_DATABASE_URL` | 数据库连接串（当 `AUDIT_STORAGE_TYPE` 含 database） | （空） | 否 |
| `AUDIT_TABLE_NAME` | 审计表名 | `audit_logs` | 否 |
| `AUDIT_FILE_PATH` | 审计文件路径（当含 file） | （空） | 否 |
| `AUDIT_WRITER_QUEUE_SIZE` | 唤醒通知队列大小，记录本身保存在磁盘 | `1000` | 否 |
| `AUDIT_WRITER_WORKERS` | 审计写入 worker 数 | `2` | 否 |
| `AUDIT_SPOOL_DIR` | 本地持久化审计缓冲目录 | `./audit-spool`（容器中为 `/var/lib/herald/audit`） | 启用审计时须可写 |
| `AUDIT_SPOOL_MAX_BYTES` | 待投递 JSON 数据的字节上限，文件系统开销需另留空间 | `67108864` | 否 |

审计记录写入私有文件并完成 fsync 后，接口才确认接收；唤醒通知队列满不会丢弃记录。
存储写入失败会保留文件并重试，正常关闭后剩余文件将在下次启动时回放。
查询接口只包含已投递到后端的记录，不包含本地积压。

请将 `AUDIT_SPOOL_DIR` 挂载到持久化本地存储；容器使用 UID/GID 10001，目录建议为 0700。
仓库 Compose 文件通过 `audit_spool` 卷挂载该目录，兼容只读根文件系统。
每个运行实例使用独立目录或卷，独占锁会拒绝并发占用。重启或升级时保留卷；
`docker compose down -v` 会删除缓冲，有待投递记录时不能使用。目录必须支持文件及目录的
fsync 和原子 rename。

目录不可用时启动失败；缓冲写满或运行中本地写入失败时，相关接口返回
`503/audit_unavailable`，不会返回业务成功。创建请求的首次审计失败会阻止发送；
验证和撤销操作会在改变 challenge 状态前记录尝试。若后续结果记录写入失败，已发生的
业务操作不会回滚；对于不确定的发送结果，应使用相同幂等键查询或重试，不能直接补发。
待投递文件不自动过期，受字节上限约束，需监控磁盘空间和投递失败。

投递语义为**至少一次**：后端已提交但本地文件尚未删除时退出，会以相同 `event_id` 回放，
下游可据此去重。destination 在本地落盘前脱敏。请保护和备份持久卷；文件系统或卷丢失
不在保障范围内。后端审计 TTL 不会淘汰本地待投递文件。
显式设置 `AUDIT_ENABLED=false` 会关闭审计及这些检查。


#### 模板与可观测性

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `TEMPLATE_DIR` | 邮件/短信模板目录（可选） | （空） | 否 |
| `OTLP_ENABLED` | 是否启用 OpenTelemetry | `false` | 否 |
| `OTLP_ENDPOINT` | OTLP 端点（如 `http://localhost:4318`） | （空） | 启用 OTLP 时 |

#### 测试与调试

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_TEST_MODE` | 仅在 `ENVIRONMENT=test` 时启用测试码暴露；生产禁止 | `false` | 否 |
| `HERALD_TEST_API_KEY` | 测试码监听器专用密钥 | （空） | 测试模式 |
| `HERALD_TEST_LISTENER_ADDR` | 独立、仅回环地址的测试监听器 | `127.0.0.1:18082` | 否 |

### 测试模式与调试

仅当同时设置 `ENVIRONMENT=test` 和 `HERALD_TEST_MODE=true` 时：

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

### TOTP（herald-totp）

当 `HERALD_TOTP_ENABLED=true` 且设置了 `HERALD_TOTP_BASE_URL` 时，Herald 会将 TOTP（Authenticator）请求代理到 [herald-totp](https://github.com/soulteary/herald-totp)。Stargate 等调用方可使用同一 Herald 地址同时完成 OTP（短信/邮件/钉钉）与 TOTP 流程。

- 将 `HERALD_TOTP_BASE_URL` 设置为 herald-totp 服务的基础 URL（例如 `http://herald-totp:8085`）。
- 若 herald-totp 需要 API Key 或 HMAC 认证，请相应设置 `HERALD_TOTP_API_KEY` 或 `HERALD_TOTP_HMAC_SECRET`。
- TOTP 端点（status、verify、enroll/start、enroll/confirm、revoke）见 [API.md](API.md#totp-代理可选)。

### 插件启用条件速查

| 能力 | 启用条件 | 备注 |
|------|----------|------|
| 内置 SMTP | 未设置 `HERALD_SMTP_API_URL` | 使用 `SMTP_*` 直连 SMTP 服务 |
| herald-smtp | `HERALD_SMTP_API_URL` 非空 | 设置后会覆盖内置 SMTP 通道 |
| SMS HTTP API | `SMS_PROVIDER` 非空，且建议同时设置 `SMS_API_BASE_URL` | `SMS_API_KEY` 视网关要求 |
| herald-dingtalk | `HERALD_DINGTALK_API_URL` 非空 | 可选 `HERALD_DINGTALK_API_KEY` |
| herald-totp 代理 | `HERALD_TOTP_ENABLED=true` 且 `HERALD_TOTP_BASE_URL` 非空 | 可选 `HERALD_TOTP_API_KEY` / `HERALD_TOTP_HMAC_SECRET` |
| OTLP | `OTLP_ENABLED=true` 且建议配置 `OTLP_ENDPOINT` | 推荐在生产明确设置 endpoint |

### Redis 配置

#### 独立 Redis 实例

对于生产部署，**强烈建议**为 Herald 使用专用的 Redis 实例或单独的数据库索引，以避免与其他服务的键空间冲突。

**键前缀：**
- `otp:ch:*` - 挑战数据（TTL：挑战过期时间）
- `ratelimit:*` - 速率限制计数器（TTL：速率限制窗口）
- `otp:idem2:*` - 幂等记录（发送前占位和成功响应按配置 TTL 过期；不确定发送不过期）
- `otp:lock:*` - 用户锁定记录（TTL：锁定持续时间）

**不确定的发送记录：**

幂等存储应使用持久化 Redis，并采用 `noeviction` 策略。要阻止重发，已确认的写入就必须
在重试期间保留；Redis 丢失或淘汰该记录后，Herald 无法保证不重复发送。请按所需的可靠性
配置 Redis 持久化和故障转移。在存在未核对记录或仍可能重试时，不要清空数据库或轮换
`HERALD_IDEMPOTENCY_SECRET`，否则派生键变化会绕过旧记录。

`sending` 状态记录不设 TTL，包含用于核对的 `challenge_id`。监控 `delivery_unknown`
结果和未完成记录数量，并用该 challenge ID（也是传给渠道的幂等键）查询渠道日志。
管理员处理记录前需确认原请求已停止、投递结果已核实，不能自动过期或删除记录，也不能
直接换键重发；确需补发时应作为业务上的独立决定处理。持久化后、发送前退出也会留下
记录，因此需要为人工核对和存储容量留出空间。这保证不确定键最多尝试发送一次，不保证投递成功。

**容量规划：**

使用代表性流量测量载荷和 Redis 开销，计入 challenge、激活索引、会过期的已完成幂等响应、
不自动过期的不确定发送、限流/冷却、nonce 和审计保留。峰值速率乘以 challenge TTL 不能
界定总容量。持久化审计缓冲磁盘及进程哈希预算需另行规划，并结合 `MEMORY USAGE`、键数量
和保留记录增长量调整配置。

`REDIS_ADDR` 指定单一 Redis 端点；Herald 未提供原生 Cluster 或 Sentinel 发现配置。
高可用部署使用外部管理的故障切换端点并保留数据，不应假定多键激活脚本兼容 Redis Cluster。

## 与其他服务集成（可选）

Herald 设计为可独立工作，也可以根据需要与其他服务集成。如果您想将 Herald 与其他认证或网关服务集成，可以配置以下内容：

以下为调用方配置示例，不是新增的 Herald 进程变量。Herald 的 API Key 模式使用
`REQUEST_AUTH_MODE=api_key` 和 `API_KEY`。

**集成配置示例：**
```bash
# Herald 可访问的服务 URL
export HERALD_URL=http://herald:8082

# 用于服务间身份验证的 API 密钥
: "${API_KEY:?load the Herald server API key}"
export HERALD_API_KEY="$API_KEY"

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
- `herald_redis_operation_duration_seconds{operation}` - Redis 包装操作延迟（限流与冷却；见 MONITORING.md）

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

## 发送渠道失败诊断

发送失败使用受控 `reason` 字段，不再记录上游错误消息；span 状态和 exception 事件也只保留安全原因码。
排障时不要启用原始渠道响应体日志。若历史日志或导出的 span 含敏感数据，应核查访问与保留策略，
并轮换已暴露凭据，字段变化见 [安全说明](SECURITY.md#发送渠道失败诊断)。

## OTP 哈希工作准入

此前验证接口独立允许最多 16 个在途操作，创建没有全局工作限制；现在的共享默认值改变突发流量
的准入行为，不改变验证码有效性。

`HERALD_MAX_CONCURRENT_HASH_OPERATIONS` 默认 4，必须为正整数。创建验证码及两版 OTP
验证接口共享同一预算，正常服务实例只有一个 handler；预算在本实例内生效，不跨副本共享。
它不改变 Argon2 参数，也不影响 TOTP 代理。

当前单次 Argon2 约需 64 MiB。应结合内存、CPU、垃圾回收和进程其他开销配置预算，不能将它
视作进程 RSS 的硬上限。超额请求不排队，返回 `503/backend_unavailable` 和 `Retry-After: 1`。
创建准入先于限流和冷却写入；Redis 可用时会释放该请求的发送前幂等占位。即使预算已满，
已完成幂等响应仍可重放。被拒绝的验证不会消费验证码或增加尝试次数。

验码锁发生竞争时只立即尝试一次，返回现有的可重试 `409/lock_contention`，不占用预算轮询等待。
创建槽位在调用发送渠道前释放，慢渠道不会占住哈希预算。重试保留原幂等键，并使用新的 HMAC
时间戳与 nonce。现有 Redis 记录及签名格式不变。

## HMAC 身份升级

HMAC 部署先暂停创建流量、排空在途创建请求，再替换全部旧实例后恢复。混合版本仍可能拆分
隐式和显式 Key ID 命名空间，因为旧代码不会检查新规范记录。保留 Redis 数据、密钥映射和
`HERALD_IDEMPOTENCY_SECRET`；改变派生密钥会绕过全部已有幂等记录。

旧空 Key ID 记录保持原样，相同服务/客户端键返回 `409/idempotency_namespace_conflict`，
需由运维核对，不能改键自动重试。不自动过期的发送记录继续保留，不能根据当前默认密钥猜测
原凭据，也不能在密钥轮换后盲目重放或迁移旧响应。没有旧记录冲突时，仍获授权的显式 Key ID 记录继续正常使用。

旧版本无法执行此保护。回滚前保持创建停用，直到受影响记录和调用身份已核对完成，且不能在
旧代码上恢复隐式 Key ID 调用方，细节见 [API](API.md#hmac-身份与旧幂等记录)。

### 单密钥 Key ID 白名单

`HMAC_SECRET` 只授权隐式身份 `default`。此前任意显式 ID 都会选中同一密钥，却建立
独立 nonce/幂等命名空间；现在未配置 ID 返回 `401/invalid_key_id`。

恢复创建前，清点历史显式 ID，并结合渠道及审计证据核对不确定发送。Redis 幂等键是不透明
摘要，不能自动重新分配身份。保留全部记录、幂等派生密钥及 nonce 前缀；不能删除发送记录
或更换调用方 ID/客户端幂等键强行重试。

需保留已知自定义 ID 时，将其显式加入 `HERALD_HMAC_KEYS`，保持调用方 ID 不变。多个 ID
若有意共用密钥，仍是不同的已配置身份，重试不能轮换使用。已有调用方使用 `default` 时，
需在 map 中显式保留，并明确配置 map 默认项。生产 key map 部署要求设置
`HERALD_IDEMPOTENCY_SECRET`；必须保持升级前的有效值，不能生成新值来代替。

暂停创建并替换全部实例。旧版本会再次接受任意 ID；回滚时保持创建停用，直到受影响身份和
记录核对完成，且由外部白名单执行同等限制。此前空 Key ID 保护继续有效，本次修复不会迁移
或删除任何记录。

## 生产 CORS 来源列表

`HERALD_CORS_ALLOW_ORIGINS` 为空时关闭 CORS。生产环境拒绝逗号列表中任意位置的独立
`*` 项，包括 `https://trusted.example,*` 及带空白的项。启动校验和路由构建都会在安装
中间件前拒绝。此前仅在整个配置值为 `*` 时拒绝，混合列表会向未列出的来源返回通配响应。

升级前移除独立通配项，只保留预期来源，或清空配置关闭 CORS。回滚前同样应修正配置，旧版本
仍接受混合通配列表。开发/测试环境的通配支持、正常显式列表及请求认证保持不变，无需迁移
Redis 记录或客户端签名。CORS 是浏览器来源边界，API Key/HMAC 认证仍独立执行。


## 从 v1.4.1 升级到 v1.4.2

先部署 herald-totp v2.0.2，再部署 Herald v1.4.2。凭据快照提交校验在 TOTP
服务内执行，仅更新 SDK 依赖不会升级运行中的后端。公开 SDK 签名、HMAC
协议、HTTP 路径及请求和成功响应格式不变。

保留现有 Redis 数据、命名空间、待确认注册、计数、幂等密钥、nonce/重放记录、
审计状态和加密密钥，两个补丁版本之间无需额外数据转换。全部替换 TOTP
实例后才具备快照校验；混合旧实例仍保留原提交行为。该校验不会撤销已经
完成的操作。

Herald 回滚至 v1.4.1 时接口保持兼容，不会随之回滚独立的 TOTP 服务。后端
回滚至 v2.0.1 会失去快照提交校验。两个方向都应保留状态，不要清空记录
绕过验证拒绝。
