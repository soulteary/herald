# HMAC 迁移与安全升级指南（zhCN）

文件名保留以兼容已有链接。HMAC v2/v3 和 v2 验证 API 是协议版本，Herald 服务版本仍为 v1.x。

> 安全更新：Go SDK 现默认使用 HMAC v3；创建验证码必须使用 v3，包括不带幂等键的请求。
> 下文 v2 签名格式仅用于保留兼容性的验证接口。需协调服务端和调用方升级，
> 详见 [API 认证](API.md#hmac-v3)。

本文档记录安全升级及历史 HMAC v2 迁移规则。现行行为以 [API](API.md)、
[部署](DEPLOYMENT.md) 和 [安全](SECURITY.md) 为准；下文 v2 签名仅适用于仍兼容它的路由。

> 其他语言文档（deDE、frFR、itIT、jaJP、koKR）相对本次发布**已过期**，仅
> enUS 与 zhCN 为最新。详见文末“过期文档”。

---

## 1. 认证：HMAC v1 → v2

### 1.1 变更内容

v1 仅对 `timestamp:service:body` 用共享密钥签名，无重放保护，允许隐式/随机的
默认密钥，且可被降级为 API Key 认证。

v2 绑定**整个请求**并具备抗重放能力，规范串（canonical）如下：

```
HERALD-HMAC-V2\n
<大写 METHOD>\n
<path>\n
<原始 query 串>\n
<unix 秒级时间戳>\n
<nonce>\n
<service>\n
<key-id>\n
<hex sha256(body)>
```

对上述规范串计算 HMAC-SHA256（hex）放入 `X-Signature`。

### 1.2 必需请求头（v2）

| 头 | 必需 | 说明 |
| --- | --- | --- |
| `X-Signature-Version` | 是 | 必须为 `v2` |
| `X-Signature` | 是 | 规范串的 hex HMAC-SHA256 |
| `X-Timestamp` | 是 | unix 秒；须在 ±`HMAC_MAX_DRIFT`（默认 60s）内 |
| `X-Nonce` | 是 | 一次性；重放将被拒绝 |
| `X-Service` | 是 | 调用方标识，参与签名绑定 |
| `X-Key-Id` | 视情况 | 配置 `HERALD_HMAC_KEYS` 后必需，包括单项 map |

### 1.3 密钥策略

- **显式映射身份**：配置 `HERALD_HMAC_KEYS` 时，每个 v2/v3 调用方必须携带
  `X-Key-Id`，配置默认项不能替代这个签名字段。单 `HMAC_SECRET` 模式保留稳定的
  隐式 `default` 身份。
- **不降级**：v2 签名存在但无效将被拒绝，绝不静默回退到 API Key。
- **旧版 v1** 仅在 `HMAC_V1_ENABLED=true` 时用于兼容的非创建路由。
  HMAC 模式的创建请求始终要求 `X-Signature-Version: v3`；其他路由接受 v2/v3。

### 1.4 重放保护

nonce 通过 Redis `SET NX EX` 在（`service|key-id|nonce`）的 keyed digest 下
一次性消费。**先验证签名再消费 nonce**，使伪造请求无法烧掉合法 nonce。当 nonce
存储后端故障时，生产环境**失败关闭**（`503`），仅非生产环境放行。

### 1.5 认证模式与客户端证书模式分离

- `REQUEST_AUTH_MODE` = `hmac_v2`（默认）| `api_key` | `none`（仅开发）。
- `CLIENT_CERT_MODE` 独立控制 mTLS，与请求体认证模式解耦。

### 1.6 SDK

SDK 的 `HMACSecret` 非空时选择 HMAC，省略 `X-API-Key`，即使同时配置了
`APIKey`。HMAC 请求被拒绝后不会换用其他凭据重试。服务端使用
`REQUEST_AUTH_MODE=api_key` 时，SDK 应只配置 API key，并留空 HMAC secret。
旧客户端若同时发送两类凭据并依赖服务端选择 API key，升级前需调整配置。

Go SDK 默认使用 v3 签名。设置 key id（测试可注入固定时钟/nonce）：

```go
c, _ := herald.NewClient(herald.DefaultOptions().
    WithBaseURL(base).
    WithHMACSecret(secret).
    WithKeyID("default"))
// v3 为默认 SignatureVersion；仅当对接开启 HMAC_V1_ENABLED=true 的旧服务端时
// 才使用 WithSignatureVersion(herald.SignatureV1)。
```

---

## 2. 幂等与并发语义

升级使用 `HERALD_HMAC_KEYS` 的验证实例前，先为所有调用方配置原有显式 key ID
（SDK 的 `WithKeyID`，Stargate 的 `HERALD_HMAC_KEY_ID`）；旧验证实例已支持这些
请求，再依次替换验证实例。
密钥 map 请求省略 ID 时，在进入 handler 和写入 nonce 前返回
`401/key_id_required`，单项 map 也一样。此前，省略 ID 的同一签名请求可能在默认
ID 改变、密钥内容不变后再次通过认证。

保留 key ID、Redis 数据、nonce 前缀和幂等派生密钥，无需转换记录。回滚会恢复旧的
省略 ID 行为；存在旧验证实例或回滚后，不要在保留相同密钥内容时重新指定默认 ID。
旧版 v1 仍是需显式启用、不提供 nonce 重放保护的独立协议。

- 在 `POST /v1/otp/challenges` 上携带 `Idempotency-Key`。
- HMAC principal 使用已签名的服务标识和实际验签密钥 ID。单密钥模式省略 ID 与显式 `default`
  使用同一命名空间。API Key/开发模式保留原始调用头，两者均缺失时使用客户端 IP；
  API key 本身不构成命名空间。服务及密钥 ID（含解析后的默认 ID）均不得含 `|kid:`。
- 首个请求**原子占位**；HMAC 创建还原子检查旧空 Key ID 命名空间。发送前相同指纹字段的并发请求返回
  `409/idempotency_in_flight`，已完成请求在 TTL 内重放响应。
- 相同键 + **不同指纹字段** → `409 Conflict`。
- 成功后精确响应会被存储，并在重试时**原样重放**。
- 调用渠道前将占位记录转为 `sending` 状态。进程退出或发送后结果不确定时，
  同一键返回 `409/delivery_unknown`。支持 `IDEMPOTENCY_UNRESOLVED_TTL` 的版本会在该时长
  （默认 `24h`）后使该记录过期；更早的版本写入时不带 TTL。发送前的失败仍可释放或
  等待占位记录过期。
- 最终成功记录写入失败返回 `503/backend_unavailable`；若写入已提交但确认丢失，重试会重放。
- 渠道报错（包括 strict 模式）也保留发送记录，以牺牲自动恢复来避免重复发送。
  运维需核对不确定记录，不能直接删除记录或更换键重发。Redis 数据丢失、淘汰记录或更换
  密钥等操作会破坏此保障；正常成功响应仍按 TTL 过期。

---

## 3. Provider 投递策略

- `PROVIDER_TIMEOUT`（如 `5s`）：显式单次发送超时（有安全下限）。
- `PROVIDER_MAX_RESPONSE_BYTES`：响应体经 `io.LimitReader` 读取。
- `PROVIDER_REDIRECT_POLICY` = `deny`（默认）| `same-origin`。跨主机/协议的
  跳转被阻断，跳转时绝不转发凭据（`Authorization`、`X-API-Key`）。
- 生产环境 Provider URL 必须为 `https`。
- `PROVIDER_FAILURE_POLICY`：
  - `strict`（**生产必需**）：发送失败**撤销**待激活 challenge，返回
    `500 send_failed`。
  - `soft`（仅开发/降级）：仍创建 challenge，响应包含
    `delivery_status: "failed"`。

---

## 4. 测试模式限制

- `debug_code` **仅**在 `ENVIRONMENT=test` **且** `HERALD_TEST_MODE=true` 时
  出现在创建响应中。
- `/v1/test/code/:id` 仅在测试模式的独立监听器挂载，需 `HERALD_TEST_API_KEY`，开发与生产
  环境均不存在。
- 生产环境若 `HERALD_TEST_MODE=true` 将拒绝启动。

---

## 5. Purpose / 上下文绑定

使用 `POST /v2/otp/verifications`，从业务后端可信状态提供 `expected_user_id`、
`expected_purpose`、`expected_channel`。v1 验证接口没有调用方提供的上下文绑定，不能把它当作 v2 使用。
所有调用方迁移到 v2 后，设置 `HERALD_VERIFY_V1_ENABLED=false` 可停止提供 v1 验证路由
（默认 `true`）。

---

## 6. 环境变量迁移

| 领域 | v1 | v2 | 操作 |
| --- | --- | --- | --- |
| 环境 | （隐式） | `ENVIRONMENT` | 生产设 `production`，启用失败关闭校验 |
| 请求认证 | 仅 `HMAC_SECRET` | `REQUEST_AUTH_MODE`、`HERALD_HMAC_KEYS`、`HERALD_HMAC_DEFAULT_KEY_ID`、`HMAC_MAX_DRIFT`、`HMAC_V1_ENABLED` | 默认 `hmac_v2`，密钥 map 的 v2/v3 请求须携带 `X-Key-Id` |
| 客户端证书 | 与请求认证混用 | `CLIENT_CERT_MODE` | 单独配置 mTLS |
| Provider | `PROVIDER_FAILURE_POLICY` | + `PROVIDER_TIMEOUT`、`PROVIDER_MAX_RESPONSE_BYTES`、`PROVIDER_REDIRECT_POLICY` | 生产用 `strict`，设超时/上限 |
| 测试模式 | `HERALD_TEST_MODE` | + `HERALD_TEST_API_KEY` | 仅测试；生产禁用 |
| 审计 | `AUDIT_LOKI_URL` | **移除** | 删除该项；使用 `database`/`file`/`redis` |
| 隐私 | — | `HERALD_PII_PEPPER`（回退到幂等/HMAC 密钥） | 生产设专用 pepper |
| CORS | 允许通配 | `HERALD_CORS_ALLOW_ORIGINS` | 生产禁止通配 |

`HERALD_SESSION_*` 变量已失效（会话存储被移除），可从环境中删除。

---

## 7. 生产部署

1. 设 `ENVIRONMENT=production`。
2. 配置请求认证（`REQUEST_AUTH_MODE=hmac_v2` + 密钥，或 `api_key`）。
3. 使用带密码的 Redis（或在私网启用风险确认标志）。
4. 设 `PROVIDER_FAILURE_POLICY=strict`、`PROVIDER_TIMEOUT` 及 HTTPS Provider URL。
5. 设 `HERALD_PII_PEPPER` 并保持 `AUDIT_MASK_DESTINATION=true`。
6. 以非 root（UID/GID 10001）运行容器，置于 TLS 或 mTLS 之后。
7. 将 `/livez`（存活）与 `/readyz`（就绪）接入编排系统。

### 回滚

- 对 API Key 部署而言配置兼容：设 `REQUEST_AUTH_MODE=api_key` 可在滚动升级
  SDK 期间保持旧客户端可用。
- `HMAC_V1_ENABLED=true` 只影响兼容的非创建路由，不能恢复 v1/v2 创建请求。
  服务端与调用方需按 [API 认证](API.md#hmac-v3) 协调升级。
- Redis 键为增量式（`otp:nonce:*`、`otp:idem2:*`）；不确定的 `sending` 记录在支持 `IDEMPOTENCY_UNRESOLVED_TTL` 的版本中于该时长后过期，由更早版本写入的则不会过期；回滚时也需保留并核对。其他键按 TTL
  过期，无需破坏性迁移。

---

## 8. 过期文档

以下本地化文档**未**随本次发布更新，在重译前可能不准确：`docs/deDE`、
`docs/frFR`、`docs/itIT`、`docs/jaJP`、`docs/koKR`。请优先参考 `docs/enUS`
与 `docs/zhCN`。

## v2 OTP 验证的必填上下文

`POST /v2/otp/verifications` 现在要求同时提供非空白的 `expected_user_id`、
`expected_purpose` 和 `expected_channel`。此前省略字段或传空串会跳过对应绑定校验。
缺少上下文现在返回 `400/context_required`，上下文不匹配返回 `409/context_mismatch`，
两种拒绝均不消费 challenge。升级前，请让现有 v2 调用方从业务后端可信状态中补齐三个值。
v1 验证接口保留原有的无上下文绑定语义。

## 持久化审计缓冲

启用审计的部署现在需要可写、持久化的 `AUDIT_SPOOL_DIR`。Compose 增加了
`audit_spool` 卷，升级和回滚时需保留，且新旧实例不能共用目录。
`AUDIT_WRITER_QUEUE_SIZE` 现在只控制通知容量，不控制记录保留。本地接收需要 fsync，
后端按至少一次语义投递。缓冲已满或不可用时返回 `503/audit_unavailable`。
容量、权限及重复事件处理方式见部署指南。

## 激活索引与渠道诊断升级

从 v1.3.10 之前的版本升级时，按 [激活索引升级](API.md#激活索引升级) 暂停创建流量、排空进行中的请求并替换全部实例，之后再恢复。保留不确定发送记录和审计持久卷；回滚前应暂停创建并等待最长验证码有效期。

渠道诊断现在使用受控 `reason`，不再记录原始错误文本。诊断字段兼容性及历史日志、span 处理要求见 [发送渠道失败诊断](SECURITY.md#发送渠道失败诊断)。

## v1.3.13 身份与来源升级

仅使用 `HMAC_SECRET` 时省略 `X-Key-Id` 或填写 `default`；自定义 ID 需显式 map 项。
不能直接重命名调用方或删除不确定发送记录。暂停创建、排空请求、核对历史身份，再替换全部
旧实例；保留 Redis、审计卷、nonce 前缀及有效幂等派生密钥。旧空 Key ID 记录以
`409/idempotency_namespace_conflict` 阻止创建，不自动迁移或重放。

遵循 [部署升级与回滚要求](DEPLOYMENT.md#hmac-身份升级)，并在升级或回滚前移除生产 CORS
列表中的独立 `*` 项。两项变化均无需新签名格式，HMAC 创建仍要求 v3。旧 v1 回退已弃用，
不具备 nonce 防重放保护。

## Herald v1.4.0 协调升级

Herald 公开 Go 模块路径及外部 HTTP 数据结构保持不变，但本次安全版本需要
部署准备：先配置独立的指标凭据并更新采集器；生产环境不再允许匿名采集，
不得复用业务 API/HMAC 密钥。启用服务策略时，为每个主体配置独立 HMAC 密钥
并列齐所需动作；该策略不提供共享 subject 空间中的用户或租户隔离。

停止 OTP 创建、排空请求并等待旧验证码最长有效期结束，再替换所有验证实例。
保留用户锁定、未决幂等记录与审计数据。将 herald-totp v2.0.0 与 Herald v1.4.0
协调升级后再恢复 HMAC 流量；上游保留 `/v1` HTTP 路径，但拒绝旧 HMAC 签名。
保留 TOTP 加密密钥和已有凭据，仅 API key 模式仍受支持。SDK 和 TOTP 代理
拒绝重定向，必须配置最终服务 URL，并在恢复前用隔离收件人验证认证操作。

Herald 内部采用 `herald-totp/v2` Go 导入；Herald SDK 的调用方仍使用
`github.com/soulteary/herald/pkg/herald`，公开 TOTP 请求/响应类型不变。
直接导入 herald-totp 的应用需自行迁移到 `/v2` 并遵循其主版本迁移说明。


## 从 v1.4.0 升级到 v1.4.1

先部署 herald-totp v2.0.1 以执行确认配额，再部署 Herald v1.4.1 以保留完整的
上游 429。更新 Go 依赖不等于部署后端。两个补丁版本之间的 TOTP `/v2` 模块、
HMAC v2 协议和已有 HTTP 路径不变。

完整的上游确认 429 现在返回 HTTP 429 与固定的
`{"ok":false,"reason":"rate_limited"}`，不转发上游正文或响应头；截断或超长
错误仍返回 502/proxy_failed。客户端应保留 pending 注册上下文供重试，但仍
遵守其有效期。subject 配额需覆盖开始、确认和错误尝试，上限为一时无法在
第一个小时窗口内完成注册。

本次补丁继续使用相同的 OTP challenge 和幂等命名空间，无需额外数据转换或
键空间迁移。保留 Redis、未决 sending 记录、幂等密钥、nonce 与审计状态和
TOTP 加密密钥，不要因拒绝请求而清除不确定记录或更换幂等键。更早主版本或
旧 OTP 命名空间仍需遵循原迁移步骤。

Herald 回滚至 v1.4.0 会再次把确认 429 映射为 502；TOTP 回滚至 v2.0.0 会失去
确认配额。保留持久状态，不要在网关回滚时隐式降级独立部署的后端。
