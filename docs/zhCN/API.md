# Herald API 文档

Herald 是一个验证码和 OTP 服务，处理通过 SMS、电子邮件和 DingTalk 发送验证码（邮件可通过内置 SMTP 或设置 `HERALD_SMTP_API_URL` 时通过 [herald-smtp](https://github.com/soulteary/herald-smtp)，DingTalk 通过 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)），具有内置的速率限制和安全控制。

## 基础 URL

```
http://localhost:8082
```

## 认证

请求认证与客户端证书策略彼此独立：

- `REQUEST_AUTH_MODE=hmac_v2`（默认）选择 HMAC 认证（保留原配置名称）；创建验证码必须使用 v3。
- `REQUEST_AUTH_MODE=api_key` 要求 `X-API-Key` 或 `Authorization: Bearer …`。
- `REQUEST_AUTH_MODE=none` 仅供开发使用，生产环境会拒绝启动。
- `CLIENT_CERT_MODE=off|optional|require` 控制 TLS 客户端证书。客户端证书校验通过也不会绕过所选的请求认证模式。

### HMAC v3

发送 `X-Signature-Version: v3`、`X-Signature`、`X-Timestamp`、`X-Nonce`、`X-Service`；多密钥且未配置默认密钥时还必须发送 `X-Key-Id`。

`X-Service` 和 `X-Key-Id` 不能包含保留文本 `|kid:`。所有请求认证模式（包括旧版 v1
回退）都会在认证、nonce 使用和业务写入前返回 `401/invalid_auth_headers`。
该文本用于分隔幂等命名空间中的服务和密钥字段。普通命名空间和已有 Redis 记录保持
不变。如果现有集成使用了该文本，需调整服务标识或配置中的 key ID；改变命名空间或
使用新幂等键重试前，应先核对尚未确认的发送结果。

签名的规范字符串以换行符分隔：

```text
HERALD-HMAC-V3
METHOD
/path
raw=query
unix_timestamp
single_use_nonce
service_name
key_id
sha256_hex_of_idempotency_key
sha256_hex_of_raw_body
```

幂等键行是 `Idempotency-Key` 请求头原始值的 SHA-256 小写十六进制摘要。
请求头缺失或为空时，对空字节串计算摘要。正文摘要后没有结尾换行。
v3 同时绑定幂等键的值与缺失状态；增加、删除或修改该请求头都会导致签名失败，且不消费 nonce。

HMAC 模式下，`POST /v1/otp/challenges` 无论是否带幂等键都要求 v3。
v1/v2 返回 `401/signature_version_required`，即使设置了 `HMAC_V1_ENABLED=true`。
如果允许不带键的 v2，中间层仍可通过删除请求头绕过保护。其他接口保留 v2 兼容性。
API key 模式和仅供开发使用的无认证模式不变。

Go SDK 默认使用 v3。需协调服务端和调用方升级：旧服务端不能验证 v3，
新服务端的创建接口不再接受旧 HMAC 签名。升级时不要更换幂等键或身份命名空间。
已有 Redis 记录、缓存响应、TTL 和未确认发送记录不变；使用相同幂等键及新的时间戳和 nonce 重试，
对未确认的发送先核对 provider 结果。

`key_id` 行必须使用 `X-Key-Id` 请求头的原始值。如果因已配置或隐式默认密钥而省略该请求头，签名时此行必须为空，不能填入默认密钥 ID。对上述内容计算小写十六进制 HMAC-SHA256。默认时间偏差窗口为 60 秒（`HMAC_MAX_DRIFT`），每个有效 nonce 只能在 Redis 中消费一次。使用多个 `HERALD_HMAC_KEYS` 时，请设置 `HERALD_HMAC_DEFAULT_KEY_ID` 或发送 `X-Key-Id`；Herald 不会使用 Go map 中的“第一个”密钥。旧版 v1 仅在 `HMAC_V1_ENABLED=true` 时启用。

### API Key

使用 `REQUEST_AUTH_MODE=api_key` 时，发送 `X-API-Key: <key>` 或 `Authorization: Bearer <key>`。

## 端点

### 健康检查

**GET /healthz**

检查服务健康状态。此端点还会验证 Redis 连接性。

**响应（成功）：**
```json
{
  "status": "ok",
  "service": "herald"
}
```

**响应（失败 - Redis 不可用）：**
```json
{
  "status": "unhealthy",
  "error": "Redis connection failed"
}
```

**注意**：实际响应格式使用 `status` 和 `service` 字段，这与规范中的 `{ "ok": true }` 格式不同。这是当前的实现，为保持向后兼容性而保留。

### 创建挑战

**POST /v1/otp/challenges**

创建新的验证挑战并发送验证码。


provider 发送完成后，Herald 通过一次 Redis Lua 操作删除被替换的旧验证码，并更新 active 索引。
重发成功后，不会再因为单独撤销旧码的请求失败，或 Herald 在两步之间退出，而导致新旧码均可兑换。
如果新验证码记录已不存在，激活失败且保留当前旧码及其索引。

激活错误仍会使创建请求失败。使用幂等键时，sending 记录保持不自动过期；使用相同键重试返回
`delivery_unknown`，不会再次发送。激活操作的响应丢失时，新旧码可能都已失效；更换幂等键重试前，
应先核对发送结果。

验证码记录键名和有效期保持不变；激活索引按下文“激活索引升级”迁移，无需新增配置。本次修复不会扫描、删除
升级前撤销失败所遗留的未知旧码；这些记录仍按原有效期过期。核对既往故障时，应撤销已知受影响的
challenge ID。升级时继续保留 Redis 持久化数据和审计持久卷。


**请求头：**
- `Idempotency-Key`（可选）：重试时使用相同键和相同请求。已完成请求在 `IDEMPOTENCY_KEY_TTL` 内重放响应；发送前的并发请求返回 `409/idempotency_in_flight`。开始发送后，尚未完成的请求返回 `409/delivery_unknown`，即使超过 TTL 也不会再次发送。相同键对应不同请求时返回 `409/idempotency_conflict`。

指纹保留字段边界，即使 JSON 字符串包含 NUL 字符（`\u0000`）也不会混淆不同请求。
普通请求的指纹和 Redis key 与旧版本兼容；含 NUL 的请求使用带版本和长度信息的指纹。
如果相同键下已有旧格式记录，重试返回 `409/idempotency_conflict`，不会修改或删除记录。
有过期时间的记录仍按原 TTL 保留；无过期时间的发送中记录需先核对渠道投递结果，不能
为了重试而直接删除记录或更换幂等键。

Herald 在调用发送渠道前保存不自动过期的发送记录。进程退出、渠道结果不确定或后续 Redis
写入失败时，该记录会阻止重试再次发送。最终成功记录写入失败返回 `503/backend_unavailable`。
使用相同键重试：若结果已经提交则重放，否则返回 `delivery_unknown`。收到该响应时不要自动
更换幂等键重发，应先向渠道核对投递情况。在真正调用渠道前退出，也可能留下尚未发送的
不确定记录。

这些保障要求请求携带幂等键且 Redis 保留记录，并不构成渠道与 Redis 之间的分布式事务。
正常成功响应仍按配置的 TTL 过期，过期后再次使用该键视为新请求。持久化与核对方式见部署指南。

**请求：**
```json
{
  "user_id": "u_123",
  "channel": "sms",
  "destination": "+8613800138000",
  "purpose": "login",
  "locale": "zh-CN",
  "client_ip": "192.168.1.1",
  "ua": "Mozilla/5.0..."
}
```

**通道：** `channel` 可为 `"sms"`、`"email"` 或 `"dingtalk"`。当为 `"email"` 且配置了 `HERALD_SMTP_API_URL` 时，Herald 将发送请求转发给 [herald-smtp](https://github.com/soulteary/herald-smtp)，`destination` 为邮箱地址。当为 `"dingtalk"` 时，Herald 将发送请求转发给 [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)（需配置 `HERALD_DINGTALK_API_URL`）；`destination` 为钉钉 userid（或在 herald-dingtalk 启用手机号查询时为 11 位手机号）。Herald 不保存任何 SMTP 或钉钉凭证。

**响应：**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

当 `HERALD_TEST_MODE=true` 时，响应中还会包含 `debug_code`（明文验证码），便于调用方（如 Stargate 调试模式）在本地/测试时直接展示。**生产环境请勿开启测试模式。**

**错误响应：**

所有错误响应都遵循以下格式：
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "可选的错误消息"
}
```

可能的错误代码：
- `invalid_request`：请求体解析失败
- `user_id_required`：缺少必需字段 `user_id`
- `invalid_channel`：无效的通道类型（必须是 "sms"、"email" 或 "dingtalk"）
- `invalid_purpose`：无效的 purpose 值（必须是允许的 purpose 之一）
- `destination_required`：缺少必需字段 `destination`
- `rate_limit_exceeded`：超过速率限制
- `resend_cooldown`：重发冷却期未过期
- `user_locked`：用户暂时被锁定
- `send_failed`：通过提供者发送验证码失败
- `internal_error`：内部服务器错误

HTTP 状态代码：
- `400 Bad Request`：无效的请求参数
- `401 Unauthorized`：认证失败
- `403 Forbidden`：用户被锁定
- `429 Too Many Requests`：超过速率限制
- `500 Internal Server Error`：内部服务器错误

### 获取测试验证码（仅测试模式）

**GET /v1/test/code/:challenge_id**

返回指定 challenge 的验证码。**仅当 `HERALD_TEST_MODE=true` 时可用。** 用于集成测试或 Stargate 调试模式下在创建 challenge 响应未包含 `debug_code` 时作为回退获取验证码。生产环境请勿开启测试模式。

**响应（200 OK）：**
```json
{
  "ok": true,
  "challenge_id": "ch_7f9b...",
  "code": "123456"
}
```

**错误：** 测试模式关闭或 challenge 不存在/未存储验证码时返回 `404 Not Found`。

### 验证挑战

**POST /v1/otp/verifications**

验证挑战代码。

**请求：**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**响应（成功）：**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**响应（失败）：**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**错误响应：**

可能的错误代码：
- `invalid_request`：请求体解析失败
- `challenge_id_required`：缺少必需字段 `challenge_id`
- `code_required`：缺少必需字段 `code`
- `invalid_code_format`：验证码格式无效
- `expired`：挑战已过期
- `invalid`：无效的验证码
- `locked`：由于尝试次数过多，挑战被锁定
- `verification_failed`：一般验证失败
- `internal_error`：内部服务器错误

HTTP 状态代码：
- `400 Bad Request`：无效的请求参数
- `401 Unauthorized`：验证失败
- `403 Forbidden`：用户被锁定
- `500 Internal Server Error`：内部服务器错误

### 带上下文验证挑战（v2）

**POST /v2/otp/verifications**

以下三个上下文字段均为必填，且必须是非空白字符串：

```json
{
  "challenge_id": "ch_example",
  "code": "123456",
  "expected_user_id": "u_123",
  "expected_purpose": "login",
  "expected_channel": "email"
}
```

预期用户、用途和渠道必须来自业务后端的可信状态，不能直接转发终端用户传入的值。
请求签名只能认证调用服务，不能代替该服务对用户和操作的业务授权。

缺少字段、传入 null、空串或纯空白时，在验证验证码之前返回 `400/context_required`。
上下文与 challenge 不匹配时返回 `409/context_mismatch`，且不消费验证码。
成功响应包含 `ok`、`user_id`、`purpose`、`channel`、`challenge_id`、`amr`、
`verified_at` 和 `issued_at`；成功验证后 challenge 不能再次使用。

兼容性：v2 不再接受无绑定或部分绑定的请求，现有 v2 调用方需要补齐三个字段。
v1 验证接口保留原有行为，不要求传入预期上下文字段。

### 撤销挑战

**POST /v1/otp/challenges/{id}/revoke**

撤销挑战（可选）。

**响应（成功）：**
```json
{
  "ok": true
}
```

**响应（失败）：**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**错误响应：**

可能的错误代码：
- `challenge_id_required`：URL 参数中缺少挑战 ID
- `internal_error`：内部服务器错误

HTTP 状态代码：
- `400 Bad Request`：无效的请求
- `500 Internal Server Error`：内部服务器错误

### TOTP 代理（可选）

当 `HERALD_TOTP_ENABLED=true` 且设置了 `HERALD_TOTP_BASE_URL` 时，Herald 会将 TOTP（Authenticator）请求代理到 [herald-totp](https://github.com/soulteary/herald-totp)。所有 TOTP 路由与 OTP 路由使用相同认证（mTLS、HMAC 或 API Key）。

#### 获取 TOTP 状态

**GET /v1/totp/status**

查询主体是否已绑定 TOTP。

**查询参数：**
- `subject`（必填）：用户标识（如 user_id）

**响应（成功）：** 由 herald-totp 代理返回（如 `{"enrolled": true}` 或 `{"enrolled": false}`）。

**错误响应：**
- `400 Bad Request`：缺少 `subject`（`invalid_request`，`subject required`）
- `503 Service Unavailable`：未配置 TOTP（`totp_not_configured`）
- `502 Bad Gateway`：代理到 herald-totp 失败（`proxy_failed`）

#### 验证 TOTP

**POST /v1/totp/verify**

验证 TOTP 码。

**请求：**
```json
{
  "subject": "u_123",
  "code": "123456"
}
```

**响应：** 由 herald-totp 代理返回（成功如 `{"ok": true}`，失败如 `{"ok": false, "reason": "..."}`）。代理错误时 Herald 返回 `502` 及 `proxy_failed`。

#### 开始 TOTP 绑定

**POST /v1/totp/enroll/start**

开始 TOTP 绑定；返回用于 Authenticator 应用的 secret/QR。

**请求：**
```json
{
  "subject": "u_123"
}
```

**响应：** 由 herald-totp 代理返回（如 `enroll_id`、QR/secret）。代理错误时 Herald 返回 `502` 及 `proxy_failed`。

#### 确认 TOTP 绑定

**POST /v1/totp/enroll/confirm**

使用应用中的验证码确认 TOTP 绑定。

**请求：**
```json
{
  "enroll_id": "enroll_xxx",
  "code": "123456"
}
```

**响应：** 由 herald-totp 代理返回。失败时 Herald 可能返回 `400` 及 `invalid` 或代理错误。

#### 撤销 TOTP

**POST /v1/totp/revoke**

撤销主体的 TOTP。

**请求：**
```json
{
  "subject": "u_123"
}
```

**响应：** 由 herald-totp 代理返回。代理错误时 Herald 返回 `502` 及 `proxy_failed`。

**TOTP 错误码（Herald 代理返回）：**
- `totp_not_configured`：未启用 TOTP 或未设置 herald-totp URL
- `proxy_failed`：请求 herald-totp 失败
- `invalid_request`：缺少或无效的请求体/参数

## 速率限制

Herald 实现多维速率限制：

- **按用户**：每小时 10 个请求（可配置）
- **按 IP**：每分钟 5 个请求（可配置）
- **按目标**：每小时 10 个请求（可配置）
- **重发冷却**：重发之间间隔 60 秒

## 错误代码

本节列出了 API 可能返回的所有错误代码。

### 请求验证错误
- `invalid_request`：请求体解析失败或无效的 JSON
- `user_id_required`：缺少必需字段 `user_id`
- `invalid_channel`：无效的通道类型（必须是 "sms"、"email" 或 "dingtalk"）
- `invalid_purpose`：无效的 purpose 值（必须是允许的 purpose 之一）
- `destination_required`：缺少必需字段 `destination`
- `challenge_id_required`：缺少必需字段 `challenge_id`
- `code_required`：缺少必需字段 `code`
- `invalid_code_format`：验证码格式无效

### 认证错误
HMAC/API Key 失败会返回 JSON `reason`：
- `signature_version_required`：创建接口要求 `X-Signature-Version: v3`；其他 HMAC 接口接受 v2/v3
- `missing_auth_headers`：缺少一个或多个 HMAC 请求头
- `key_id_required` / `invalid_key_id`：未提供可用的 HMAC 密钥 ID
- `timestamp_out_of_range`：时间戳超出配置的偏差窗口
- `invalid_signature`：HMAC 签名验证失败
- `replayed_nonce`：nonce 已被消费
- `nonce_store_unavailable`：故障关闭模式下 Redis nonce 存储失败
- `unauthorized`：认证失败

### 挑战错误
- `expired`：挑战已过期
- `invalid`：无效的验证码
- `locked`：由于尝试次数过多，挑战被锁定
- `too_many_attempts`：失败尝试次数过多（可能包含在 `locked` 中）
- `verification_failed`：一般验证失败
- `send_failed`：通过提供者发送验证码失败（仅在挑战创建时）

### 速率限制错误
- `rate_limit_exceeded`：超过速率限制
- `resend_cooldown`：重发冷却期未过期

### 用户状态错误
- `user_locked`：用户暂时被锁定

### 系统错误
- `internal_error`：内部服务器错误

## 审计接收失败

启用审计后，必要事件必须先保存到本地持久化缓冲。缓冲已满或不可写时返回
`503`，响应为 `{"ok":false,"reason":"audit_unavailable"}`。后端投递故障不会在缓冲仍有
容量时拒绝请求。验证和撤销的尝试事件在操作前记录，结果事件在操作后记录。
这些写入不是分布式事务，结果记录失败时业务副作用可能已经发生；503 不代表验证码
仍可使用或消息尚未发送。发送重试应遵循上述幂等规则。

### 激活索引升级

激活索引按用户 ID、用途、渠道和目标地址的原值生成，使用带版本和明确字段边界的编码。
`Alice` 与 `alice` 不再共享索引，避免其他字段相同时，一个身份申请验证码会撤销另一个身份的验证码。
该问题不允许跨用户验证验证码。

旧验证码记录在原有效期内仍可验证。重发时，只有旧索引所指记录的四个身份字段完全相同，
才会原子迁移并撤销旧验证码；发生身份冲突的旧记录会被保留。
此前被覆盖的旧索引无法自动重建，失去索引的验证码会正常过期，已知受影响的 challenge ID 可以主动撤销。

升级时先暂停创建流量，替换全部旧实例后再恢复。混合版本使用不同索引，可能使被替换的验证码继续有效。
回滚时应暂停创建，等待最长验证码有效期后再恢复旧版本。不需要修改客户端 API 或验证码记录格式。
