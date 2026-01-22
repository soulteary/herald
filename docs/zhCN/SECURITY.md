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
- 必须设置 `API_KEY` 环境变量用于 API Key 认证
- 配置 `HERALD_HMAC_KEYS` 用于 HMAC 签名认证（生产环境推荐）
- 设置 `MODE=production` 启用生产模式
- 使用 `REDIS_PASSWORD` 或 `REDIS_PASSWORD_FILE` 配置 Redis 密码保护
- 为 HMAC 密钥使用强且唯一的密钥

**配置示例**:
```bash
export API_KEY="your-strong-api-key-here"
export HERALD_HMAC_KEYS='{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}'
export MODE=production
export REDIS_PASSWORD="your-redis-password"
export REDIS_ADDR="redis:6379"
```

### 2. 敏感信息管理

**推荐做法**:
- ✅ 使用环境变量存储 API 密钥和密钥
- ✅ 使用密码文件（`REDIS_PASSWORD_FILE`）存储 Redis 密码
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
- 配置 `TRUSTED_PROXY_IPS` 以正确获取客户端真实 IP
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
export HERALD_RATE_LIMIT_USER=10      # 按 user_id
export HERALD_RATE_LIMIT_DESTINATION=5 # 按邮箱/手机
export HERALD_RATE_LIMIT_IP=20        # 按 IP 地址
```

### 5. Challenge 安全

**Challenge 配置**:
- **TTL**: Challenge 过期时间（默认：300 秒）
- **最大尝试次数**: 每个 challenge 的最大验证尝试次数（默认：5 次）
- **重发冷却**: 两次重发之间的最小时间（默认：60 秒）

**配置示例**:
```bash
export HERALD_CHALLENGE_TTL_SECONDS=300
export HERALD_MAX_ATTEMPTS=5
export HERALD_RESEND_COOLDOWN_SECONDS=60
```

## API 安全

### 认证方式

Herald 支持三种认证方式，优先级如下：

1. **mTLS**（最安全 - 生产环境推荐）
   - 双向 TLS 客户端证书验证
   - 最高安全级别
   - 需要 TLS 证书配置

2. **HMAC 签名**（安全 - 生产环境推荐）
   - HMAC-SHA256 签名验证
   - 基于时间戳的重放保护（5 分钟窗口）
   - 服务标识符支持多租户场景

3. **API Key**（简单 - 仅开发环境）
   - 通过 `X-API-Key` 请求头进行基本 API 密钥认证
   - 适用于开发和测试
   - 不推荐用于生产环境服务间通信

### HMAC 签名认证

**签名算法**:
```
signature = HMAC-SHA256(timestamp:service:body, secret)
```

**请求头**:
- `X-Signature`: HMAC 签名值
- `X-Timestamp`: Unix 时间戳（秒）
- `X-Service`: 服务标识符（可选）

**配置**:
```bash
export HERALD_HMAC_KEYS='{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}'
export HERALD_HMAC_TIMESTAMP_TOLERANCE=300  # 5 分钟（秒）
```

**安全注意事项**:
- 时间戳必须在容差窗口内（默认：5 分钟）以防止重放攻击
- 为每个服务使用强且唯一的密钥
- 定期轮换 HMAC 密钥

### mTLS 认证

为了获得最高安全性，使用双向 TLS：

**配置**:
```bash
export HERALD_TLS_CERT=/path/to/herald.crt
export HERALD_TLS_KEY=/path/to/herald.key
export HERALD_TLS_CA=/path/to/ca.crt
export HERALD_TLS_REQUIRE_CLIENT_CERT=true
```

## 数据安全

### 验证码存储

- **从不以明文存储**: 所有验证码都使用 Argon2 进行哈希处理
- **一次性使用**: Challenge 在成功验证后立即删除
- **自动过期**: Challenge 在 TTL 后过期，防止使用过期的验证码

### Redis 安全

- Redis 应配置密码保护
- 使用 `REDIS_PASSWORD` 或 `REDIS_PASSWORD_FILE` 环境变量
- 限制 Redis 的网络访问（仅允许 Herald 服务访问）
- 使用 Redis AUTH 进行认证
- 定期更新 Redis 以修复已知漏洞
- 生产环境考虑使用 Redis over TLS

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
# 限流配置（每小时）
export HERALD_RATE_LIMIT_USER=10
export HERALD_RATE_LIMIT_DESTINATION=5
export HERALD_RATE_LIMIT_IP=20

# Challenge 设置
export HERALD_CHALLENGE_TTL_SECONDS=300
export HERALD_MAX_ATTEMPTS=5
export HERALD_RESEND_COOLDOWN_SECONDS=60
```

## 错误处理

### 生产模式

在生产模式下（`MODE=production` 或 `MODE=prod`）：

- 隐藏详细错误信息，防止信息泄露
- 返回通用错误消息
- 详细错误信息仅记录在日志中
- 审计日志包含完整详细信息用于安全分析

### 开发模式

在开发模式下：

- 显示详细错误信息以便调试
- 包含堆栈跟踪信息
- 更详细的日志记录

## 安全响应头

Herald 自动添加以下安全相关的 HTTP 响应头：

- `X-Content-Type-Options: nosniff` - 防止 MIME 类型嗅探
- `X-Frame-Options: DENY` - 防止点击劫持
- `X-XSS-Protection: 1; mode=block` - XSS 保护

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

## 相关文档

- [API 文档](API.md) - 了解 API 安全特性和认证
- [部署文档](DEPLOYMENT.md) - 了解生产环境部署建议
- [监控文档](MONITORING.md) - 了解安全监控和告警
- [故障排查文档](TROUBLESHOOTING.md) - 了解安全相关的故障排查
