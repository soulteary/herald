# Herald 故障排查指南

本指南帮助您诊断和解决 Herald OTP 和验证码服务的常见问题。

## 目录

- [收不到验证码](#收不到验证码)
- [验证码错误](#验证码错误)
- [401 未授权错误](#401-未授权错误)
- [速率限制问题](#速率限制问题)
- [Redis 连接问题](#redis-连接问题)
- [提供者发送失败](#提供者发送失败)
- [性能问题](#性能问题)

## 收不到验证码

### 症状
- 用户报告未收到通过 SMS、电子邮件或 DingTalk 发送的验证码
- 挑战创建成功但未发送验证码

### 诊断步骤

1. **检查提供者连接**
   ```bash
   # 检查 Herald 日志中的提供者错误
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **验证提供者配置**
   - 检查 SMTP 设置（用于电子邮件）：`SMTP_HOST`、`SMTP_PORT`、`SMTP_USER`、`SMTP_PASSWORD`
   - 检查 SMS 提供者设置：`SMS_PROVIDER`、`ALIYUN_ACCESS_KEY` 等
   - DingTalk 通道：确认已设置 `HERALD_DINGTALK_API_URL` 且 herald-dingtalk 可访问；若 herald-dingtalk 启用 API Key 认证，需设置 `HERALD_DINGTALK_API_KEY`
   - 验证提供者凭据是否正确

3. **检查 Prometheus 指标**
   ```promql
   # 检查发送失败率
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # 按提供者检查发送成功率
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **检查审计日志**
   - 查看 `send_failed` 事件的审计日志
   - 检查提供者错误代码和消息
   - 验证目标地址是否有效

5. **直接测试提供者**
   - 手动测试 SMTP 连接
   - 直接测试 SMS 提供者 API
   - 验证到提供者端点的网络连接

### 解决方案

- **提供者配置问题**：更新提供者凭据和设置
- **网络问题**：检查防火墙规则和网络连接
- **提供者速率限制**：检查提供者是否有被超过的速率限制
- **无效目标**：验证电子邮件地址和电话号码是否有效

## 验证码错误

### 症状
- 用户报告"无效代码"错误
- 即使使用正确代码，验证也失败
- 挑战显示已过期或已锁定

### 诊断步骤

1. **检查挑战状态**
   ```bash
   # 检查 Redis 中的挑战数据
   redis-cli GET "otp:ch:{challenge_id}"
   ```

2. **验证挑战过期**
   - 检查 `CHALLENGE_EXPIRY` 配置（默认：5 分钟）
   - 验证系统时间是否同步（NTP）
   - 检查挑战是否已过期

3. **检查尝试次数**
   - 验证 `MAX_ATTEMPTS` 配置（默认：5）
   - 检查挑战是否因尝试次数过多而被锁定
   - 查看审计日志中的尝试历史

4. **检查 Prometheus 指标**
   ```promql
   # 检查验证失败原因
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # 检查锁定的挑战
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
   ```

5. **查看审计日志**
   - 检查验证尝试和结果
   - 验证代码格式是否匹配预期格式
   - 检查是否存在时间问题

### 解决方案

- **过期挑战**：用户需要请求新的验证码
- **尝试次数过多**：挑战已锁定，等待锁定持续时间或请求新挑战
- **代码格式不匹配**：验证代码长度是否匹配 `CODE_LENGTH` 配置
- **时间同步**：确保系统时钟已同步

## 401 未授权错误

### 症状
- API 请求返回 401 未授权
- 日志中的身份验证失败
- 服务间通信失败

### 诊断步骤

1. **检查身份验证方法**
   - 验证正在使用的身份验证方法（mTLS、HMAC、API 密钥）
   - 检查是否存在身份验证请求头

2. **对于 HMAC 身份验证**
   ```bash
   # 验证 HMAC_SECRET 已配置
   echo $HMAC_SECRET
   
   # 检查时间戳是否在 5 分钟窗口内
   # 验证签名计算是否与 Herald 的实现匹配
   ```

3. **对于 API 密钥身份验证**
   ```bash
   # 验证 API_KEY 已设置
   echo $API_KEY
   
   # 检查 X-API-Key 请求头是否匹配配置的密钥
   ```

4. **对于 mTLS 身份验证**
   - 验证客户端证书是否有效
   - 检查证书链是否受信任
   - 验证 TLS 连接是否已建立

5. **检查 Herald 日志**
   ```bash
   # 检查身份验证错误
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### 解决方案

- **缺少凭据**：设置 `API_KEY` 或 `HMAC_SECRET` 环境变量
- **无效签名**：验证 HMAC 签名计算是否与 Herald 的实现匹配
- **时间戳过期**：确保客户端和服务器时钟已同步（在 5 分钟内）
- **证书问题**：验证 mTLS 证书是否有效且受信任

## 速率限制问题

### 症状
- 用户报告"超过速率限制"错误
- 合法请求被阻止
- 高速率限制命中指标

### 诊断步骤

1. **检查速率限制配置**
   ```bash
   # 验证速率限制设置
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **检查 Prometheus 指标**
   ```promql
   # 按范围检查速率限制命中
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # 检查速率限制命中率
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **查看 Redis 键**
   ```bash
   # 检查速率限制键
   redis-cli KEYS "otp:rate:*"
   
   # 检查特定速率限制键
   redis-cli GET "otp:rate:user:{user_id}"
   ```

4. **分析使用模式**
   - 检查合法用户是否达到限制
   - 识别潜在的滥用或机器人活动
   - 查看挑战创建模式

### 解决方案

- **调整速率限制**：如果合法用户受到影响，增加限制
- **用户教育**：告知用户速率限制和冷却期
- **防止滥用**：为疑似滥用实施额外的安全措施
- **重发冷却**：用户必须等待 `RESEND_COOLDOWN` 期间（默认：60 秒）

## Redis 连接问题

### 症状
- 服务无法启动
- 健康检查返回不健康
- 挑战创建/验证失败
- 高 Redis 延迟指标

### 诊断步骤

1. **检查 Redis 连接性**
   ```bash
   # 测试 Redis 连接
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **验证配置**
   ```bash
   # 检查 Redis 配置
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
   echo $REDIS_DB
   ```

3. **检查 Redis 健康状态**
   ```bash
   # 检查 Redis 信息
   redis-cli INFO
   
   # 检查内存使用情况
   redis-cli INFO memory
   ```

4. **检查 Prometheus 指标**
   ```promql
   # 检查 Redis 延迟
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # 检查 Redis 操作错误
   rate(herald_redis_latency_seconds_count[5m])
   ```

5. **查看 Herald 日志**
   ```bash
   # 检查 Redis 错误
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### 解决方案

- **连接问题**：验证 `REDIS_ADDR` 是否正确且 Redis 可访问
- **身份验证问题**：检查 `REDIS_PASSWORD` 是否正确
- **数据库选择**：验证 `REDIS_DB` 是否正确（默认：0）
- **性能问题**：检查 Redis 内存使用情况并考虑扩展
- **网络问题**：验证网络连接和防火墙规则

## 提供者发送失败

### 症状
- 指标中高发送失败率
- 日志中的 `send_failed` 错误
- 提供者特定的错误消息

### 诊断步骤

1. **检查提供者指标**
   ```promql
   # 按提供者检查发送失败率
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # 按提供者检查发送持续时间
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **查看提供者日志**
   - 检查提供者特定的错误消息
   - 查看提供者错误的审计日志
   - 检查提供者 API 响应代码

3. **测试提供者连接**
   - 手动测试 SMTP 连接
   - 直接测试 SMS 提供者 API
   - 验证提供者凭据

4. **检查提供者状态**
   - 检查提供者状态页面
   - 验证提供者 API 是否正常运行
   - 检查提供者速率限制

### 解决方案

- **提供者中断**：等待提供者恢复服务
- **凭据问题**：更新提供者凭据
- **速率限制**：检查是否超过提供者速率限制
- **配置问题**：验证提供者配置是否正确
- **网络问题**：检查到提供者的网络连接

## 性能问题

### 症状
- 响应时间慢
- 高延迟指标
- 超时错误
- 高资源使用

### 诊断步骤

1. **检查响应时间**
   ```promql
   # 检查发送持续时间
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # 检查 Redis 延迟
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **检查资源使用**
   ```bash
   # 检查 CPU 和内存
   top -p $(pgrep herald)
   
   # 检查 Goroutine 数量
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
   ```

3. **查看请求模式**
   - 检查请求速率
   - 识别峰值使用时间
   - 检查流量峰值

4. **检查 Redis 性能**
   ```bash
   # 检查 Redis 慢日志
   redis-cli SLOWLOG GET 10
   
   # 检查 Redis 内存
   redis-cli INFO memory
   ```

### 解决方案

- **扩展资源**：增加 CPU/内存分配
- **优化 Redis**：使用 Redis Cluster 或优化查询
- **提供者优化**：使用更快的提供者或优化提供者调用
- **缓存**：为频繁访问的数据实现缓存
- **负载均衡**：在多个实例之间分配负载

## 获取帮助

如果您无法解决问题：

1. **检查日志**：查看 Herald 日志以获取详细错误消息
2. **检查指标**：查看 Prometheus 指标以查找模式
3. **查看文档**：检查 [API.md](API.md) 和 [DEPLOYMENT.md](DEPLOYMENT.md)
4. **提交问题**：创建问题时包含：
   - 错误消息和日志
   - 配置（不含密钥）
   - 重现步骤
   - 预期与实际行为

## 相关文档

- [API 文档](API.md) - API 端点详情
- [部署指南](DEPLOYMENT.md) - 部署和配置
- [监控指南](MONITORING.md) - 监控和指标
