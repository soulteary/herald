# Herald API 文档

Herald 是一个验证码和 OTP 服务，处理通过 SMS 和电子邮件发送验证码，具有内置的速率限制和安全控制。

## 基础 URL

```
http://localhost:8082
```

## 认证

Herald 支持两种认证方法：

1. **API 密钥**（简单）：设置 `X-API-Key` 请求头
2. **HMAC 签名**（安全）：设置 `X-Signature`、`X-Timestamp` 和 `X-Service` 请求头

### HMAC 签名

HMAC 签名的计算方式为：
```
HMAC-SHA256(timestamp:service:body, secret)
```

其中：
- `timestamp`：Unix 时间戳（秒）
- `service`：服务标识符（例如，"stargate"）
- `body`：请求体（JSON 字符串）
- `secret`：HMAC 密钥

## 端点

### 健康检查

**GET /health**

检查服务健康状态。

**响应：**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### 创建挑战

**POST /v1/otp/challenges**

创建新的验证挑战并发送验证码。

**请求头（可选）：**
- `Idempotency-Key`：幂等键，同一键返回相同 challenge 结果

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

**响应：**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

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
- `invalid_channel`：无效的通道类型（必须是 "sms" 或 "email"）
- `destination_required`：缺少必需字段 `destination`
- `idempotency_conflict`：幂等键已存在但请求内容不一致
- `idempotency_in_progress`：幂等键正在处理中，请稍后重试
- `rate_limit_exceeded`：超过速率限制
- `resend_cooldown`：重发冷却期未过期
- `user_locked`：用户暂时被锁定
- `send_failed`：外部发送失败（严格模式）
- `internal_error`：内部服务器错误

HTTP 状态代码：
- `400 Bad Request`：无效的请求参数
- `401 Unauthorized`：认证失败
- `403 Forbidden`：用户被锁定
- `409 Conflict`：幂等键冲突或正在处理中
- `429 Too Many Requests`：超过速率限制
- `502 Bad Gateway`：外部发送失败（严格模式）
- `500 Internal Server Error`：内部服务器错误

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
- `invalid_channel`：无效的通道类型（必须是 "sms" 或 "email"）
- `destination_required`：缺少必需字段 `destination`
- `challenge_id_required`：缺少必需字段 `challenge_id`
- `code_required`：缺少必需字段 `code`
- `invalid_code_format`：验证码格式无效
- `idempotency_conflict`：幂等键冲突，请求内容不一致
- `idempotency_in_progress`：幂等键处理中

### 认证错误
- `authentication_required`：未提供有效的认证
- `invalid_timestamp`：无效的时间戳格式
- `timestamp_expired`：时间戳超出允许的窗口（5 分钟）
- `invalid_signature`：HMAC 签名验证失败

### 挑战错误
- `expired`：挑战已过期
- `invalid`：无效的验证码
- `locked`：由于尝试次数过多，挑战被锁定
- `too_many_attempts`：失败尝试次数过多（可能包含在 `locked` 中）
- `verification_failed`：一般验证失败

### 速率限制错误
- `rate_limit_exceeded`：超过速率限制
- `resend_cooldown`：重发冷却期未过期

### 用户状态错误
- `user_locked`：用户暂时被锁定

### 系统错误
- `send_failed`：外部发送失败（严格模式）
- `internal_error`：内部服务器错误
