# Herald OpenTelemetry Tracing

Herald 的 OpenTelemetry 分布式追踪支持。

## 功能

- ✅ 透传 `traceparent`/`tracestate` 请求头
- ✅ 创建核心 span（`otp.challenge.create`、`otp.provider.send`、`otp.verify`）
- ✅ 添加标签和属性（channel、purpose、user_id、provider 等）
- ✅ 与外部服务 span 关联
- ✅ 错误记录和状态设置

## 配置

在环境变量中设置：

```bash
# 启用 OpenTelemetry
OTLP_ENABLED=true

# OTLP/HTTP 端点：host:port，或 http(s)://host:port[/path]
OTLP_ENDPOINT=http://localhost:4318
```

## 核心 Span

### `otp.challenge.create`
- 创建验证码 challenge 的 span
- 属性：`channel`、`purpose`、`user_id_digest`、`destination_digest`（均为加盐摘要，不导出原始值）

### `otp.provider.send`
- 发送验证码到 provider 的 span
- 属性：`channel`、`provider`、`result`、`duration_ms`

### `otp.verify`
- 验证验证码的 span
- 属性：`challenge_id`、`result`、`reason`、`user_id_digest`、`channel`、`purpose`

## 使用示例

### 在代码中创建 Span

```go
import (
    "errors"
    tracing "github.com/soulteary/tracing-kit/v2"
    "go.opentelemetry.io/otel/attribute"
)

// 创建 span
ctx, span := tracing.StartSpan(ctx, "custom.operation")
defer span.End()

// 设置属性
span.SetAttributes(
    attribute.String("key", "value"),
    attribute.Int("count", 42),
)

// 记录错误
if err != nil {
    tracing.RecordError(span, errors.New("operation failed")) // 不传递上游原始错误
}
```

## 参考

- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)

## 渠道错误诊断

发送失败的 span 状态和 exception 事件仅记录 `provider send failed: <reason>`，
不导出上游错误文本。历史 span 不会被改写；处理说明见 [安全文档](../../docs/zhCN/SECURITY.md#发送渠道失败诊断)。
