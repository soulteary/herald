# 安全文档

> 🌐 **Language / 语言**: [English](../enUS/SECURITY.md) | [中文](SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](../koKR/SECURITY.md)

本文档说明 Herald 的安全特性、安全配置和最佳实践。

## 已实现的安全功能

1. **基于 Challenge 的验证**: 使用 challenge-verify 模型防止重放攻击，确保验证码一次性使用
2. **安全代码存储**: 验证码使用 Argon2 哈希存储，从不以明文形式存储
3. **多维度限流**: 按 user_id、destination（邮箱/手机）和 IP 地址进行限流，防止滥用
4. **服务认证**: 支持 mTLS、HMAC 签名和 API Key 认证，用于服务间通信
5. **幂等性保护**: 使用幂等键防止重复创建 challenge 和重复发送验证码
6. **Challenge 过期**: Challenge 自动过期，可配置 TTL
7. **尝试次数限制**: 每个 challenge 的最大尝试次数限制，防止暴力破解
8. **重发冷却**: 防止快速重发验证码
9. **审计日志**: 所有操作的完整审计跟踪，包括发送、验证和失败
10. **Provider 安全**: 与邮件和短信提供商的安全通信

## 安全最佳实践

### 1. 生产环境配置

**必须配置项**:
- 设置 `ENVIRONMENT=production`；生产配置校验会对不安全组合执行 fail-closed
- 选择 `REQUEST_AUTH_MODE=hmac_v2`（推荐）或 `api_key`，并配置该模式对应的凭据
- 保持 `HERALD_TEST_MODE=false`；生产环境会拒绝测试模式
- 设置 `PROVIDER_FAILURE_POLICY=strict`，Provider URL 必须使用 HTTPS
- 使用 `REDIS_PASSWORD` 保护 Redis（或通过 `HERALD_RISK_ACK_PASSWORDLESS_REDIS=true` 显式接受风险）
- 使用强且唯一的 HMAC 密钥，并通过明确的 Key ID 完成轮换

**配置示例**:
```bash
export ENVIRONMENT=production
export REQUEST_AUTH_MODE=hmac_v2
# 在安全的管理主机上仅生成一次以下三个值，将其保存到密钥管理系统，并向
# 所有副本注入相同值。不要在应用入口脚本或每次重启时重新生成：
#   openssl rand -hex 32  # HERALD_HMAC_KEY_1
#   openssl rand -hex 32  # HERALD_HMAC_KEY_2
#   openssl rand -hex 32  # HERALD_IDEMPOTENCY_KEY
: "${HERALD_HMAC_KEY_1:?请从密钥管理系统加载 HERALD_HMAC_KEY_1}"
: "${HERALD_HMAC_KEY_2:?请从密钥管理系统加载 HERALD_HMAC_KEY_2}"
: "${HERALD_IDEMPOTENCY_KEY:?请从密钥管理系统加载 HERALD_IDEMPOTENCY_KEY}"
export HERALD_HMAC_KEYS="{\"key-id-1\":\"${HERALD_HMAC_KEY_1}\",\"key-id-2\":\"${HERALD_HMAC_KEY_2}\"}"
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HERALD_IDEMPOTENCY_SECRET="${HERALD_IDEMPOTENCY_KEY}"
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
unset HERALD_HMAC_KEY_1 HERALD_HMAC_KEY_2 HERALD_IDEMPOTENCY_KEY
export HERALD_TEST_MODE=false
export PROVIDER_FAILURE_POLICY=strict
export REDIS_PASSWORD="your-redis-password"
export REDIS_ADDR="redis:6379"
```

### 2. 敏感信息管理

**推荐做法**:
- ✅ 使用环境变量存储 API 密钥和密钥
- ✅ 需要时使用 `REDIS_PASSWORD` 或密钥管理服务存储 Redis 密码
- ✅ 使用密钥管理服务（如 HashiCorp Vault）管理生产环境密钥
- ✅ 确保配置文件权限设置正确（如 `chmod 600`）
- ✅ 永远不要记录验证码或敏感用户数据

**不推荐做法**:
- ❌ 在配置文件中硬编码密钥
- ❌ 通过命令行参数传递密钥（会出现在进程列表中）
- ❌ 将包含敏感信息的配置文件提交到版本控制
- ❌ 在生产环境中记录验证码或 challenge 详细信息

### 3. 网络安全

**必须配置**:
- 生产环境必须使用 HTTPS
- 配置防火墙规则限制对 Herald 服务的访问
- 使用 mTLS 进行服务间通信（最高安全性）
- 定期更新依赖项以修复已知漏洞

**推荐配置**:
- 使用反向代理（如 Nginx 或 Traefik）处理 SSL/TLS
- 若按 IP 限流，确保反向代理正确转发客户端 IP（如 X-Forwarded-For）
- 使用强 HMAC 密钥（最少 32 个字符）
- 在生产环境中禁用不安全的认证方法（使用 mTLS 或 HMAC，尽可能避免 API Key）

### 4. 限流配置

Herald 实现多维度限流以防止滥用：

**限流维度**:
- **按用户 ID**: 限制每个 user_id 在时间窗口内的 challenge 数量
- **按目标地址**: 限制每个邮箱/手机号在时间窗口内的 challenge 数量
- **按 IP 地址**: 限制每个客户端 IP 在时间窗口内的 challenge 数量

**配置示例**:
```bash
# 限流配置（每小时请求数）
export RATE_LIMIT_PER_USER=10         # 按 user_id，每小时
export RATE_LIMIT_PER_DESTINATION=10  # 按邮箱/手机，每小时
export RATE_LIMIT_PER_IP=5            # 按 IP，每分钟
```

### 5. Challenge 安全

**Challenge 配置**:
- **TTL**: Challenge 过期时间（默认：300 秒）
- **最大尝试次数**: 每个 challenge 的最大验证尝试次数（默认：5 次）
- **重发冷却**: 两次重发之间的最小时间（默认：60 秒）

**配置示例**:
```bash
export CHALLENGE_EXPIRY=5m
export MAX_ATTEMPTS=5
export RESEND_COOLDOWN=60s
```

## API 安全

### 相互独立的安全层

Herald 分别配置传输层认证和请求层认证：

1. **传输层（可选 mTLS）**
   - `CLIENT_CERT_MODE=off|optional|require`
   - 当所有调用方都持有可信客户端证书时，推荐使用 `require`
   - mTLS 不替代请求正文完整性校验

2. **请求层**
   - `REQUEST_AUTH_MODE=hmac_v2`（推荐）：可防重放的 HMAC-SHA256
   - `REQUEST_AUTH_MODE=api_key`：通过 `X-API-Key` 或 `Authorization: Bearer ...` 传递 API Key
   - `REQUEST_AUTH_MODE=none`：仅限开发；生产环境会拒绝启动

### HMAC v3 认证

创建验证码时即使没有幂等键也必须使用 v3，防止删除请求头后退回旧签名。
修改、添加或删除幂等键都会使签名失效，且不会消费 nonce。旧 HMAC 调用方需要升级；
其他验证接口保留 v2 兼容性。完整签名格式和升级规则见 [API 认证](API.md#hmac-v3)。

HMAC v3 对请求形态、幂等键摘要和正文摘要进行签名：

```text
HERALD-HMAC-V3
METHOD
PATH
QUERY
TIMESTAMP
NONCE
SERVICE
KEY_ID
SHA256_HEX(IDEMPOTENCY_KEY_OR_EMPTY)
SHA256_HEX(RAW_BODY)
```

**必需请求头**:
- `X-Signature-Version: v3`
- `X-Signature`：规范串的 HMAC-SHA256 小写十六进制值
- `X-Timestamp`：Unix 秒级时间戳
- `X-Nonce`：本次请求的唯一值
- `X-Service`：调用方服务标识
- `X-Key-Id`：Key ID；仅当配置 `HERALD_HMAC_DEFAULT_KEY_ID` 时可省略

省略 `X-Key-Id` 时，签名方必须将规范串中的 `KEY_ID` 行保持为空。`HERALD_HMAC_DEFAULT_KEY_ID` 只用于服务端选择验签密钥，不会填充被签名的规范字段。

**服务端配置**:
```bash
export REQUEST_AUTH_MODE=hmac_v2
: "${HERALD_HMAC_KEYS:?load the key map with production-strength secrets}"
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
```

Herald 先校验签名，再在 Redis 中原子消费 Nonce；重复使用的 Nonce 会被拒绝。默认允许前后各 60 秒的时间偏差。Nonce 至少保留允许偏差的两倍再加 30 秒（默认 150 秒），覆盖包括未来时间戳请求在内的完整有效窗口。旧版 v1 默认关闭，只应在迁移期通过 `HMAC_V1_ENABLED=true` 临时启用。

### mTLS 认证

要求可信客户端证书时：

```bash
export TLS_CERT_FILE=/path/to/herald.crt
export TLS_KEY_FILE=/path/to/herald.key
export TLS_CA_CERT_FILE=/path/to/client-ca.crt
export CLIENT_CERT_MODE=require
```

`TLS_CLIENT_CA_FILE` 可作为 `TLS_CA_CERT_FILE` 的别名。容器内启用 TLS 时，还需通过相应的 `HERALD_HEALTHCHECK_TLS_*` 变量配置回环健康检查。

## 数据安全

### 验证码存储

- **从不以明文存储**: 所有验证码都使用 Argon2 进行哈希处理
- **一次性使用**: Challenge 在成功验证后立即删除
- **自动过期**: Challenge 在 TTL 后过期，防止使用过期的验证码

### Redis 安全

- Redis 应配置密码保护
- 需要时使用 `REDIS_PASSWORD` 环境变量
- 限制 Redis 的网络访问（仅允许 Herald 服务访问）
- 使用 Redis AUTH 进行认证
- 定期更新 Redis 以修复已知漏洞
- Herald 未提供原生 Redis TLS 配置；需要 Redis 传输加密时，使用受保护的私有网络或经过认证的加密隧道

### 审计日志

Herald 维护以下操作的完整审计日志：
- Challenge 创建和发送
- 验证尝试（成功和失败）
- 限流命中
- Provider 通信（成功和失败）
- 认证失败

**审计日志字段**:
- 时间戳
- 操作类型
- 用户 ID
- 目标地址（脱敏）
- 通道（email/sms）
- 结果（成功/失败）
- 原因（失败时）
- 客户端 IP
- Provider 信息

## Provider 安全

### 邮件 Provider 安全

- 使用安全的 SMTP 连接（TLS/SSL）
- 使用安全凭据与 provider 进行认证
- 安全存储 provider 凭据（环境变量或密钥管理）
- 监控 provider 通信失败情况

### 短信 Provider 安全

- 使用 HTTPS 进行 API 通信
- 使用安全 API 密钥与 provider 进行认证
- 安全存储 provider 凭据
- 监控 provider 通信失败情况

## 限流和滥用防护

### 限流策略

Herald 实现多层限流：

1. **按用户限流**: 防止单个用户创建过多 challenge
2. **按目标地址限流**: 防止滥用特定邮箱/手机号
3. **按 IP 限流**: 防止单个 IP 地址的滥用
4. **重发冷却**: 防止快速重发验证码到同一 challenge
5. **尝试次数限制**: 限制每个 challenge 的验证尝试次数

### 配置

```bash
# 限流配置
export RATE_LIMIT_PER_USER=10         # 每小时
export RATE_LIMIT_PER_DESTINATION=10  # 每小时
export RATE_LIMIT_PER_IP=5            # 每分钟

# Challenge 设置
export CHALLENGE_EXPIRY=5m
export MAX_ATTEMPTS=5
export RESEND_COOLDOWN=60s
```

## 错误处理

### 生产模式

在生产环境（`ENVIRONMENT=production`，且 `HERALD_TEST_MODE=true` 会被拒绝）下：

- 隐藏详细错误信息，防止信息泄露
- 返回通用错误消息
- 详细错误信息仅记录在日志中
- 审计日志包含完整详细信息用于安全分析

### 开发模式

在开发模式下：

- API 仍返回通用错误，开发环境不会启用明文渠道错误诊断
- 验证码暴露要求明确配置测试环境和测试模式

## 安全响应头

Herald 默认未安装浏览器安全响应头中间件。按需在 HTTPS 反向代理上配置
`X-Content-Type-Options`、`Content-Security-Policy` 和 HSTS，不能假定 Herald 会自动发送这些头。

## 漏洞报告

如果发现安全漏洞，请通过以下方式报告：

1. **GitHub Security Advisory**（推荐）
   - 访问仓库的 [Security 标签页](https://github.com/soulteary/herald/security)
   - 点击 "Report a vulnerability"
   - 填写安全咨询表单

2. **邮件**（如果 GitHub Security Advisory 不可用）
   - 发送邮件给项目维护者
   - 包含漏洞的详细描述

**请不要通过公开的 GitHub Issues 报告安全漏洞。**

## 服务间鉴权

与其他服务（如 Stargate）集成时，使用安全认证：

### 推荐：mTLS

使用双向 TLS 证书获得最高安全级别。

### 替代方案：HMAC 签名

使用带时间戳验证的 HMAC-SHA256 签名进行安全服务间通信。

### 不推荐：API Key

API Key 认证适用于开发环境，但不推荐用于生产环境服务间通信。

## 发送渠道失败诊断

发送渠道的错误消息可能回显验证码正文、目标地址或凭据。Herald 的发送失败日志、span 状态与
exception 事件、审计失败记录只保留已知原因码。未知原因码归为 `send_failed`，截止时间超时
仍归为 `timeout`。日志使用方应读取 `reason` 字段，不再依赖原先的自由文本 `error` 字段。

本次只调整诊断数据；客户端响应、严格策略撤销和幂等 `delivery_unknown` 处理保持不变，
无需迁移 Redis 记录或更新客户端。既有日志和已导出的 span 不会被改写；若渠道曾回显敏感数据，
应核查历史诊断数据的访问范围和保留期限，并轮换其中已暴露的凭据。

## 相关文档

- [API 文档](API.md) - 了解 API 安全特性和认证
- [部署文档](DEPLOYMENT.md) - 了解生产环境部署建议
- [监控文档](MONITORING.md) - 了解安全监控和告警
- [故障排查文档](TROUBLESHOOTING.md) - 了解安全相关的故障排查

测试专用例外：`ENVIRONMENT=test` 且 `HERALD_TEST_MODE=true` 时，测试缓存保存明文验证码，
可通过受保护的独立测试监听器和 `debug_code` 读取；生产环境拒绝此模式。

## 哈希资源限制

用户、IP 和目标限流不能限制不同身份与目标的总并发 Argon2 工作。共享 OTP 准入预算同时覆盖
此前没有全局工作上限的创建路径，避免已认证且符合业务限额的并发流量耗尽内存。
它不替代业务限流或进程资源限制，配置见 [部署说明](DEPLOYMENT.md#otp-哈希工作准入)。

## 已验证 HMAC 身份

省略 Key ID 和显式默认 ID 不是不同凭据，必须共享幂等保护。Herald 在 HMAC 验签及 nonce
处理通过后才传递实际身份，不改写已签名原始字段。无法归属凭据的旧记录会阻止新创建，不暴露
旧响应。本次修复防止成功记录写入失败后的重复发送，以及默认密钥轮换后重放上一凭据的响应，
不涉及 HMAC 签名伪造，升级见 [部署说明](DEPLOYMENT.md#hmac-身份升级)。
## 单密钥 HMAC 身份

仅配置 `HMAC_SECRET` 时，此前未配置的 Key ID 也会选中该密钥，却建立独立 nonce 和幂等
命名空间。在渠道发送成功、Redis 完成写入失败后，换 ID 的有效签名重试可能重复发送。
此风险需要掌握共享密钥，不属于签名伪造。

单密钥查找现在只允许省略 ID 或精确填写 `default`，其他 ID 在消费 nonce 和进入 handler
前拒绝。显式 key map 保留配置中的身份，HMAC v2/v3 格式不变。旧记录保持原样；直接将旧
自定义 ID 改为 `default` 不能解决不确定发送，需遵循
[部署迁移与回滚要求](DEPLOYMENT.md#单密钥-key-id-白名单)。

## CORS 通配项

生产部署不能在 `HERALD_CORS_ALLOW_ORIGINS` 中混用独立 `*` 和其他来源。此前校验接受
这类列表，中间件向未列出的来源返回 `Access-Control-Allow-Origin: *`。此风险要求存在
该配置，默认仍关闭 CORS；它不会绕过 API Key 或 HMAC，无签名业务请求仍返回 401。
现在启动和路由构建都会拒绝生产列表任意位置的独立通配项，详见
[部署要求](DEPLOYMENT.md#生产-cors-来源列表)。
