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
- `result`: 操作结果（`success` 或 `failed`）

**示例：**
```
herald_otp_challenges_total{channel="email",purpose="login",result="success"} 1234
herald_otp_challenges_total{channel="sms",purpose="login",result="failed"} 5
```

### 发送指标

#### `herald_otp_sends_total`

计数器，跟踪通过提供者发送的 OTP 总数。

**标签：**
- `channel`: 通道类型（`sms`、`email` 或 `dingtalk`）
- `provider`: 提供者名称（例如：`smtp`、`aliyun`、`placeholder`）
- `result`: 发送操作结果（`success` 或 `failed`）

**示例：**
```
herald_otp_sends_total{channel="email",provider="smtp",result="success"} 1200
herald_otp_sends_total{channel="sms",provider="placeholder",result="failed"} 10
```

#### `herald_otp_send_duration_seconds`

直方图，跟踪 OTP 发送操作的持续时间。

**标签：**
- `provider`: 提供者名称

**桶：** 默认 Prometheus 桶（0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10）

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
- `result`: 验证结果（`success` 或 `failed`）
- `reason`: 失败原因（例如：`expired`、`invalid`、`locked`、`too_many_attempts`）

**示例：**
```
herald_otp_verifications_total{result="success",reason=""} 1100
herald_otp_verifications_total{result="failed",reason="expired"} 50
herald_otp_verifications_total{result="failed",reason="invalid"} 30
herald_otp_verifications_total{result="failed",reason="locked"} 5
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

#### `herald_redis_latency_seconds`

直方图，跟踪 Redis 操作延迟。

**标签：**
- `operation`: Redis 操作类型（`get`、`set`、`del`、`exists`）

**桶：** [0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]

**示例：**
```
herald_redis_latency_seconds_bucket{operation="get",le="0.01"} 5000
herald_redis_latency_seconds_sum{operation="get"} 12.5
herald_redis_latency_seconds_count{operation="get"} 10000
```

## Prometheus 配置

### 基本抓取配置

将以下内容添加到 `prometheus.yml`：

```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    scrape_interval: 15s
    scrape_timeout: 10s
```

### 服务发现

对于 Kubernetes 部署，使用服务发现：

```yaml
scrape_configs:
  - job_name: 'herald'
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

2. **挑战成功率**
   ```
   rate(herald_otp_challenges_total{result="success"}[5m]) / rate(herald_otp_challenges_total[5m])
   ```

3. **验证成功率**
   ```
   rate(herald_otp_verifications_total{result="success"}[5m]) / rate(herald_otp_verifications_total[5m])
   ```

4. **按提供者的发送成功率**
   ```
   rate(herald_otp_sends_total{result="success"}[5m]) / rate(herald_otp_sends_total[5m])
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
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

### 错误指标

1. **速率限制命中率**
   ```
   rate(herald_rate_limit_hits_total[5m])
   ```

2. **验证失败原因**
   ```
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   ```

3. **发送失败率**
   ```
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
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
          rate(herald_otp_verifications_total{result="failed"}[5m]) / 
          rate(herald_otp_verifications_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Herald 验证失败率较高"
          description: "验证失败率为 {{ $value | humanizePercentage }}"

      # 高发送失败率
      - alert: HeraldHighSendFailureRate
        expr: |
          rate(herald_otp_sends_total{result="failed"}[5m]) / 
          rate(herald_otp_sends_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Herald 发送失败率较高"
          description: "发送失败率为 {{ $value | humanizePercentage }}"

      # 高 Redis 延迟
      - alert: HeraldHighRedisLatency
        expr: |
          histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m])) > 0.1
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

**注意**：用于分布式追踪的 OpenTelemetry 支持已计划但尚未实现。这包括：
- `traceparent` / `tracestate` 请求头传播
- 核心 span：`otp.challenge.create`、`otp.provider.send`、`otp.verify`
- Span 标签：`channel`、`purpose`、`provider`、`result`、`reason`

请参阅项目路线图了解实施时间表。

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
