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

仓库中的 Compose 服务已设置 `PROVIDER_FAILURE_POLICY=strict`。生产环境要求 `strict`；未设置该变量时，生产环境也使用 `strict`。

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
`HERALD_API_KEY` 是 Compose 输入，会映射为进程的 `API_KEY`；Herald 不读取
`HERALD_REDIS_ADDR`。仅明确列出的别名有效。

### `REDIS_ADDR` 与 `HERALD_REDIS_*` 约定

当前 Herald 进程实际读取的是：

- `REDIS_ADDR`
- `REDIS_PASSWORD`
- `REDIS_DB`
- `REDIS_USERNAME`
- `REDIS_TLS_ENABLED`
- `REDIS_TLS_CA_FILE`
- `REDIS_TLS_SERVER_NAME`

Herald 不读取任何 `HERALD_REDIS_*` 变量，本仓库的 `docker-compose.yml` 也没有定义这类别名：它只向进程传入 `REDIS_ADDR`、`REDIS_PASSWORD` 和 `REDIS_DB`，其中只有密码取自宿主环境的同名变量 `REDIS_PASSWORD`。使用 ACL 用户名或 TLS 时，需自行在 `herald` 服务的 `environment` 中加入 `REDIS_USERNAME` 和 `REDIS_TLS_*`。

在 `stargate-suite` 或其他 compose 模板中，可能会看到 `HERALD_REDIS_ADDR` 等变量名。这类变量是上层编排层的统一命名，只有被该编排层映射为 Herald 实际识别的 `REDIS_*` 后才会生效。
部署时请以 Herald 本仓库 `internal/config/config.go` 的 `REDIS_*` 为准。

### 环境变量

以下与代码实现一致（参见 `internal/config/config.go`）。

取值格式在启动时检查。布尔值必须是 `1`、`t`、`T`、`TRUE`、`true`、`True`、`0`、`f`、`F`、
`FALSE`、`false`、`False` 之一；不接受 `yes`、`on` 以及首尾带空白的值。整数必须是普通的
十进制数字。时长必须带单位，例如 `60s` 或 `5m`；不接受 `60`。生产环境中，格式错误的值会
阻止启动；其他环境中 Herald 记录警告并使用默认值。

#### 服务与 Redis

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `ENVIRONMENT` | 运行策略：`development`/`dev`、`test`/`testing`、`production`/`prod`；未知值拒绝启动。未设置时 Herald 会在启动时记录警告，因为此时不会强制执行生产校验；后续版本将在未设置时拒绝启动 | `development` | 请显式设置；生产设置 `production` |
| `PORT` | 服务器监听端口（可带或不带前导冒号，如 `8082` 或 `:8082`） | `:8082` | 否 |
| `REDIS_ADDR` | Redis 地址（`host:port`）；无效值在生产环境会导致启动失败，在其他环境回退到默认值并记录警告 | `localhost:6379` | 否 |
| `REDIS_PASSWORD` | Redis 密码 | （空） | 生产必需，除非显式风险确认 |
| `REDIS_DB` | Redis 数据库索引 | `0` | 否 |
| `REDIS_USERNAME` | Redis ACL 用户名；留空使用默认用户。须同时设置 `REDIS_PASSWORD` | （空） | 否 |
| `REDIS_TLS_ENABLED` | 通过 TLS（1.2 及以上）连接 Redis；始终校验服务端证书 | `false` | 否 |
| `REDIS_TLS_CA_FILE` | 用于校验 Redis 证书的 PEM CA 文件；留空使用系统根证书。须设置 `REDIS_TLS_ENABLED=true` | （空） | 否 |
| `REDIS_TLS_SERVER_NAME` | Redis 证书中应包含的名称；留空使用 `REDIS_ADDR` 的主机名。须设置 `REDIS_TLS_ENABLED=true` | （空） | 否 |
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

这两项设置还决定 `RATE_LIMIT_PER_IP` 的计数对象。未配置时，限额按直连对端计算，因此代表众多终端用户调用 Herald 的网关会让这些用户共用一份配额。配置后，可信网关可以转发终端用户地址（Go SDK：`WithClientIPHeader`），限额按终端用户计算。被该配额拒绝的请求返回 `429/rate_limit_exceeded`，并带有 `"limit":"ip"`。

Herald 从右向左解析转发 IP 链，仅跳过明确配置为可信的代理节点，并将遇到的第一个不可信地址作为客户端 IP；不会自动信任调用方可控的最左侧前缀。

#### 服务间认证

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `REQUEST_AUTH_MODE` | 请求认证：`hmac_v2`、`api_key` 或 `none`（仅开发） | `hmac_v2` | 生产必需 |
| `API_KEY` | `api_key` 模式密钥；生产至少 32 字节 | （空） | API Key 模式 |
| `HMAC_SECRET` | 单 HMAC 密钥；生产至少 32 字节 | （空） | HMAC 单密钥模式 |
| `HERALD_HMAC_KEYS` | HMAC 密钥映射；ID/密钥不得为空，生产密钥至少 32 字节。已设置但格式错误的映射在生产环境会导致启动失败；其他环境会忽略该映射并记录警告，改用 `HMAC_SECRET` | （空） | HMAC 多密钥模式 |
| `HERALD_HMAC_DEFAULT_KEY_ID` | 配置查找默认项；密钥 map 的 v2/v3 请求仍必须显式发送 `X-Key-Id` | （空） | 可选 |
| `HMAC_MAX_DRIFT` | HMAC v2/v3 时间戳允许偏差 | `60s` | 否 |
| `HMAC_V1_ENABLED` | 临时启用旧版 HMAC v1 | `false` | 否 |
| `HERALD_IDEMPOTENCY_SECRET` | 不透明幂等记录的密钥；生产仅配置多 HMAC 密钥时必需。非生产环境未设置时，Herald 依次使用 `HMAC_SECRET`、`API_KEY`、由 `HERALD_HMAC_KEYS` 派生的值；三者都没有时，每次启动生成一个随机值并记录警告，此时幂等、nonce、限流和冷却记录不在实例间共享，重启后也不会保留 | （空） | 条件必需 |
| `HERALD_PII_PEPPER` | PII 派生 Redis/指标键的 pepper；生产至少 32 字节 | （空） | 建议 |

缺少凭据不会关闭认证。使用 `REQUEST_AUTH_MODE=hmac_v2`（默认）但未配置 `HMAC_SECRET`/`HERALD_HMAC_KEYS`，或使用 `api_key` 但未配置 `API_KEY` 时，Herald 在非生产环境启动时记录警告，所有需要认证的端点在配置凭据前均返回 401；生产环境拒绝启动。只有 `REQUEST_AUTH_MODE=none`（仅限开发/测试）接受未经认证的请求。

#### OTP/Challenge

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_MAX_CONCURRENT_HASH_OPERATIONS` | 每个 handler 实例中创建与 v1/v2 验证共享的并发预算，必须为正整数 | `4` | 否 |
| `HERALD_CODE_HASH_KEY` | 为 Redis 中保存的验证码摘要加密钥（HMAC-SHA-256）的密钥。留空则继续使用不带密钥的 Argon2 哈希。在任何环境下都至少 32 字节，否则启动失败。所有实例必须使用同一个值；参见[验证码哈希密钥](#验证码哈希密钥) | （空） | 建议 |
| `HERALD_CODE_HASH_VERIFY_ONLY` | 已设置 `HERALD_CODE_HASH_KEY` 时：新 challenge 仍保存 Argon2 哈希，同时可以验证带密钥的摘要。用于滚动变更密钥设置及其回滚的第一阶段 | `false` | 否 |
| `HERALD_CODE_HASH_KEY_PREVIOUS` | 以逗号分隔的密钥列表，最多 4 个：这些密钥只用于验证带密钥的摘要，从不用于写入。用于更换 `HERALD_CODE_HASH_KEY` 而不拒绝未完成的 challenge，见[更换密钥](#更换密钥)。在任何环境下每个密钥都至少 32 字节，否则启动失败。未设置 `HERALD_CODE_HASH_KEY` 时不起作用 | （空） | 否 |
| `CHALLENGE_EXPIRY` | Challenge 过期时间（如 `5m`、`300s`）。内置消息以整分钟表示该时间，向下取整；不足一分钟时显示为 1 分钟 | `5m` | 否 |
| `MAX_ATTEMPTS` | 单个 challenge 可接受的错误验证码次数，达到后该 challenge 耗尽并返回 `locked` | `5` | 否 |
| `LOCKOUT_DURATION` | 被锁定用户保持锁定的时长，也是统计已耗尽 challenge 的时间窗口（如 `10m`） | `10m` | 否 |
| `LOCKOUT_CHALLENGE_THRESHOLD` | 用户在 `LOCKOUT_DURATION` 内（自第一个耗尽的 challenge 起计）须耗尽多少个 challenge（每个 `MAX_ATTEMPTS` 次错误验证码）才会被锁定。`1` 表示第一个 challenge 耗尽即锁定，与 v1.6.0 的行为相同。最小值 `1` | `3` | 否 |
| `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` | 用户在 `LOCKOUT_FAILED_ATTEMPT_WINDOW` 内可提交的错误验证码次数，跨该用户的所有 challenge 累计，不论是否有 challenge 耗尽；达到后用户被锁定 `LOCKOUT_DURATION`。`0` 表示关闭此计数。见[安全说明](SECURITY.md#用户锁定) | `15` | 否 |
| `LOCKOUT_FAILED_ATTEMPT_WINDOW` | `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` 的统计窗口，自第一次错误验证码起计，不因后续错误而延长；除非阈值为 `0`，否则必须为正值 | `1h` | 否 |
| `RESEND_COOLDOWN` | 同一用户与 destination 两次创建 challenge 的最小间隔，同一 destination 不论用户也适用该间隔；必须为正值 | `60s` | 否 |
| `CODE_LENGTH` | 验证码位数（最小 `4`；还须在扣除紧凑的上下文绑定 V2 请求封装后不超过 `HERALD_MAX_BODY_BYTES`，当前封装为 136 字节） | `6` | 否 |
| `IDEMPOTENCY_KEY_TTL` | 发送前占位和已完成响应的 TTL；不确定发送使用 `IDEMPOTENCY_UNRESOLVED_TTL`；`0` 表示使用 `CHALLENGE_EXPIRY` | `0` | 否 |
| `IDEMPOTENCY_UNRESOLVED_TTL` | 已开始发送但尚未完成的幂等键持续返回 `409/delivery_unknown` 的时长。必须为正值，且大于 `IDEMPOTENCY_KEY_TTL`（该值为 `0` 时取 `CHALLENGE_EXPIRY`），否则启动失败。到期后该键可以发起新请求 | `24h` | 否 |
| `ALLOWED_PURPOSES` | 允许的 purpose，逗号分隔，如 `login,reset,bind,stepup` | `login` | 否 |
| `HERALD_VERIFY_V1_ENABLED` | 是否注册 `POST /v1/otp/verifications`（该路由不校验预期的用户、purpose 和渠道）。所有调用方改用 `POST /v2/otp/verifications` 后可设为 `false`，此后 v1 路由返回 404 | `true` | 否 |

#### 限流

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `RATE_LIMIT_PER_USER` | 每 user_id 每小时可创建 challenge 数 | `10` | 否 |
| `RATE_LIMIT_PER_IP` | 每 IP 每分钟可创建 challenge 数 | `5` | 否 |
| `RATE_LIMIT_PER_DESTINATION` | 每 destination（邮箱/手机）每小时可创建数 | `10` | 否 |

#### Provider 部署版本

固定使用 `ghcr.io/soulteary/herald-dingtalk:1.2.8`、
`ghcr.io/soulteary/herald-smtp:1.2.9`、
`ghcr.io/soulteary/herald-totp:2.0.4`，或经过核验的不可变摘要。
Herald v1.8.0 导入 `github.com/soulteary/herald-totp/v2` v2.0.4。SMTP 和钉钉
保留补丁版本接口。TOTP 保留 `/v1` HTTP 路由与凭据存储格式，但 HMAC 协议
升级为绑定操作并带 nonce 防重放的 v2；必须协调升级 TOTP 服务和 Herald
代理，旧 HMAC 签名不再接受。仅使用 API key 的模式仍可用。
保留 provider 凭据、TOTP 加密密钥和 Redis 数据。恢复流量前检查健康/就绪状态
及所用认证模式下的真实操作，并遵循[安全要求](SECURITY.md)与
[pending challenge 迁移说明](MIGRATION_V2.md#pending-challenge-验证协议)。邮件和钉钉发送有外部副作用，演练应使用隔离目标。

#### 邮件通道

**内置 SMTP**（当未设置 `HERALD_SMTP_API_URL` 时使用）：

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `SMTP_HOST` | SMTP 服务器主机 | （空） | 使用内置时 |
| `SMTP_PORT` | SMTP 端口；服务器必须在该端口上接受 STARTTLS | `587` | 否 |
| `SMTP_USER` | SMTP 用户名 | （空） | 否 |
| `SMTP_PASSWORD` | SMTP 密码 | （空） | 否 |
| `SMTP_FROM` | 发件人地址 | （空） | 建议 |
| `PROVIDER_FAILURE_POLICY` | 发送失败策略：`soft`（仍创建 challenge）或 `strict`（失败则不创建） | `soft`；`ENVIRONMENT=production` 时为 `strict` | 否 |

内置 SMTP 在任何环境下都会先用 STARTTLS 升级连接，然后才进行认证或发送，并按 `SMTP_HOST` 校验服务器证书（TLS 1.2 及以上）。没有可以关闭这一行为的配置项。如果服务器不提供 STARTTLS、拒绝 STARTTLS，或出示的证书对 `SMTP_HOST` 不受信任，发送即失败（`failure_category` 为 `rejected` 或 `tls`），密码和消息都不会发出。内置 SMTP 不支持隐式 TLS（SMTPS 端口 465），也不支持不带 TLS 的明文 SMTP。

生产环境禁止 `soft`。生产环境中该变量未设置或为空时，Herald 使用 `strict` 并记录日志 `Production default: PROVIDER_FAILURE_POLICY=strict`；显式设置 `soft` 仍会导致启动失败。

Provider 和 TOTP 的 Base URL（`HERALD_SMTP_API_URL`、`SMS_API_BASE_URL`、`HERALD_DINGTALK_API_URL`、`HERALD_TOTP_BASE_URL`）在生产环境必须使用 `https://`，使用明文 `http://` 会导致启动失败。其他环境接受 `http://` 并记录警告。

**herald-smtp 插件**（设置后不再使用内置 SMTP）：

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_SMTP_API_URL` | [herald-smtp](https://github.com/soulteary/herald-smtp) 服务 Base URL（如 `https://herald-smtp.internal:8084`） | （空） | 使用插件时 |
| `HERALD_SMTP_API_KEY` | 若 herald-smtp 启用 `API_KEY`，需与此一致 | （空） | 否 |

#### 短信通道（HTTP API 模式）

当前实现通过 HTTP 调用外部 SMS API，不直接使用阿里云等密钥；密钥由外部 SMS 服务或网关保管。

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `SMS_PROVIDER` | 提供商名称（如 `aliyun`、`tencent`、`http`），用于标识与日志 | （空） | 使用 SMS 时 |
| `SMS_API_BASE_URL` | SMS HTTP API 的 Base URL | （空） | 使用 SMS 时 |
| `SMS_API_KEY` | SMS API 认证密钥（若需要） | （空） | 视网关要求 |

`SMS_PROVIDER` 和 `SMS_API_BASE_URL` 必须同时设置：只有设置了 `SMS_PROVIDER` 才会注册 provider；设置了它却没有可用的 Base URL 时启动失败。只设置 Base URL 而不设置 `SMS_PROVIDER` 时，该通道仍处于未配置状态，并记录一条警告。

#### 没有 provider 的通道

启动时 Herald 记录一行日志，列出已有 provider 的通道和没有 provider 的通道（`channels_with_provider`、`channels_without_provider`）；所有通道都没有 provider 时另记一条警告。`PROVIDER_FAILURE_POLICY=strict` 时，针对没有 provider 的通道的创建请求返回 `503/channel_not_configured`，此时尚未使用任何配额、冷却或幂等键。`soft` 时仍创建 challenge，响应带有 `delivery_status=failed`，与此前相同。

发送失败的请求已经通过准入：无论哪种情况，该请求的按用户、按 IP 和按 destination 配额都视为已消耗。`PROVIDER_FAILURE_POLICY=strict` 时，两项重发冷却如何处理取决于失败的类型：

- 失败能够证明没有任何 provider 接受该消息时，冷却会被释放，重试不会得到 `429/resend_cooldown`。对于内置 SMTP，这对应 `failure_category` 为 `dns`、`connect`、`auth`、`validation`，以及服务器证书校验失败时的 `tls`。对于 HTTP provider（herald-smtp、herald-dingtalk、短信网关），这指无法解析或连接 provider 服务、或其证书校验失败，并且尚未向它写入任何请求。
- 其他失败之后冷却保持不变，因为消息可能已被接受：超时或请求被取消、连接建立后中断、服务器拒绝了某条 SMTP 命令、HTTP provider 服务返回了任何响应，以及任何无法识别的错误。此时在 `RESEND_COOLDOWN` 内对同一 destination 重试会得到 `429/resend_cooldown`，尽管并未确认送达。

失败日志带有 `cooldown_released`（`true` 或 `false`）。冷却释放后的重试仍计入 `RATE_LIMIT_PER_USER`、`RATE_LIMIT_PER_IP` 和 `RATE_LIMIT_PER_DESTINATION`。重复使用失败请求的 `Idempotency-Key` 进行重试，仍会得到 `409/delivery_unknown`，应换用新的键。`PROVIDER_FAILURE_POLICY=soft` 时响应为 `200` 且 `delivery_status=failed`，冷却保留。对于内置 SMTP，失败日志会指出投递停在哪一步，见[故障排查](TROUBLESHOOTING.md#提供者发送失败)。

#### DingTalk 通道（herald-dingtalk 插件）

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) 服务 Base URL（如 `https://herald-dingtalk.internal:8083`） | （空） | 使用 DingTalk 时 |
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
| `HERALD_TOTP_BASE_URL` | herald-totp 服务 Base URL（如 `https://herald-totp.internal:8085`） | （空） | 启用 TOTP 时 |
| `HERALD_TOTP_API_KEY` | Herald 调用 herald-totp 时使用的 API Key（若 herald-totp 需要） | （空） | 否 |
| `HERALD_TOTP_HMAC_SECRET` | Herald 调用 herald-totp 时使用的 HMAC 密钥（若 herald-totp 使用 HMAC） | （空） | 否 |
| `HERALD_TOTP_HMAC_KEY_ID` | TOTP 签名 key ID；后端配置多个密钥时需要显式指定 | （空，省略 ID） | 否 |
| `HERALD_TOTP_HMAC_SERVICE` | TOTP 签名的服务身份 | （空，保留 SDK 默认 `stargate`） | 否 |

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

#### 启动警告

有些配置虽然有效，但很可能不是本意。Herald 在启动时为每一项记录一条警告，并继续启动。
日志中的 `check` 字段标明是哪一项警告。

| `check` | 触发条件 | 影响 |
|---------|----------|------|
| `config.rate_limit_ip` | 未设置 `HERALD_TRUSTED_PROXY_HEADER` 和 `HERALD_TRUSTED_PROXIES` | `RATE_LIMIT_PER_IP` 按直连对端计数，因此代表众多终端用户调用 Herald 的网关会让所有用户共用一份配额 |
| `config.listener_tls` | `ENVIRONMENT=production` 且未设置 `TLS_CERT_FILE`/`TLS_KEY_FILE` | 验证请求的正文中带有验证码；`REQUEST_AUTH_MODE=api_key` 时每个请求还带有静态密钥，被截获后可以重放。如果 TLS 在 Herald 前面的代理处终止，应确保代理到 Herald 的这一段位于可信的本地网络内 |
| `config.code_length` | `CODE_LENGTH` 小于 `6` | 长度 `4` 和 `5` 可以使用，但每减少一位，单次猜中的概率就提高十倍，而尝试次数和限流保持不变 |
| `config.test_mode` | `HERALD_TEST_MODE=true` 而 `ENVIRONMENT` 不是 `test`（生产环境会直接拒绝启动） | 该开关单独设置不起作用：不会返回 `debug_code`，也不会启动测试码端点 |
| `config.totp` | `HERALD_TOTP_ENABLED=true` 且 `HERALD_TOTP_BASE_URL` 为空 | TOTP 代理未配置；所有 `/v1/totp` 请求返回 `503/totp_not_configured` |
| `config.otlp` | `OTLP_ENABLED=true` 且 `OTLP_ENDPOINT` 为空 | 没有配置 trace 导出器，不会导出任何 span |
| `config.code_hash` | `HERALD_CODE_HASH_VERIFY_ONLY=true` 且 `HERALD_CODE_HASH_KEY` 为空 | 该开关单独设置不起作用：验证码仍以不带密钥的 Argon2 哈希保存，也无法验证带密钥的摘要 |
| `config.code_hash_previous` | 设置了 `HERALD_CODE_HASH_KEY_PREVIOUS` 而 `HERALD_CODE_HASH_KEY` 为空，或列表中某个密钥与 `HERALD_CODE_HASH_KEY` 相同 | 没有主密钥时，任何密钥下的带密钥摘要都无法验证。与主密钥相同的列表项不增加任何密钥，通常说明两个变量没有按预期对调 |

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

能够读取但无法解码的待投递文件会尝试 5 次，之后被重命名为 `<id>.json.quarantined`。它仍留在
`AUDIT_SPOOL_DIR` 中，不再重试，重启后也不会重新处理，并且不计入 `AUDIT_SPOOL_MAX_BYTES`；
见[监控说明](MONITORING.md#审计投递)。无法读取的文件（I/O 错误、没有可用的文件描述符、
权限问题）不会被隔离：它保持待投递状态并持续重试，重试间隔从 1 秒起逐次加倍，最长 5 分钟，
直到可以读取为止。已从目录中删除的待投递文件不会重试：其记录已经丢失，Herald 只报告一次
（一条 error 日志和 `herald_audit_spool_missing_total`），该文件也不再计入
`AUDIT_SPOOL_MAX_BYTES`。后端投递失败同样不会导致隔离，这些记录会一直重试到后端接收为止。

请将 `AUDIT_SPOOL_DIR` 挂载到持久化本地存储；容器使用 UID/GID 10001，目录建议为 0700。
仓库 Compose 文件通过 `audit_spool` 卷挂载该目录，兼容只读根文件系统。
每个运行实例使用独立目录或卷，独占锁会拒绝并发占用。重启或升级时保留卷；
`docker compose down -v` 会删除缓冲，有待投递记录时不能使用。目录必须支持文件及目录的
fsync 和原子 rename。

目录不可用时启动失败；缓冲写满或运行中本地写入失败时，相关接口返回
`503/audit_unavailable`，不会返回业务成功。创建请求的首次审计失败会阻止发送；
验证和撤销操作会在改变 challenge 状态前记录尝试。在消息已被接受、验证码已被消费或撤销
已完成之后，结果记录写入失败不会改变响应：请求成功，
`herald_audit_post_action_failure_total{operation}` 加一，并以一条 error 日志记录该事件。
应对该计数器配置告警，因为这些事件不会出现在审计记录中。
对于不确定的发送结果，应使用相同幂等键查询或重试，不能直接补发。
待投递文件不自动过期，受字节上限约束，需监控磁盘空间和投递失败。

投递语义为**至少一次**：后端已提交但本地文件尚未删除时退出，会以相同 `event_id` 回放，
下游可据此去重。destination 在本地落盘前脱敏。请保护和备份持久卷；文件系统或卷丢失
不在保障范围内。后端审计 TTL 在投递后生效，不会淘汰本地待投递文件。
显式设置 `AUDIT_ENABLED=false` 会关闭审计及这些检查。


#### 模板与可观测性

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `TEMPLATE_DIR` | 邮件/短信模板目录（可选）；见[消息模板](#消息模板) | （空） | 否 |
| `OTLP_ENABLED` | 是否启用 OpenTelemetry | `false` | 否 |
| `OTLP_ENDPOINT` | OTLP/HTTP collector 地址：`host:port`（明文 HTTP，路径为 `/v1/traces`），或 `http://host:port[/path]` / `https://host:port[/path]`。`https` 使用 TLS，并以系统根证书校验 collector；设置了 `OTEL_EXPORTER_OTLP_CERTIFICATE` 时改用该变量指定的文件，见 [OTLP 导出器环境变量](#otlp-导出器环境变量)。`/` 以外的路径按原样用作请求路径；未给出路径时 span 发送到 `/v1/traces`。其他形式的取值会在启动时报告，并且不导出任何 span | （空） | 启用 OTLP 时 |

#### OTLP 导出器环境变量

Herald 创建 OTLP/HTTP 导出器时，只传入 `OTLP_ENDPOINT` 中的主机、路径和协议。除此之外，导出器库还会从 Herald 的环境中读取下列标准 `OTEL_EXPORTER_OTLP_*` 变量。每个变量都有一个只对 trace 生效的形式，即在 `OTLP_` 之后加上 `TRACES_`（例如 `OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE`）；两者都设置为可用的值时，以 trace 专用的为准。空值视为未设置。

| 变量 | 作用 |
|------|------|
| `OTEL_EXPORTER_OTLP_CERTIFICATE` | PEM 文件路径，其中的 CA 证书用于校验 collector。该文件会替换系统根证书，而不是在其基础上追加：证书只能链到公共 CA 的 collector 会被拒绝，除非该 CA 也在文件中。collector 使用私有 CA 时，就通过这个变量让导出器信任它。文件无法读取或不含证书时，导出器会在标准错误输出中报告，并继续使用系统根证书 |
| `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`、`OTEL_EXPORTER_OTLP_CLIENT_KEY` | 向 collector 出示的 PEM 客户端证书和私钥（双向 TLS）。两者必须同时设置，只设置其中一个会被忽略 |
| `OTEL_EXPORTER_OTLP_HEADERS` | 以逗号分隔的 `name=value` 列表，附加到每个导出请求上；值会做 URL 解码。trace 专用变量会替换整个列表。这些请求头会发送给 collector，通常带有凭据（例如 `Authorization=Bearer%20<token>`）：请把该变量当作密钥保管；另外，`OTLP_ENDPOINT` 为 `host:port` 或 `http://` 形式时，这些请求头以明文在网络上传输 |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | 单次导出请求的时间上限，单位为毫秒。默认 10000 |
| `OTEL_EXPORTER_OTLP_COMPRESSION` | 取值 `gzip` 时压缩请求体；其他取值表示不压缩（默认） |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | 取值 `http/json` 时以 JSON 代替 protobuf 发送。`grpc` 不会切换传输方式：Herald 只通过 HTTP 导出 |

有两点限制：

- `OTEL_EXPORTER_OTLP_ENDPOINT` 和 `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` 不会改变 span 的发送目标。主机和路径始终取自 `OTLP_ENDPOINT`。
- 证书和客户端证书变量要求 `OTLP_ENDPOINT` 为 `https://` 形式。如果端点是 `host:port` 或 `http://` 形式，而其中某个变量指向了可用的文件，导出器不会被创建：Herald 记录 `Failed to initialize OpenTelemetry tracing`，不导出任何 span。

协议同样始终取自 `OTLP_ENDPOINT`。`OTEL_EXPORTER_OTLP_INSECURE`、`OTEL_EXPORTER_OTLP_TRACES_INSECURE`，以及把上述任一 `..._ENDPOINT` 变量设为 `http://` 地址，都不会让 `https://` 形式的 `OTLP_ENDPOINT` 以明文 HTTP 导出；把它们设为 `https://` 地址，也不会让 `host:port` 或 `http://` 形式的端点改用 TLS。

导出器同样遵循 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY`。

#### 测试与调试

| 变量 | 描述 | 默认值 | 必需 |
|------|------|--------|------|
| `HERALD_TEST_MODE` | 仅在 `ENVIRONMENT=test` 时启用测试码暴露；生产禁止 | `false` | 否 |
| `HERALD_TEST_API_KEY` | 测试码监听器专用密钥 | （空） | 测试模式 |
| `HERALD_TEST_LISTENER_ADDR` | 独立、仅回环地址的测试监听器 | `127.0.0.1:18082` | 否 |

### 测试模式与调试

仅当同时设置 `ENVIRONMENT=test` 和 `HERALD_TEST_MODE=true` 时：

- 创建 challenge 的响应中会包含 `debug_code` 字段（明文验证码），便于调用方（如 Stargate 在 `DEBUG=true` 时）在登录页直接展示。
- `GET /v1/test/code/:challenge_id` 挂载在独立的 `HERALD_TEST_LISTENER_ADDR` 监听器上（默认 `127.0.0.1:18082`），不在主监听器上。请求须携带 `X-Test-Api-Key: $HERALD_TEST_API_KEY`（或使用 Bearer 认证）。

**安全：** 生产环境必须设置 `HERALD_TEST_MODE=false`。测试模式会暴露验证码，不得在生产环境使用。

### 消息模板

未设置 `TEMPLATE_DIR` 时，Herald 发送内置文本。设置后，Herald 在启动时按以下目录结构一次性
加载自定义模板：

```text
{TEMPLATE_DIR}/{locale}/{channel}/{purpose}.txt

templates/
├── en/
│   ├── email/login.txt
│   └── sms/login.txt
└── zh-CN/
    ├── email/login.txt
    └── sms/reset.txt
```

- `{locale}` 是语言或区域标签，例如 `en`、`zh`、`zh-CN` 或 `pt_BR`；不区分大小写，`-` 与
  `_` 等价。模板只按语言选择：`zh-CN`、`zh_TW` 和 `zh` 是同一种语言，请求中的 `locale`
  为其中任何一个时也是如此。如果两个目录对应同一种语言，使用按名称排序的第一个，另一个在
  启动时报告。`locale` 未知或为空的请求使用 `en`。
- `{channel}` 为 `email` 或 `sms`。DingTalk 消息使用 `sms` 模板。
- `{purpose}` 是 `ALLOWED_PURPOSES` 中的值，例如 `login`。文件扩展名会被忽略。

模板文件是 Go `text/template` 源文件。可用字段为 `{{.Code}}`、`{{.ExpiresIn}}`（秒）、
`{{.Purpose}}` 和 `{{.Locale}}`；使用其他字段会出错。对于 `email`，第一行是主题，其余为
正文；只有一行的模板用作正文，主题为 `Verification Code`。文件可以使用 LF 或 CRLF 换行。
如果邮件模板的主题行中仍含有回车符（不属于行尾换行的回车符）或 NUL 字符，该模板不会被
加载，因为 SMTP provider 会拒绝这样的主题。渲染后的消息不得超过 16 KiB。

无法读取或解析的文件、位于其他目录层级的文件，以及 locale 或 channel 无法识别的文件不会被
加载。Herald 为每个这样的文件记录一条警告（`check=templates`，消息为
`template not loaded: ...`），并继续启动。模板缺失或渲染失败时回退到内置文本，因此模板
出错只会改变消息的措辞，不会中断发送。修改模板后需重启 Herald。

### Email 通道（herald-smtp）

当设置 `HERALD_SMTP_API_URL` 时，Herald 不再使用内置 SMTP，而是通过 HTTP 将邮件发送请求转发给 [herald-smtp](https://github.com/soulteary/herald-smtp)。所有 SMTP 凭证与发送逻辑均在 herald-smtp 中；此模式下 Herald 不保存任何 email 通道的 SMTP 凭证。

- 将 `HERALD_SMTP_API_URL` 设置为 herald-smtp 服务的基础 URL（例如 `https://herald-smtp.internal:8084`）。
- 若 herald-smtp 配置了 `API_KEY`，请将 `HERALD_SMTP_API_KEY` 设为相同值，以便 Herald 调用 herald-smtp 时通过认证。
- 设置 `HERALD_SMTP_API_URL` 后，Herald 会忽略 `SMTP_HOST` 等内置 SMTP 配置（email 通道不再使用它们）。

### DingTalk 通道（herald-dingtalk）

当 `channel` 为 `dingtalk` 时，Herald 不直接发送消息，而是通过 HTTP 将发送请求转发给 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)。所有钉钉凭证与业务逻辑均在 herald-dingtalk 中；Herald 不保存任何钉钉凭证。

- 将 `HERALD_DINGTALK_API_URL` 设置为 herald-dingtalk 服务的基础 URL（例如 `https://herald-dingtalk.internal:8083`）。
- 若 herald-dingtalk 配置了 `API_KEY`，请将 `HERALD_DINGTALK_API_KEY` 设为相同值，以便 Herald 调用 herald-dingtalk 时通过认证。

### TOTP（herald-totp）

当 `HERALD_TOTP_ENABLED=true` 且设置了 `HERALD_TOTP_BASE_URL` 时，Herald 会将 TOTP（Authenticator）请求代理到 [herald-totp](https://github.com/soulteary/herald-totp)。Stargate 等调用方可使用同一 Herald 地址同时完成 OTP（短信/邮件/钉钉）与 TOTP 流程。

- 将 `HERALD_TOTP_BASE_URL` 设置为 herald-totp 服务的基础 URL（例如 `https://herald-totp.internal:8085`）。
- 若 herald-totp 需要 API Key 或 HMAC 认证，请相应设置 `HERALD_TOTP_API_KEY` 或 `HERALD_TOTP_HMAC_SECRET`。
- TOTP 端点（status、verify、enroll/start、enroll/confirm、revoke）见 [API.md](API.md#totp-代理可选)。

### 插件启用条件速查

| 能力 | 启用条件 | 备注 |
|------|----------|------|
| 内置 SMTP | 未设置 `HERALD_SMTP_API_URL` 且 `SMTP_HOST` 非空 | 使用 `SMTP_*` 直连 SMTP 服务 |
| herald-smtp | `HERALD_SMTP_API_URL` 非空 | 设置后会覆盖内置 SMTP 通道 |
| SMS HTTP API | `SMS_PROVIDER` 非空，且必须同时设置 `SMS_API_BASE_URL` | 缺少 `SMS_API_BASE_URL` 时启动失败；`SMS_API_KEY` 视网关要求 |
| herald-dingtalk | `HERALD_DINGTALK_API_URL` 非空 | 可选 `HERALD_DINGTALK_API_KEY` |
| herald-totp 代理 | `HERALD_TOTP_ENABLED=true` 且 `HERALD_TOTP_BASE_URL` 非空 | 可选 `HERALD_TOTP_API_KEY` / `HERALD_TOTP_HMAC_SECRET` |
| OTLP | `OTLP_ENABLED=true` 且 `OTLP_ENDPOINT` 非空 | endpoint 为空时不导出任何 span |

### Redis 配置

#### 独立 Redis 实例

对于生产部署，**强烈建议**为 Herald 使用专用的 Redis 实例或单独的数据库索引，以避免与其他服务的键空间冲突。

**键前缀：**
- `otp2:ch:*` - 挑战数据（TTL：挑战过期时间）
- `ratelimit:*` - 速率限制计数器（TTL：速率限制窗口）
- `otp:idem2:*` - 幂等记录（发送前占位和已完成响应：`IDEMPOTENCY_KEY_TTL`；不确定发送：`IDEMPOTENCY_UNRESOLVED_TTL`）
- `otp:lock:v2:<digest>` - 用户锁定记录（TTL：锁定持续时间）。`<digest>` 是由用户 ID 结合 `HERALD_PII_PEPPER` 派生的摘要
- `otp:lock:exhausted:<digest>` - 计入锁定的已耗尽 challenge（TTL：自第一个起计的锁定持续时间）
- `otp:lock:failed:<digest>` - 跨 challenge 计入锁定的错误验证码（TTL：自第一次起计的 `LOCKOUT_FAILED_ATTEMPT_WINDOW`）

**不确定的发送记录：**

幂等存储应使用持久化 Redis，并采用 `noeviction` 策略。要阻止重发，已确认的写入就必须
在重试期间保留；Redis 丢失或淘汰该记录后，Herald 无法保证不重复发送。请按所需的可靠性
配置 Redis 持久化和故障转移。在存在未核对记录或仍可能重试时，不要清空数据库或轮换
`HERALD_IDEMPOTENCY_SECRET`，否则派生键变化会绕过旧记录。

`sending` 状态记录在 `IDEMPOTENCY_UNRESOLVED_TTL`（默认 `24h`）后过期，包含用于核对的
`challenge_id`。在此之前该键返回 `409/delivery_unknown`。监控 `delivery_unknown` 幂等
结果和未完成记录数量。管理员处理记录前，应先用该 challenge ID（也是传给渠道的幂等键）
查询渠道投递日志。不要提前删除这些记录：原请求可能仍在执行，其消息也可能已经送达。
确需补发时，应作为一项单独、明确的业务决定处理。TTL 到期后记录被移除，使用相同键的请求
视为新请求，因此取值应长于客户端对同一个键持续重试的时间。在 TTL 内，这保证不确定键最多
尝试发送一次，不保证投递成功。

`IDEMPOTENCY_UNRESOLVED_TTL` 出现之前的版本写入的无 TTL 记录不会被修改，也不会过期。
仍按原有方式核对并移除这些记录。

**容量规划：**

使用代表性流量测量载荷和 Redis 开销，计入 challenge、激活索引、会过期的已完成幂等响应、
在 `IDEMPOTENCY_UNRESOLVED_TTL` 内保留的不确定发送、限流/冷却、nonce 和审计保留。峰值速率乘以 challenge TTL 不能
界定总容量。持久化审计缓冲磁盘及进程哈希预算需另行规划，并结合 `MEMORY USAGE`、键数量
和保留记录增长量调整配置。

`REDIS_ADDR` 指定单一 Redis 端点；Herald 未提供原生 Cluster 或 Sentinel 发现配置。
高可用部署使用外部管理的故障切换端点并保留数据。多键激活脚本和幂等脚本不兼容 Redis Cluster。

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

先为 Herald 配置独立的 `HERALD_METRICS_API_KEY`，并把相同值挂载到 Prometheus 的 `/run/secrets/herald_metrics_key`。不得复用业务 API/HMAC 密钥。

**Prometheus 抓取配置示例：**
```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
```

详细的监控文档，请参阅 [MONITORING.md](MONITORING.md)。

## 安全

- 生产环境使用 HMAC 或 API Key 请求认证，并可叠加 mTLS 传输保护；客户端证书不能替代配置的请求凭据
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
视作进程 RSS 的硬上限。超额请求不排队，返回 `503/hash_capacity_exceeded` 和 `Retry-After: 1`（v1.6.0 及更早版本返回 `503/backend_unavailable`）。该原因码持续出现说明预算相对负载偏小，与 Redis 健康状况无关。
创建准入先于用户锁定检查以及限流和冷却步骤；Redis 可用时会释放该请求的发送前幂等占位。即使预算已满，
已完成幂等响应仍可重放。被拒绝的验证不会消费验证码或增加尝试次数。

验码锁发生竞争时只立即尝试一次，返回现有的可重试 `409/lock_contention`，不占用预算轮询等待。
创建槽位在调用发送渠道前释放，慢渠道不会占住哈希预算。重试保留原幂等键，并使用新的 HMAC
时间戳与 nonce。现有 Redis 记录及签名格式不变。

## 验证码哈希密钥

默认情况下，Herald 在 challenge 记录（`otp2:ch:*`）中保存验证码的 Argon2 哈希。Argon2
不使用任何密钥，而 6 位验证码只有 10^6 个候选值，因此在 challenge 有效期内能读到该记录的人
（Redis 访问权限、从库、快照或备份）可以离线逐个尝试候选值，找出验证码。

设置 `HERALD_CODE_HASH_KEY` 后，新 challenge 保存的值变为 `hmac-sha256$v1$<摘要>`，即用该
密钥对 challenge ID 和验证码计算的 HMAC-SHA-256。没有密钥就无法用候选验证码去比对保存的值。
前提是读取 Redis 的人不知道密钥：同时拿到密钥和保存的值，就能立即还原验证码。

- 密钥只生成一次，例如 `openssl rand -hex 32`，保存在密钥管理系统中，并向每个实例注入同一个
  值。不要把它保存在存放 challenge 的 Redis 实例中，也不要放进随该 Redis 一起备份的任何位置。
  不要复用其他密钥。
- 密钥至少 32 字节。该检查在任何环境下都生效，不限于生产环境：长度不足会使启动失败，错误信息
  中会写明 `HERALD_CODE_HASH_KEY`。没有 `HERALD_CODE_HASH_KEY_FILE`；与其他密钥一样，只从
  环境变量读取。
- Herald 不会把密钥写入日志。启动日志 `Configuration initialized` 一行以 `code_hash` 字段
  报告当前方案：`argon2`、`hmac-sha256` 或 `argon2 (hmac-sha256 verify-only)`，并以
  `code_hash_previous_keys` 字段报告从 `HERALD_CODE_HASH_KEY_PREVIOUS` 读到的密钥个数。
- 尝试次数、用户锁定、限流和过期行为不变。创建与验证仍受
  `HERALD_MAX_CONCURRENT_HASH_OPERATIONS` 准入预算约束；计算带密钥的摘要不需要 Argon2
  所占用的内存。

### 对未完成 challenge 的影响

challenge 从创建到验证成功或过期为止，一直保留创建时写入的值；最长为创建后的
`CHALLENGE_EXPIRY`（默认 `5m`）。

| 变更 | 变更前创建的 challenge |
|------|------------------------|
| 首次设置密钥 | 继续可以验证。保存的 Argon2 哈希始终用 Argon2 比对 |
| 移除密钥，或请求落到没有密钥的实例（包括 v1.7.0 及更早版本） | 正确的验证码会像错误验证码一样返回 `401/invalid`，并消耗一次尝试，直到 challenge 过期 |
| 更换密钥，或各实例密钥不一致，且负责验证的实例没有把创建时所用的密钥列入 `HERALD_CODE_HASH_KEY_PREVIOUS` | 同上：正确的验证码返回 `401/invalid` 并消耗一次尝试，直到 challenge 过期 |
| 更换密钥，且负责验证的实例已把创建时所用的密钥列入 `HERALD_CODE_HASH_KEY_PREVIOUS` | 继续可以验证 |

以这种方式被拒绝的验证码与普通错误验证码同样计数：会减少 `remaining_attempts`，并计入
`LOCKOUT_FAILED_ATTEMPT_THRESHOLD`。用户需要重新申请验证码。Herald 只用一个密钥
（`HERALD_CODE_HASH_KEY`）写入，验证时使用该密钥以及 `HERALD_CODE_HASH_KEY_PREVIOUS` 中
列出的密钥。

### 单实例启用

设置 `HERALD_CODE_HASH_KEY` 并重启。重启前创建的 challenge 继续可以验证，新 challenge 以带
密钥的摘要保存。

### 多实例启用

滚动重启期间，尚未拿到密钥的实例（或仍运行 v1.7.0 及更早版本的实例）无法验证已有密钥的实例
创建的 challenge。要避免验证码被拒绝，请分两个阶段变更：

1. 所有实例部署本版本，设置 `HERALD_CODE_HASH_KEY` 并设置
   `HERALD_CODE_HASH_VERIFY_ONLY=true`。新 challenge 仍以 Argon2 保存，每个实例都能验证
   两种形式。
2. 所有实例都以上述方式运行后，移除 `HERALD_CODE_HASH_VERIFY_ONLY` 并重启。新 challenge 以
   带密钥的摘要保存。

如果可以接受重启期间最长 `CHALLENGE_EXPIRY` 内的验证码被拒绝，也可以跳过第一阶段，直接在
所有实例上设置密钥。

### 更换密钥

`HERALD_CODE_HASH_KEY_PREVIOUS` 列出的密钥只用于验证带密钥的摘要，从不用于写入。保存的值
不记录它由哪个密钥写入，因此 Herald 会用 `HERALD_CODE_HASH_KEY` 和列表中的每个密钥分别比对
提交的验证码；在所有密钥下都不匹配的验证码是错误验证码。每次验证为每个密钥计算一次
HMAC-SHA-256。

- 取值是以逗号分隔的列表，最多 4 个密钥。每一项两端的空白会被去掉，空项会被忽略。每个密钥
  至少 32 字节；长度不足或出现第 5 个密钥时，在任何环境下启动都会失败，错误信息只给出位置和
  长度，不包含密钥。
- 包含逗号，或以空白开头或结尾的密钥无法列入。要更换这样的密钥，请使用
  [移除密钥或回滚](#移除密钥或回滚)中的 verify-only 步骤。
- 只要还有用某个列出的密钥写入的未完成 challenge，该密钥就与 `HERALD_CODE_HASH_KEY` 同样
  敏感。同样不要把它放进 Redis，并按下面的步骤及时移除。
- 所有实例都必须运行支持该变量的版本。v1.8.0 及更早版本会忽略它，开始之前请先升级全部实例。

下文中 `A` 是当前使用的密钥，`B` 是新密钥。

**单实例，或所有实例一起重启**

1. 设置 `HERALD_CODE_HASH_KEY=B` 和 `HERALD_CODE_HASH_KEY_PREVIOUS=A` 并重启。新 challenge
   用 `B` 写入；用 `A` 写入的 challenge 继续可以验证。
2. 重启后至少等待 `CHALLENGE_EXPIRY`，再移除 `HERALD_CODE_HASH_KEY_PREVIOUS` 并重启。

**多个实例逐个重启**

验证请求可能落到任何实例，而不一定是创建该 challenge 的实例。如果一个实例已经开始用 `B`
写入，而另一个实例还只知道 `A`，后者会对正确的验证码返回 `401/invalid` 并消耗一次尝试。
在新实例上把 `A` 列为 previous 解决不了这个问题：它让新实例能验证旧 challenge，却不能让
旧实例验证新 challenge。因此在任何实例用 `B` 写入之前，所有实例都必须已经能够验证 `B`，
也就是说第一步里 `B` 才是“previous”密钥：

1. 在所有实例上设置 `HERALD_CODE_HASH_KEY_PREVIOUS=B`，保持 `HERALD_CODE_HASH_KEY=A`，
   逐个重启。保存的值没有任何变化。等到所有实例都以这种方式运行（每个实例启动时报告
   `code_hash_previous_keys: 1`）。
2. 在所有实例上设置 `HERALD_CODE_HASH_KEY=B` 和 `HERALD_CODE_HASH_KEY_PREVIOUS=A`，逐个
   重启。已重启的实例用 `B` 写入，仍处于第 1 步的实例通过 previous 密钥验证这些 challenge；
   仍处于第 1 步的实例用 `A` 写入，已重启的实例同样通过 previous 密钥验证它们。
3. 第 2 步中最后一个实例重启后，至少等待 `CHALLENGE_EXPIRY`。此后不再有用 `A` 写入的未完成
   challenge。
4. 从所有实例上移除 `HERALD_CODE_HASH_KEY_PREVIOUS`，逐个重启。此时可以销毁 `A`。

不要把第 1 步和第 2 步合并，也不要在第 3 步的等待结束之前开始第 4 步：这两种做法都会在最长
`CHALLENGE_EXPIRY` 的时间内造成[对未完成 challenge 的影响](#对未完成-challenge-的影响)中
所述的拒绝。

在第 2 步期间回退时，把所有实例逐个改回第 1 步的设置（`HERALD_CODE_HASH_KEY=A`、
`HERALD_CODE_HASH_KEY_PREVIOUS=B`），并在最后一个实例停止用 `B` 写入之后至少等待
`CHALLENGE_EXPIRY`，再移除 `B`。从第 1 步回退不需要等待：还没有任何值是用 `B` 写入的。

如果更换 `A` 是因为它可能已被他人获知，第一步会让切换推迟一轮滚动重启。按单步设置让所有
实例一起重启更快。两种情况下，用 `A` 写入的 challenge 在过期之前，对同时持有 `A` 和保存值
的人来说仍然可以穷举；把 `A` 列为 previous 不改变这一点。

`HERALD_CODE_HASH_VERIFY_ONLY=true` 可以与该列表同时使用：新 challenge 以 Argon2 保存，
带密钥的摘要在每个密钥下都能验证。

### 移除密钥或回滚

1. 在所有实例上设置 `HERALD_CODE_HASH_VERIFY_ONLY=true`，保留当前密钥并重启。
2. 最后一个实例重启后至少等待 `CHALLENGE_EXPIRY`。此后不再有以带密钥摘要保存的未完成
   challenge。
3. 在所有实例上移除 `HERALD_CODE_HASH_KEY` 和 `HERALD_CODE_HASH_KEY_PREVIOUS`，或降级到
   v1.7.0。

同样的步骤也可用于更换无法列入 `HERALD_CODE_HASH_KEY_PREVIOUS` 的密钥：第 3 步中改为设置
新密钥而不是移除，待所有实例都使用新密钥后，再移除 `HERALD_CODE_HASH_VERIFY_ONLY`。

跳过这些步骤不会造成持久影响：未完成的带密钥 challenge 会按上文所述被拒绝，并在
`CHALLENGE_EXPIRY` 内过期。两个方向都不需要修改或删除 Redis 数据。

## HMAC 身份升级

HMAC 部署先暂停创建流量、排空在途创建请求，再替换全部旧实例后恢复。混合版本仍可能拆分
隐式和显式 Key ID 命名空间，因为旧代码不会检查新规范记录。保留 Redis 数据、密钥映射和
`HERALD_IDEMPOTENCY_SECRET`；改变派生密钥会绕过全部已有幂等记录。

旧空 Key ID 记录保持原样，相同服务/客户端键返回 `409/idempotency_namespace_conflict`，
需由运维核对，不能改键自动重试。更早版本写入的无 TTL 发送记录仍不带 TTL。不能根据当前默认密钥猜测
原凭据，也不能在密钥轮换后盲目重放或迁移旧响应。没有旧记录冲突时，仍获授权的显式 Key ID 记录继续正常使用。

旧版本无法执行此保护。回滚前保持创建停用，直到受影响记录和调用身份已核对完成，且不能在
旧代码上恢复隐式 Key ID 调用方，细节见 [API](API.md#hmac-身份与旧幂等记录)。

### 单密钥 Key ID 白名单

`HMAC_SECRET` 只授权隐式身份 `default`。此前任意显式 ID 都会选中同一密钥，却建立
独立 nonce/幂等命名空间；现在未配置 ID 返回 `401/invalid_signature`，与签名错误相同（v1.6.0 及更早版本返回 `401/invalid_key_id`）。

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

### 服务权限与指标凭据

`HERALD_SERVICE_POLICIES` 为可选配置，用于把每个已配置的 HMAC Key ID 绑定到一个服务及其
允许的操作。它要求使用 HMAC v2/v3 认证，且旧版 v1 兼容处于关闭状态（`HMAC_V1_ENABLED=false`），否则启动失败。策略格式以及共享用户命名空间
的限制见[服务操作策略](SECURITY.md#限制服务操作)。

生产环境抓取指标现在要求配置 `HERALD_METRICS_API_KEY`（至少 32 字节）；未配置时
`/metrics` 返回 401。部署前应配置一个不同于业务凭据的随机密钥，并更新 Prometheus。见
[指标认证](MONITORING.md#指标认证)。

未知的 `ENVIRONMENT` 和 `REQUEST_AUTH_MODE` 值会在启动前被拒绝。受支持的认证别名通过与
路由相同的解析器规范化。显式设置的开发和测试环境保留原有策略；未设置环境时仍默认为
开发环境。


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


### TOTP 代理签名身份与密钥轮换

配置 `HERALD_TOTP_HMAC_SECRET` 时，使用 `HERALD_TOTP_HMAC_KEY_ID` 指定后端映射中的 key ID，可选设置 `HERALD_TOTP_HMAC_SERVICE`。两项遵循 SDK 的 128 字节上限，不接受控制字符或首尾空白。不设置时仍省略 key ID，并保留 SDK 默认服务名 `stargate`。仅 API key 配置不变；代理使用 HMAC 时不发送 API key。

旧 Herald 无法显式选择 TOTP 签名密钥。省略 ID 可用于单密钥映射，但无法在不同密钥重叠期间选择目标密钥。先在全部代理部署本次改动，显式配置现有 key ID 与对应密钥并验证调用，再向后端添加新密钥。随后将代理切换为新的 ID 与对应密钥，排空旧在途调用，确认没有调用方依赖旧密钥后才移除它。不要复用 nonce 或清空重放记录来测试轮换。

回滚代理会恢复省略 ID 和默认服务名的行为。回滚前应把后端协调为与旧代理密钥和预期服务身份兼容的单密钥配置，否则继续使用支持显式身份的代理版本。保留 TOTP Redis 数据、加密密钥、nonce/配额状态及 Herald 幂等和审计状态。本次是可选代理配置支持，不迁移 HTTP 协议或凭据记录。


## 从 v1.4.2 升级到 v1.4.3

使用 `HERALD_HMAC_KEYS` 的验证端升级前，应先为全部 HMAC v2/v3 调用方配置
显式 key ID，单项映射也不例外。Go SDK 设置 `Options.KeyID` (`WithKeyID`)，其他客户端发送参与
签名的 `X-Key-Id`。验证端会在消耗 nonce 前以 `401/key_id_required` 拒绝省略
ID 的请求。单一 `HMAC_SECRET` 配置仍支持隐式 `default`。旧 v1 仍受原兼容
开关控制且没有重放保护，不应作为安全轮换的替代方案。

SDK 配置 HMAC secret 后只发送 HMAC 凭据，失败时不以 API key 重试。连接仅
接受 API key 的服务时，应清空调用方 HMAC secret 并配置 API key。公开 Go
方法签名、HTTP 数据格式及 HMAC 签名格式不变。TOTP 代理可选的 key ID 和服务
配置见上节；先部署并验证显式身份，再开始密钥重叠。

保留仍在使用的 key ID、Redis 命名空间、nonce 记录、幂等密钥、审计状态及
TOTP 加密密钥和配额状态，无需转换记录或迁移命名空间。混合部署中的旧验证端
仍接受省略的映射 ID，须完成验证端替换才能全面执行新要求。回滚验证端会恢复
接受省略 ID 的行为；调用方单独回滚到旧 SDK 后，可能恢复同时发送两类凭据。
仍接受省略 ID 的请求时，不应更改默认 ID
或以相同 secret 重命名映射密钥。代理回滚还须先按上节协调后端单密钥及服务
身份配置。


## 从 v1.4.3 升级到 v1.4.4

先部署 herald-totp v2.0.3，再部署 Herald v1.4.4。依赖更新包含 secure-kit
v2.1.4 的 Unicode 脱敏修复及对应 audit/challenge/TOTP 模块。ASCII 输出、公开
SDK 方法、HTTP 成功响应、HMAC v2/v3 格式和映射 key ID 显式身份规则不变。
升级 key map 验证端前仍须先配置调用方 key ID；仅用 API key 的调用方应清空
HMAC secret。

保留 Redis 记录和命名空间、待激活挑战与注册状态、幂等密钥及未决发送、nonce、
配额、计数器及恢复码消费状态、审计缓冲和原 TOTP 加密密钥，无需转换记录、
迁移密钥或重新绑定。已有激活与快照提交校验保持有效。已有 Herald v1.4.3
二进制或镜像使用其原先编译的依赖。回滚不会撤销已保存的诊断值或已完成请求，
也不会随之回滚独立 TOTP 服务。

从源码重建 Go 应用时，应恢复升级前的 `go.mod` 和 `go.sum` 快照，并在构建前用
`go list -m all` 检查实际选中的版本。仅降低 Herald 版本，可能因 Go 最小版本
选择或其他直接依赖仍保留新的 audit/challenge/TOTP/secure-kit；须核实选中版本。
更换后端密钥时仍须遵循此前代理密钥轮换与回滚要求。

## 重试时间元数据升级与回滚（v1.5.0）

Herald v1.5.0 为创建验证码的配额与冷却错误提供可选重试时间。SDK 使用方选择
`github.com/soulteary/herald@v1.5.0` 后，可调用 `CreateChallengeWithMetadata`。
原有 `CreateChallenge` 签名和公开错误类型不变。新增公开 SDK 方法属于兼容的次版本更新。

更新 SDK 不会更新服务。新版 SDK 可以调用旧服务，缺少时间时保持未知。元数据仅表示
最早可尝试的时机，不保证之后一定放行。不要自动重试验证码创建或验证、重置配额，
也不要在缺少元数据时编造等待时间。

本次依赖与配置相对 v1.4.4 不变。升级与回滚保留 Redis 命名空间、待处理 challenge、
幂等及重放状态、审计缓冲和提供方/TOTP 凭据。不需要结构转换或重新绑定，TOTP 服务仍须独立部署。

服务回滚至 v1.4.4 后不再提供新的可选时间字段，但原有错误状态及状态规则保持不变。
调用新方法的客户端须先撤回该调用，再降低模块版本。源码回滚应恢复对应源码和应用模块清单，
重新构建前核对实际选中的模块版本。上文更早的迁移要求仍然适用。

## 从 v1.5.2 升级到 v1.6.0

Herald v1.6.0 新增可选配置和一个 SDK 选项。既有 SDK 方法、HTTP 成功响应、认证模式、
Redis 记录与凭据格式不变，无需转换数据、迁移密钥或重新绑定。因新增配置项和公开
SDK API，本次为次版本更新。

部署前请检查配置。以下取值此前会被接受或被默认值静默替代，现在会阻止启动：

- `RESEND_COOLDOWN=0`，所有环境均拒绝，须使用正的时长。
- 生产环境中，已设置但无效的 `REDIS_ADDR`，以及已设置但格式错误的
  `HERALD_HMAC_KEYS`。
- 生产环境中，`strconv.ParseBool` 不接受的布尔值（如 `yes`、`on`），以及无法解析的
  整数或时长（如未带单位的 `HMAC_MAX_DRIFT=60`）。其他环境记录警告并继续使用默认值。

其他需要关注的行为变化：

- 写作 `+86...`、`86...` 和 `0086...` 的同一短信号码现在共用 destination 配额和
  重发冷却。以 `+` 或 `00` 书写的号码在旧键下的计数不会迁移，升级后从新的配额开始；
  滚动升级期间，新旧实例对这类号码分别计数。
- 追踪 span 以 `user_id_digest` 取代 `user_id`。按 `user_id` 过滤的追踪查询和看板
  需要调整，审计记录不变。
- Fiber v3.7.0 不再把请求 trailer 当作请求头读取，认证和幂等请求头须放在头部。
  在最后一个分块之前结束的分块请求体返回 400 `request_error`。
- 缺少服务凭据时的启动警告文案已更改，匹配旧文案的日志告警需要更新。

新增配置均需显式启用。`REDIS_USERNAME`、`REDIS_TLS_ENABLED`、`REDIS_TLS_CA_FILE`
和 `REDIS_TLS_SERVER_NAME` 默认关闭或为空，应在 Redis 服务端支持 TLS 或已创建 ACL
用户后再启用。生产环境中，设置了 CA 文件或服务器名称但未设置
`REDIS_TLS_ENABLED=true`，或设置了用户名但未设置 `REDIS_PASSWORD`，都会阻止启动。
`HERALD_VERIFY_V1_ENABLED` 默认为 `true`，仅在所有调用方改用
`POST /v2/otp/verifications` 后再设为 `false`。Go SDK 的 `VerifyChallenge` 调用 v1，
应先改用 `VerifyChallengeV2`。

选择 `github.com/soulteary/herald@v1.6.0` 的 SDK 使用方需要 Go 1.27.4 或以上。
`WithClientIPHeader` 仅在设置后才转发终端用户地址；Herald 只在配置了
`HERALD_TRUSTED_PROXY_HEADER` 且对端位于 `HERALD_TRUSTED_PROXIES` 时读取该请求头。
当 Herald 返回 200 但不含 `ok: true` 时，`TOTPRevoke` 现在返回原因为
`revoke_not_confirmed` 的错误，应按撤销失败处理。更新 SDK 不会更新服务，新版 SDK
可以调用旧服务。

Herald 导入 `github.com/soulteary/herald-totp/v2` v2.0.4，该客户端模块仅更新依赖。
TOTP 服务仍须独立部署，协议与 v2.0.3 相同。

服务回滚至 v1.5.2 时保留 Redis 数据、待处理 challenge、幂等及重放状态、审计缓冲和
提供方/TOTP 凭据。v1.5.2 会忽略 Redis TLS 与 ACL 用户名配置，回滚前须恢复其可连接的
Redis 端点。回滚后会重新注册 v1 验证路由，span 重新输出 `user_id`；以 `+` 或 `00`
书写的号码在规范化键下记录的短信配额不会被 v1.5.2 读取。使用 `WithClientIPHeader`
的客户端须先移除该调用，再降低模块版本。从源码重建 Go 应用时，应恢复升级前的
`go.mod` 和 `go.sum`，并在构建前用 `go list -m all` 核对实际选中的版本。上文更早的
迁移要求仍然适用。

## 从 v1.7.0 升级到 v1.8.0

Herald v1.8.0 新增 `HERALD_CODE_HASH_KEY` 和 `HERALD_CODE_HASH_VERIFY_ONLY`，
并将 `challenge-kit` 升级到 v1.13.0。HTTP 路由、请求与响应、错误原因码、认证方式、
指标、审计 spool 文件格式和 Go SDK（`pkg/herald`）均无变化。`herald-totp/v2`
保持 v2.0.4，各 provider 服务无需升级。无需数据转换或密钥迁移。因新增配置项，
本次为次版本发布。

未设置 `HERALD_CODE_HASH_KEY` 时，v1.8.0 保存和校验验证码的方式与 v1.7.0 完全
相同；滚动升级或回滚期间，v1.7.0 与 v1.8.0 实例可以按任意顺序共用同一个 Redis。

部署前请检查配置：

- 名为 `HERALD_CODE_HASH_KEY` 或 `HERALD_CODE_HASH_VERIFY_ONLY` 的环境变量在
  v1.7.0 中被忽略，现在会生效。`HERALD_CODE_HASH_KEY` 已设置但不足 32 字节，或
  `HERALD_CODE_HASH_VERIFY_ONLY` 不是布尔值时，Herald 在任何环境下都拒绝启动。
- `Configuration initialized` 日志行新增字段 `code_hash`。

启用密钥是所有实例都运行 v1.8.0 之后的单独步骤，请按[验证码哈希密钥](#验证码哈希密钥)
操作：多实例部署先让全部实例带上密钥并设置 `HERALD_CODE_HASH_VERIFY_ONLY=true`，
然后再移除 `HERALD_CODE_HASH_VERIFY_ONLY`。没有密钥、密钥不同或仍运行 v1.7.0 的
实例，会把带密钥 challenge 的正确验证码按 `401/invalid` 拒绝并消耗一次尝试，直到
该 challenge 过期。

回滚到 v1.7.0：

- 从未设置过 `HERALD_CODE_HASH_KEY`：直接替换镜像即可。
- 已设置：先在所有实例上设置 `HERALD_CODE_HASH_VERIFY_ONLY=true`，等待
  `CHALLENGE_EXPIRY`（默认 `5m`），确保没有未过期的带密钥 challenge，然后再降级。
  v1.7.0 会忽略这两个变量。跳过等待只会在最长 `CHALLENGE_EXPIRY` 的时间内出现
  上述拒绝。
- 两个方向都不需要修改或删除 Redis 数据。

本次发布 Go SDK 没有变化。选用 `github.com/soulteary/herald@v1.8.0` 的应用可以
与 v1.7.0 服务配合使用，反之亦然。

## 从 v1.6.0 升级到 v1.7.0

Herald v1.7.0 调整了用户锁定的触发条件、创建 challenge 时限额与冷却的执行方式、若干错误原因码，
以及指标、span 和日志输出。HTTP 路由、成功响应、认证模式、challenge 与幂等记录格式、审计
spool 文件格式以及 Go SDK（`pkg/herald`）均不变，无需转换数据、迁移密钥或重新绑定。
`challenge-kit` 升级到 v1.12.0；`herald-totp/v2` 仍为 v2.0.4，各 provider 服务无需升级。
因新增配置项，本次为次版本更新。

部署前请检查配置。以下取值在 v1.6.0 可以启动，现在在所有环境都会阻止启动：

- `IDEMPOTENCY_UNRESOLVED_TTL`（新增，默认 `24h`）必须为正值，且大于 `IDEMPOTENCY_KEY_TTL`
  （该值为 `0` 时取 `CHALLENGE_EXPIRY`）。`IDEMPOTENCY_KEY_TTL` 为 24h 或更长的部署，或该值
  保持 `0` 且 `CHALLENGE_EXPIRY` 为 24h 或更长的部署，升级前必须把
  `IDEMPOTENCY_UNRESOLVED_TTL` 设为更大的值。
- 新增的锁定配置有各自的取值范围：`LOCKOUT_CHALLENGE_THRESHOLD` 至少为 `1`，
  `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` 不得为负，阈值不为 `0` 时
  `LOCKOUT_FAILED_ATTEMPT_WINDOW` 必须为正值。v1.6.0 没有这些配置，只有环境中已经定义了
  这些变量时才需要关注。

有一种此前会阻止启动的情况现在可以启动：生产环境中 `PROVIDER_FAILURE_POLICY` 未设置或为空时
按 `strict` 处理，而 v1.6.0 在未设置该变量时拒绝启动。生产环境显式设置 `soft` 仍会被拒绝。

升级检查清单。每一项说明变化内容、需要检查什么以及如何处理：

- 用户锁定阈值。单个 challenge 耗尽（`MAX_ATTEMPTS` 次错误验证码）不再锁定其用户，须在
  `LOCKOUT_DURATION` 内耗尽 `LOCKOUT_CHALLENGE_THRESHOLD`（默认 `3`）个 challenge 才会锁定。
  已耗尽的 challenge 本身仍返回 `locked`。设置 `LOCKOUT_CHALLENGE_THRESHOLD=1` 可保持
  v1.6.0 的行为。
- 跨 challenge 的错误次数锁定。用户在 `LOCKOUT_FAILED_ATTEMPT_WINDOW`（默认 `1h`）内提交
  `LOCKOUT_FAILED_ATTEMPT_THRESHOLD`（默认 `15`）次错误验证码后也会被锁定，不论这些验证码
  针对哪个 challenge。v1.6.0 没有这项计数。请确认默认值适合确有频繁重试需求的调用方；设为
  `0` 可关闭计数。因 Redis 故障而无法记录错误次数时，该次验证返回 503。
- 锁定键。锁定键由 `otp:lock:<user_id>` 改为 `otp:lock:v2:<digest>`，并新增
  `otp:lock:exhausted:<digest>`（已耗尽 challenge ID 的集合）和 `otp:lock:failed:<digest>`
  （错误验证码计数器）。摘要依赖 `HERALD_PII_PEPPER`，轮换 pepper 会丢弃当前的锁定和计数。
  旧锁定键不再读取，会在 `LOCKOUT_DURATION` 内过期。升级时正在生效的锁定不会延续。
- 创建限额。创建 challenge 时先检查用户锁定，再在一次 Redis Lua 调用中执行用户、IP 和
  destination 限额以及冷却，该调用读写五个键。Herald 连接单一 Redis 端点；把键路由到不同
  分片的代理必须支持多键脚本。被限额、冷却或 `user_locked` 拒绝的请求不再消耗配额。
  被锁定且同时超出限额的用户现在收到 `403/user_locked`，而不是 `429`；依据 `429`
  安排重试的调用方需要处理 `403`。
- destination 冷却。新增第二个冷却键 `ratelimit:cooldown:dest:<digest>`，按 destination
  应用 `RESEND_COOLDOWN`，不区分 `user_id`。在冷却时间内以多个用户 ID 向同一 destination
  正常发送的调用方，现在会收到 `429/resend_cooldown`。
- 冷却释放。在 `PROVIDER_FAILURE_POLICY=strict` 下，创建请求返回 `500/send_failed`
  且失败能够证明没有任何 provider 接受该消息时（见[没有 provider 的通道](#没有-provider-的通道)），
  重发冷却会被释放，立即重试不再返回 `429/resend_cooldown`；配额仍然计入。为此冷却键的值由
  `1` 改为随机令牌；两个版本都只读取冷却键是否存在及其 TTL。
- Redis 权限。Redis ACL 用户如果按命令限制，需要允许 `EVAL`，以及脚本中使用的 `GET`、`SET`、
  `DEL`、`EXISTS`、`INCR`、`PTTL`、`PEXPIRE`、`SADD` 和 `SCARD`。如果按键模式限制，现有的
  `ratelimit:*` 和 `otp:lock:*` 模式已覆盖新增的键。创建 challenge 不再发送 `PING`。
- 新增和变更的错误响应。按这些响应做判断的客户端和告警需要更新：
  - `challenge_id` 不是 1-64 字节的 `A-Za-z0-9_-` 时，两个验证路由和撤销路由返回
    `400/invalid_challenge_id`。Herald 签发的 ID 始终符合要求。
  - strict 策略下，针对没有 provider 的通道的创建请求返回 `503/channel_not_configured`，
    此时不使用任何配额、冷却或幂等键。请查看 Herald 启动时记录的 provider 覆盖情况日志，
    为缺少 provider 的通道补上配置。
  - 哈希槽位全部占用时，创建和验证返回 `503/hash_capacity_exceeded`。v1.6.0 返回
    `503/backend_unavailable`；把 `backend_unavailable` 视为 Redis 故障的告警不会再收到
    这种情况。
  - `429/rate_limit_exceeded` 新增 `limit` 字段（`user`、`ip` 或 `destination`）。
    `429/resend_cooldown` 没有该字段。
  - 未知的 HMAC 密钥 ID 在 v2/v3 签名方案和旧的 v1 签名方案下都返回
    `401/invalid_signature`，不再返回原因码 `invalid_key_id`。可通过
    `herald_auth_failure_total{reason="invalid_key_id"}` 区分这两种情况。
  - 仅结果记录在操作完成后写入失败时，创建、验证和撤销不再返回 `503/audit_unavailable`，
    应改为对 `herald_audit_post_action_failure_total` 配置告警。
  - 非生产环境中，nonce 无法记录的 HMAC 请求现在与生产环境一样返回
    `503/nonce_store_unavailable`，此前会被放行。
- 不确定的幂等记录。`sending` 记录现在会在 `IDEMPOTENCY_UNRESOLVED_TTL` 后过期。过期前该键
  仍返回 `409/delivery_unknown`；过期后同一个键会发起新的请求。旧版本写入的无 TTL 记录保持
  原样。
- 指标。`herald_redis_operation_duration_seconds` 和 `herald_redis_operations_total` 以
  `operation="ratelimit_create"` 报告创建 challenge 时的限流操作。创建 challenge 不再输出
  `ratelimit_user`、`ratelimit_ip`、`ratelimit_dest` 和 `cooldown` 这些取值，筛选这些取值的
  看板需要更新。新增 `operation="cooldown_release"`，以及计数器
  `herald_audit_post_action_failure_total{operation}`、`herald_audit_quarantined_total`、
  `herald_audit_spool_missing_total`、`herald_auth_failure_total{reason}` 和
  `herald_failed_attempt_lock_total`。
- 追踪。请求 span 的名称由 `/` 改为 `<METHOD> <路由模板>`（例如
  `POST /v1/otp/challenges`）。不再输出 `http.target`、客户端地址（`http.remote_addr`）
  和部分掩码的 `destination` 属性；创建 span 改为携带 `destination_digest`。带有 W3C
  `traceparent` 请求头的请求现在会加入调用方的 trace。使用旧名称或旧属性的追踪查询和看板
  需要更新。
- OTLP 端点。`OTLP_ENDPOINT` 除 `host:port` 外，也接受 `http://` 和 `https://` URL。
  v1.6.0 对 URL 形式不导出任何数据，因此这样配置的部署在升级后会开始导出 span，请确认
  collector 可达且容量足够。协议始终取自 `OTLP_ENDPOINT`：此前通过
  `OTEL_EXPORTER_OTLP_INSECURE`、`OTEL_EXPORTER_OTLP_TRACES_INSECURE` 或 `http://` 形式的
  `OTEL_EXPORTER_OTLP_ENDPOINT`，让 `https://` 端点以明文 HTTP 连接 collector 的部署，必须把
  `OTLP_ENDPOINT` 改为 `http://` 或 `host:port`。只有在导出器建立成功时才会输出
  `OpenTelemetry tracing initialized`。
- 启动日志。新增的警告带有 `check` 字段：`config.environment`（未设置 `ENVIRONMENT`；
  后续版本将在未设置时拒绝启动）、`config.rate_limit_ip`（启用按 IP 限额但未配置
  `HERALD_TRUSTED_PROXY_HEADER` 和 `HERALD_TRUSTED_PROXIES`）、`config.listener_tls`
  （生产环境监听端口未启用 TLS）、`config.code_length`（`CODE_LENGTH` 小于 6）、
  `config.test_mode`、`config.totp`、`config.otlp`、`config.otlp_endpoint` 和
  `config.secrets`。这些警告都不会阻止启动。对启动阶段任何警告都触发的日志告警需要调整。
- 发送失败日志。内置 SMTP provider 的失败日志带有 `failure_category`，服务器返回状态码时还带有
  `smtp_code`；strict 策略下的发送失败日志带有 `cooldown_released`。
- 审计 spool。连续 5 次无法解码的待投递文件会被重命名为 `<id>.json.quarantined`，不再重试；
  无法读取的文件按最长 5 分钟的间隔重试；从 `AUDIT_SPOOL_DIR` 中被删除的待投递文件会被丢弃，
  并记录一次日志。应对 `herald_audit_quarantined_total` 和
  `herald_audit_spool_missing_total` 配置告警。并发的审计记录共用 spool 的 sync 操作；
  每条记录仍然只在其文件和目录项都已 sync 之后才确认。文件格式不变。
- 模板。带地区标记的目录（如 `zh-CN`）中的自定义模板现在会被选用，v1.6.0 对这类目录发送的是
  内置文案。以 CRLF 换行保存的邮件模板现在可以发送，v1.6.0 中由这类模板生成的邮件全部发送
  失败。升级前请检查 `TEMPLATE_DIR` 下的文件，因为其内容将开始对外发送。主题行仍含回车符或
  NUL 的模板不会被加载，并在启动时报告；无法加载或永远不会被选用的模板文件同样会在启动时
  报告。
- 非生产环境的密钥。固定的回退密钥已移除。`HERALD_IDEMPOTENCY_SECRET`、`HMAC_SECRET` 和
  `API_KEY` 均未设置的部署（仅非生产环境可能出现）改为从 `HERALD_HMAC_KEYS` 派生密钥；没有
  密钥映射时，每次启动生成随机密钥。以旧密钥为键的幂等和 nonce 记录将无法再找到；完全没有
  密钥材料时，这些记录既不在实例之间共享，也不会跨重启保留。设置
  `HERALD_IDEMPOTENCY_SECRET` 可避免这一情况。已设置密钥的部署和生产环境不受影响。

滚动升级。两个版本的副本可以同时对外提供服务，副本之间没有先后顺序要求。请先完成上述配置
调整，再替换全部副本，并尽量缩短新旧版本并存的时间：

- 两个版本共用：challenge、限流计数器、既有的用户+destination 冷却、幂等记录和 nonce 记录。
  计数器和冷却的键名、窗口和 TTL 均未改变。v1.7.0 写入的冷却键保存随机令牌，v1.6.0 写入的
  保存 `1`；两个版本都认可这两种值，v1.7.0 只会释放仍保存着自身令牌的冷却。
- 不共用：用户锁定。v1.6.0 实例读写 `otp:lock:<user_id>`，v1.7.0 实例读写
  `otp:lock:v2:<digest>`，因此经一个版本锁定的用户仍可通过另一个版本创建和验证 challenge。
  由 v1.6.0 实例处理的 challenge 耗尽和错误验证码不计入 v1.7.0 的阈值。
- v1.6.0 实例不读取 `ratelimit:cooldown:dest:<digest>`，仅按 destination 的冷却要到所有
  实例升级后才完全生效。
- 两个版本并存期间，客户端收到哪一套错误原因码、Redis 指标带有哪一套 `operation` 取值，
  取决于处理请求的实例。

回滚到 v1.6.0 会保留全部 Redis 数据和审计 spool，无需删除任何内容：

- 在生产环境启动 v1.6.0 之前，须显式设置 `PROVIDER_FAILURE_POLICY=strict`；v1.6.0 在未设置
  该变量时拒绝启动。v1.6.0 忽略新增的 `LOCKOUT_*` 配置和 `IDEMPOTENCY_UNRESOLVED_TTL`。
  `OTLP_ENDPOINT` 写成 URL 时 v1.6.0 不导出 span；连接明文 HTTP 的 collector 请使用
  `host:port`。
- v1.6.0 不读取 `otp:lock:v2:<digest>`、`otp:lock:exhausted:<digest>`、
  `otp:lock:failed:<digest>` 和 `ratelimit:cooldown:dest:<digest>`。这些键会自行过期
  （分别取决于 `LOCKOUT_DURATION`、`LOCKOUT_FAILED_ATTEMPT_WINDOW` 和 `RESEND_COOLDOWN`）。
  因此 v1.7.0 设置的锁定在回滚后不再生效，v1.6.0 重新在第一个 challenge 耗尽时锁定用户，
  并写入 `otp:lock:<user_id>`。
- 保存令牌的冷却键在过期前会被 v1.6.0 视为生效中的冷却。限流计数器和待处理的 challenge
  继续有效。
- v1.7.0 写入的 `sending` 幂等记录保留其 TTL，仍会过期；v1.6.0 写入的这类记录重新变为没有
  TTL。
- v1.7.0 写入的待投递审计 spool 文件由 v1.6.0 继续投递。`<id>.json.quarantined` 文件会被
  v1.6.0 忽略并留在 `AUDIT_SPOOL_DIR` 中；对无法解码或已被删除的待投递文件，v1.6.0 重新
  每秒重试一次。
- 错误原因码、指标标签、span 名称与属性以及模板选用规则恢复为 v1.6.0 的行为。非生产环境且
  未配置密钥时，v1.6.0 重新使用固定的回退密钥，找不到 v1.7.0 写入的相关记录。

本次发布未改动 Go SDK。选择 `github.com/soulteary/herald@v1.7.0` 的应用可以调用 v1.6.0
的服务，反之亦然，只是错误原因码以上文所列为准。上文更早的迁移要求仍然适用。
