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

# OTLP 端点（例如：http://localhost:4318）
OTLP_ENDPOINT=http://localhost:4318
```

## 核心 Span

### `otp.challenge.create`
- 创建验证码 challenge 的 span
- 属性：`channel`、`purpose`、`user_id`、`destination`（脱敏）

### `otp.provider.send`
- 发送验证码到 provider 的 span
- 属性：`channel`、`provider`、`result`、`duration_ms`

### `otp.verify`
- 验证验证码的 span
- 属性：`challenge_id`、`result`、`reason`、`user_id`、`channel`、`purpose`

## 使用示例

### 在代码中创建 Span

```go
import "github.com/soulteary/herald/internal/tracing"

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
    tracing.RecordError(span, err)
}
```

## 参考

- [OpenTelemetry Go](https://opentelemetry.io/docs/languages/go/)
