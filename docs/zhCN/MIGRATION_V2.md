# Herald v2 迁移与安全加固指南（zhCN）

本文档说明安全加固版本引入的破坏性变更、迁移方式与回滚方法，是 v1 → v2 认证
变更、幂等/并发语义、Provider 投递策略、环境变量迁移与生产部署的权威说明。

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
| `X-Key-Id` | 视情况 | 未设置 `HERALD_HMAC_DEFAULT_KEY_ID` 时必需 |

### 1.3 密钥策略

- **无随机默认密钥**：配置多把密钥时，调用方须携带 `X-Key-Id`，或设置
  `HERALD_HMAC_DEFAULT_KEY_ID` 指向已知 id。
- **不降级**：v2 签名存在但无效将被拒绝，绝不静默回退到 API Key。
- **旧版 v1** 仅在 `HMAC_V1_ENABLED=true` 时接受；否则缺少
  `X-Signature-Version: v2` 的请求将被拒绝。

### 1.4 重放保护

nonce 通过 Redis `SET NX EX` 在（`service|key-id|nonce`）的 keyed digest 下
一次性消费。**先验证签名再消费 nonce**，使伪造请求无法烧掉合法 nonce。当 nonce
存储后端故障时，生产环境**失败关闭**（`503`），仅非生产环境放行。

### 1.5 认证模式与客户端证书模式分离

- `REQUEST_AUTH_MODE` = `hmac_v2`（默认）| `api_key` | `none`（仅开发）。
- `CLIENT_CERT_MODE` 独立控制 mTLS，与请求体认证模式解耦。

### 1.6 SDK

Go SDK 默认使用 v2 签名。设置 key id（测试可注入固定时钟/nonce）：

```go
c, _ := herald.NewClient(herald.DefaultOptions().
    WithBaseURL(base).
    WithHMACSecret(secret).
    WithKeyID("primary"))
// v2 为默认 SignatureVersion；仅当对接开启 HMAC_V1_ENABLED=true 的旧服务端时
// 才使用 WithSignatureVersion(herald.SignatureV1)。
```

---

## 2. 幂等与并发语义

- 在 `POST /v1/otp/challenges` 上携带 `Idempotency-Key`。
- 键**按 principal 命名空间隔离**（由 key-id/service/api-key 派生），不同调用方
  即便使用相同的客户端键也不会冲突。
- 首个请求**原子占位**（`SET NX`）。相同 body 的并发重复请求会等待/重放，最终
  **仅触发一次** Provider 发送。
- 相同键 + **不同 body** → `409 Conflict`。
- 成功后精确响应会被存储，并在重试时**原样重放**。
- 幂等后端不可用 → `503`（失败关闭），绝不重复发送。

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
- `/v1/test/code/:id` 仅在测试模式挂载，需 `HERALD_TEST_API_KEY`，开发与生产
  环境均不存在。
- 生产环境若 `HERALD_TEST_MODE=true` 将拒绝启动。

---

## 5. Purpose / 上下文绑定

校验与 challenge 的 `purpose` 绑定；为某 purpose 签发的码不能用于其他 purpose。
同一流程的 create/verify 请保持 `purpose` 一致（如 `login`）。

---

## 6. 环境变量迁移

| 领域 | v1 | v2 | 操作 |
| --- | --- | --- | --- |
| 环境 | （隐式） | `ENVIRONMENT` | 生产设 `production`，启用失败关闭校验 |
| 请求认证 | 仅 `HMAC_SECRET` | `REQUEST_AUTH_MODE`、`HERALD_HMAC_KEYS`、`HERALD_HMAC_DEFAULT_KEY_ID`、`HMAC_MAX_DRIFT`、`HMAC_V1_ENABLED` | 默认 `hmac_v2`，设默认 key id 或携带 `X-Key-Id` |
| 客户端证书 | 与请求认证混用 | `CLIENT_CERT_MODE` | 单独配置 mTLS |
| Provider | `PROVIDER_FAILURE_POLICY` | + `PROVIDER_TIMEOUT`、`PROVIDER_MAX_RESPONSE_BYTES`、`PROVIDER_REDIRECT_POLICY` | 生产用 `strict`，设超时/上限 |
| 测试模式 | `HERALD_TEST_MODE` | + `HERALD_TEST_API_KEY` | 仅测试；生产禁用 |
| 审计 | `AUDIT_LOKI_URL` | **移除** | 删除该项；使用 `database`/`file`/`redis` |
| 隐私 | — | `PII_PEPPER`（回退到幂等/HMAC 密钥） | 生产设专用 pepper |
| CORS | 允许通配 | `CORS_ALLOW_ORIGINS` | 生产禁止通配 |

`HERALD_SESSION_*` 变量已失效（会话存储被移除），可从环境中删除。

---

## 7. 生产部署

1. 设 `ENVIRONMENT=production`。
2. 配置请求认证（`REQUEST_AUTH_MODE=hmac_v2` + 密钥，或 `api_key`）。
3. 使用带密码的 Redis（或在私网启用风险确认标志）。
4. 设 `PROVIDER_FAILURE_POLICY=strict`、`PROVIDER_TIMEOUT` 及 HTTPS Provider URL。
5. 设 `PII_PEPPER` 并保持 `AUDIT_MASK_DESTINATION=true`。
6. 以非 root（UID/GID 10001）运行容器，置于 TLS 或 mTLS 之后。
7. 将 `/livez`（存活）与 `/readyz`（就绪）接入编排系统。

### 回滚

- 对 API Key 部署而言配置兼容：设 `REQUEST_AUTH_MODE=api_key` 可在滚动升级
  SDK 期间保持旧客户端可用。
- 若需临时接受旧 HMAC v1 客户端，设 `HMAC_V1_ENABLED=true`（不建议长期启用，
  其无重放保护）。
- Redis 键为增量式（`otp:nonce:*`、`otp:idem:*`）；回滚二进制后残留键将按 TTL
  过期，无需破坏性迁移。

---

## 8. 过期文档

以下本地化文档**未**随本次发布更新，在重译前可能不准确：`docs/deDE`、
`docs/frFR`、`docs/itIT`、`docs/jaJP`、`docs/koKR`。请优先参考 `docs/enUS`
与 `docs/zhCN`。
