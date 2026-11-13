# Herald 监控指南

Herald 暴露 Prometheus 指标用于监控和可观测性。本文档描述了所有可用指标、如何配置监控以及最佳实践。

## 指标端点

Herald 在 `/metrics` 端点暴露 Prometheus 指标：

```
GET http://localhost:8082/metrics
```

## 可用指标

### 挑战指标

#### `herald_otp_challenges_total`

计数器，跟踪创建的 OTP 挑战总数。

**标签：**
- `channel`: 通道类型（`sms`、`email` 或 `dingtalk`）
- `purpose`: 挑战目的（例如：`login`、`reset`、`bind`）
- `result`: 当前在 challenge 写入后、发送前记录 `success`，不能用它计算创建 API 的成功率。

**示例：**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
```

### 发送指标

#### `herald_otp_sends_total`

计数器，跟踪通过提供者发送的 OTP 总数。

**标签：**
- `channel`: 通道类型（`sms`、`email` 或 `dingtalk`）
- `provider`: 提供者名称（例如：`smtp`、`aliyun`、`placeholder`）
- `result`: 发送操作结果（`success` 或 `failure`）

**示例：**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failure"} 10
```

#### `herald_otp_send_duration_seconds`

直方图，跟踪 OTP 发送操作的持续时间。

**标签：**
- `provider`: 提供者名称

**桶：** 外部 API 桶（0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30）

**示例：**
```
herald_otp_send_duration_seconds_bucket{provider="smtp",le="0.1"} 800
herald_otp_send_duration_seconds_sum{provider="smtp"} 45.2
herald_otp_send_duration_seconds_count{provider="smtp"} 1200
```

### 验证指标

#### `herald_otp_verifications_total`

计数器，跟踪 OTP 验证总数。

**标签：**
- `result`: 验证结果（`success` 或 `failure`）
- `reason`: 失败原因（例如：`expired`、`invalid`、`locked`）

**示例：**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failure",reason="expired"} 50
herald_otp_verifications_total{result="failure",reason="invalid"} 30
herald_otp_verifications_total{result="failure",reason="locked"} 5
```

### 速率限制指标

#### `herald_rate_limit_hits_total`

计数器，跟踪速率限制命中总数。

**标签：**
- `scope`: 速率限制范围（`user`、`ip`、`destination`、`resend_cooldown`）

**示例：**
```
herald_rate_limit_hits_total{scope="user"} 25
herald_rate_limit_hits_total{scope="ip"} 100
herald_rate_limit_hits_total{scope="destination"} 15
herald_rate_limit_hits_total{scope="resend_cooldown"} 200
```

### Redis 指标

#### `herald_redis_operation_duration_seconds`

直方图，跟踪 Redis 操作延迟。

**标签：**
- `operation`: 已埋点的包装操作，并非全部 Redis 命令。创建 challenge 时各项限流和冷却在一次调用中执行，记为 `ratelimit_create`。各限流器的包装操作（`ratelimit`、`ratelimit_user`、`ratelimit_ip`、`ratelimit_dest`、`cooldown`）不再用于创建 challenge

**桶：** [0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0]

**示例：**
```
herald_redis_operation_duration_seconds_bucket{operation="ratelimit_user",le="0.01"} 5000
herald_redis_operation_duration_seconds_sum{operation="ratelimit_user"} 12.5
herald_redis_operation_duration_seconds_count{operation="ratelimit_user"} 10000
```

## Prometheus 配置

### 基本抓取配置

先配置独立的 `HERALD_METRICS_API_KEY`，并将其值挂载到下方抓取端凭据文件。生产环境拒绝匿名抓取，不得复用业务 API/HMAC 密钥。

将以下内容添加到 `prometheus.yml`：

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
    scrape_interval: 15s
    scrape_timeout: 10s
```

### 服务发现

对于 Kubernetes 部署，使用服务发现：

```yaml
scrape_configs:
  - job_name: 'herald'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - default
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: herald
        action: keep
      - source_labels: [__meta_kubernetes_pod_ip]
        target_label: __address__
        replacement: '${1}:8082'
```

## 关键监控指标

### 业务指标

1. **挑战创建速率**
   ```
   rate(herald_otp_challenges_total[5m])
   ```

2. **按渠道与用途统计已创建挑战的速率**
   ```
   sum by (channel, purpose) (rate(herald_otp_challenges_total{result="success"}[5m]))
   ```

3. **验证成功率**
   ```
   sum(rate(herald_otp_verifications_total{result="success"}[5m])) / sum(rate(herald_otp_verifications_total[5m]))
   ```

4. **按提供者的发送成功率**
   ```
   sum by (provider) (rate(herald_otp_sends_total{result="success"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m]))
   ```

### 性能指标

1. **平均发送持续时间**
   ```
   rate(herald_otp_send_duration_seconds_sum[5m]) / rate(herald_otp_send_duration_seconds_count[5m])
   ```

2. **P95 发送持续时间**
   ```
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   ```

3. **Redis P99 延迟**
   ```
   histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m]))
   ```

### 错误指标

1. **速率限制命中率**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **验证失败原因**
   ```
   sum by (reason) (rate(herald_otp_verifications_total{result="failure"}[5m]))
   ```

3. **发送失败率**
   ```
   sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m]))
   ```

## 告警规则

### 示例告警规则

```yaml
groups:
  - name: herald
    interval: 30s
    rules:
      # 高失败率
      - alert: HeraldHighVerificationFailureRate
        expr: |
          sum(rate(herald_otp_verifications_total{result="failure"}[5m])) / sum(rate(herald_otp_verifications_total[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald 验证失败率较高"
          description: "验证失败率为 {{ $value | humanizePercentage }}"

      # 高发送失败率
      - alert: HeraldHighSendFailureRate
        expr: |
          sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m])) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald 发送失败率较高"
          description: "发送失败率为 {{ $value | humanizePercentage }}"

      # 高 Redis 延迟
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald Redis 延迟较高"
          description: "P99 Redis 延迟为 {{ $value }}s"

      # 高速率限制命中
      - alert: HeraldHighRateLimitHits
        expr: |
          rate(herald_rate_limit_hits_total[5m]) > 10
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Herald 速率限制命中较高"
          description: "速率限制命中率为 {{ $value }} 次/秒"
```

## Grafana 仪表板

### 推荐仪表板面板

1. **概览面板**
   - 挑战创建速率
   - 验证成功率
   - 发送成功率
   - 当前活动挑战数（如果跟踪）

2. **性能面板**
   - 发送持续时间（P50、P95、P99）
   - Redis 延迟（P50、P95、P99）
   - 请求速率

3. **错误面板**
   - 验证失败原因分解
   - 按提供者的发送失败率
   - 按范围的速率限制命中

4. **提供者面板**
   - 按提供者的发送成功率
   - 按提供者的发送持续时间
   - 按提供者的发送量

## OpenTelemetry 支持

当设置 `OTLP_ENABLED=true` 且配置 `OTLP_ENDPOINT` 时，Herald 会启用 OpenTelemetry 追踪中间件。追踪上下文通过 `traceparent` 与 `tracestate` 请求头传播。核心 span 包括 challenge 创建、provider 发送与验码；span 标签包括 `channel`、`purpose`、`provider`、`result`、`reason`。span 上以 `user_id_digest` 标识用户，这是以 `HERALD_PII_PEPPER` 为密钥的加 pepper 摘要；原始 `user_id` 不会被导出。同一 pepper 下摘要保持稳定，仍可用于关联同一用户的 span。配置方式见 [DEPLOYMENT.md](DEPLOYMENT.md) 中的 `OTLP_ENDPOINT`（如 OTLP HTTP 使用 `http://localhost:4318`）。

## 最佳实践

1. **监控关键业务指标**：跟踪挑战创建、验证成功率和发送成功率
2. **设置告警**：为高失败率和性能下降配置告警
3. **跟踪提供者性能**：监控按提供者的发送持续时间和成功率
4. **监控 Redis 健康状态**：跟踪 Redis 延迟和连接问题
5. **速率限制监控**：监控速率限制命中以了解使用模式和潜在的滥用

## 相关文档

- [API 文档](API.md) - API 端点详情
- [部署指南](DEPLOYMENT.md) - 部署和配置
- [故障排查指南](TROUBLESHOOTING.md) - 常见问题和解决方案

## 审计投递

`herald_audit_write_failure_total` 表示后端投递或清理失败，未投递记录仍留在本地缓冲中重试。
请同时监控 `AUDIT_SPOOL_DIR` 中的 `*.json` 文件和剩余磁盘空间，持续积压会达到
`AUDIT_SPOOL_MAX_BYTES` 并拒绝新的审计事件。

`herald_audit_dropped_total` 现在统计持久化接收前被拒绝的事件，对应接口返回
`503/audit_unavailable`，不再统计可恢复的后端投递失败。存储恢复后应确认缓冲逐步清空，
不要通过删除待投递文件来消除告警。

应对 `herald_audit_post_action_failure_total{operation}`（`create`、`verify`、`revoke`）配置
告警：消息已发送、验证码已消费或撤销已完成后，结果记录未能被接收。请求已正常响应，因此
审计记录中缺少该事件。每次发生还会使 `herald_audit_dropped_total` 加一，并写入一条 error
日志，包含 `stage="post_action"`、事件类型、challenge ID、结果、原因和 `user_id_digest`；
可据此日志补录该记录。

## 发送渠道失败诊断

发送失败使用受控 `reason` 字段，不再记录上游错误消息；span 状态和 exception 事件也只保留安全原因码。
排障时不要启用原始渠道响应体日志。若历史日志或导出的 span 含敏感数据，应核查访问与保留策略，
并轮换已暴露凭据，字段变化见 [安全说明](SECURITY.md#发送渠道失败诊断)。

## 认证与幂等结果

- `herald_idempotency_total{result}` 使用 `replay`、`conflict`、`in_flight`、`delivery_unknown`、`ownership_lost` 或 `backend_error`。指纹冲突和旧命名空间冲突均计入 `conflict`，需按 API reason 区分，不能增加身份标签。
- `herald_nonce_replay_total` 统计已拒绝的 nonce 重放。
- `herald_provider_timeout_total{channel}` 统计渠道超时。
- `herald_verification_contention_total{result}` 在验证路径埋点处记录 `contended` 或 `acquired`。
- `herald_challenge_state_total{state}` 记录实际埋点的 `activated`、`superseded`、`revoked` 等状态。
- `herald_redis_operations_total{operation,result}` 的结果为 `success` 或 `failure`；延迟直方图 count 包含两种结果，不是错误计数。

没有独立哈希预算占用指标。结合 `503/backend_unavailable`、重试头、运行时内存和请求结果
观察过载，不得将原始服务/密钥 ID、目标地址或客户端键加入指标标签。

### 指标认证

生产环境中，`/metrics` 要求配置独立的 `HERALD_METRICS_API_KEY`，长度至少 32 字节。未配置
指标密钥时，该端点返回 401。业务 API Key 不能用于抓取，指标密钥也不能用于访问 OTP 或
TOTP。应为两者分别生成相互独立的随机凭据。启动时会拒绝将指标密钥复用为 API Key 或任一
已配置的 HMAC 密钥，开发和测试环境也是如此。

开发和测试环境仅在未设置指标密钥时允许匿名抓取。配置密钥后，所有环境的抓取都受保护。
可接受的请求头为 `Authorization: Bearer <key>` 和 `X-Metrics-API-Key: <key>`；不接受查询
参数。

升级生产环境前，应同时配置密钥并更新抓取端：

```yaml
scrape_configs:
  - job_name: herald
    static_configs:
      - targets: ['herald:8082']
    metrics_path: /metrics
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
```

即使启用了认证，也应将该端点置于私有网络，或在反向代理处限制访问。存活和就绪探针不变。
