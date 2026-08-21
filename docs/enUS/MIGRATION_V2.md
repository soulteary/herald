# Herald v2 Migration & Security Hardening Guide (enUS)

This document describes the breaking changes introduced by the security-hardening
release, how to migrate, and how to roll back. It is the authoritative source for
the v1 → v2 authentication change, idempotency/concurrency semantics, provider
delivery policy, environment-variable migration, and production deployment.

> Other-language docs (deDE, frFR, itIT, jaJP, koKR) are **stale** with respect
> to this release. Only enUS and zhCN are current. See "Stale docs" at the end.

---

## 1. Authentication: HMAC v1 → v2

### 1.1 What changed

v1 signed only `timestamp:service:body` with a shared secret. It had no
replay protection, allowed an implicit/random default key, and could be
downgraded to API-key auth.

v2 binds the **entire request** and is replay-resistant:

```
HERALD-HMAC-V2\n
<METHOD uppercased>\n
<path>\n
<raw query string>\n
<unix-seconds timestamp>\n
<nonce>\n
<service>\n
<key-id>\n
<hex sha256(body)>
```

The HMAC-SHA256 of that canonical string (hex) is sent in `X-Signature`.

### 1.2 Required headers (v2)

| Header | Required | Notes |
| --- | --- | --- |
| `X-Signature-Version` | yes | must be `v2` |
| `X-Signature` | yes | hex HMAC-SHA256 of the canonical string |
| `X-Timestamp` | yes | unix seconds; must be within ±`HMAC_MAX_DRIFT` (default 60s) |
| `X-Nonce` | yes | single-use; replays are rejected |
| `X-Service` | yes | logical caller identity, bound into the signature |
| `X-Key-Id` | conditionally | required unless `HERALD_HMAC_DEFAULT_KEY_ID` is set |

### 1.3 Key policy

- **No random default key.** If multiple keys are configured, a caller must send
  `X-Key-Id`, or you must set `HERALD_HMAC_DEFAULT_KEY_ID` to a known id.
- **No downgrade.** A present-but-invalid v2 signature is rejected; Herald never
  silently falls back to API-key auth.
- **Legacy v1** is only accepted when `HMAC_V1_ENABLED=true`. Requests without
  `X-Signature-Version: v2` are otherwise rejected.

### 1.4 Replay protection

Nonces are consumed exactly once via Redis `SET NX EX` under a keyed digest
(`service|key-id|nonce`). The signature is verified **before** the nonce is
consumed so a forged request cannot burn a legitimate nonce. On nonce-store
backend failure Herald **fails closed** in production (`503`), and allows the
request only in non-production.

### 1.5 Auth vs client-cert modes are split

- `REQUEST_AUTH_MODE` = `hmac_v2` (default) | `api_key` | `none` (dev only).
- `CLIENT_CERT_MODE` controls mTLS independently of the request-body auth mode.

### 1.6 SDK

The Go SDK signs v2 by default. Set the key id and (optionally) a fixed clock/
nonce for tests:

```go
c, _ := herald.NewClient(herald.DefaultOptions().
    WithBaseURL(base).
    WithHMACSecret(secret).
    WithKeyID("primary"))
// v2 is the default SignatureVersion; use WithSignatureVersion(herald.SignatureV1)
// only when talking to a legacy server with HMAC_V1_ENABLED=true.
```

---

## 2. Idempotency & concurrency semantics

- Send `Idempotency-Key` on `POST /v1/otp/challenges`.
- Keys are **namespaced per principal** (derived from key-id/service/api-key), so
  two callers using the same client-chosen key do not collide.
- The first request **atomically claims** the key (`SET NX`). Concurrent
  duplicates with the **same body** wait/replay and result in **exactly one**
  provider send.
- Same key + **different body** → `409 Conflict`.
- After success, the exact response is stored and **replayed** for retries.
- If the idempotency backend is unavailable → `503` (fail closed), never a
  double-send.

---

## 3. Provider delivery policy

- `PROVIDER_TIMEOUT` (e.g. `5s`): explicit per-send timeout (floored to a safe
  minimum).
- `PROVIDER_MAX_RESPONSE_BYTES`: response body is read via `io.LimitReader`.
- `PROVIDER_REDIRECT_POLICY` = `deny` (default) | `same-origin`. Redirects to a
  different host/scheme are blocked and credentials (`Authorization`,
  `X-API-Key`) are never forwarded across redirects.
- Provider URLs must be `https` in production.
- `PROVIDER_FAILURE_POLICY`:
  - `strict` (**required in production**): a send failure **revokes** the pending
    challenge and returns `500 send_failed`.
  - `soft` (dev/degraded only): the challenge is still created and the response
    includes `delivery_status: "failed"`.

---

## 4. Test-mode limits

- `debug_code` appears in the create response **only** when
  `ENVIRONMENT=test` **and** `HERALD_TEST_MODE=true`.
- `/v1/test/code/:id` is mounted only in test mode, requires
  `HERALD_TEST_API_KEY`, and is never present in development or production.
- Production refuses to start if `HERALD_TEST_MODE=true`.

---

## 5. Purpose / context binding

Verification is bound to the challenge's `purpose`; a code issued for one purpose
cannot be redeemed for another. Keep the `purpose` value stable across
create/verify for a given flow (e.g. `login`).

---

## 6. Environment variable migration

| Area | v1 | v2 | Action |
| --- | --- | --- | --- |
| Environment | (implicit) | `ENVIRONMENT` | Set `production` in prod; enables fail-closed validation |
| Request auth | `HMAC_SECRET` only | `REQUEST_AUTH_MODE`, `HERALD_HMAC_KEYS`, `HERALD_HMAC_DEFAULT_KEY_ID`, `HMAC_MAX_DRIFT`, `HMAC_V1_ENABLED` | Default is `hmac_v2`; set a default key id or send `X-Key-Id` |
| Client certs | mixed with request auth | `CLIENT_CERT_MODE` | Configure mTLS separately |
| Provider | `PROVIDER_FAILURE_POLICY` | + `PROVIDER_TIMEOUT`, `PROVIDER_MAX_RESPONSE_BYTES`, `PROVIDER_REDIRECT_POLICY` | Use `strict` in prod; set timeout/bounds |
| Test mode | `HERALD_TEST_MODE` | + `HERALD_TEST_API_KEY` | Test-only; forbidden in prod |
| Audit | `AUDIT_LOKI_URL` | **removed** | Remove it; use `database`/`file`/`redis` |
| Privacy | — | `PII_PEPPER` (falls back to idempotency/HMAC secret) | Set a dedicated pepper in prod |
| Rate-limit CORS | wildcard allowed | `CORS_ALLOW_ORIGINS` | No wildcard in prod |

`HERALD_SESSION_*` variables are inert (session storage was removed) and can be
deleted from your environment.

---

## 7. Production deployment

1. Set `ENVIRONMENT=production`.
2. Configure request auth (`REQUEST_AUTH_MODE=hmac_v2` + keys, or `api_key`).
3. Set a password-protected Redis (or the risk-ack flag on a private network).
4. Set `PROVIDER_FAILURE_POLICY=strict`, `PROVIDER_TIMEOUT`, HTTPS provider URLs.
5. Set `PII_PEPPER` and keep `AUDIT_MASK_DESTINATION=true`.
6. Deploy the container (runs as non-root UID/GID 10001) behind TLS or mTLS.
7. Wire `/livez` (liveness) and `/readyz` (readiness) into your orchestrator.

### Rollback

- The change is config-compatible for API-key deployments: set
  `REQUEST_AUTH_MODE=api_key` to keep pre-v2 clients working while you roll SDKs.
- To temporarily accept old HMAC v1 clients, set `HMAC_V1_ENABLED=true` (not
  recommended long-term; it has no replay protection).
- Redis keys are additive (`otp:nonce:*`, `otp:idem:*`); rolling back the binary
  leaves stale keys that expire by TTL. No destructive migration is required.

---

## 8. Stale docs

The following localized docs were **not** updated for this release and may be
inaccurate until retranslated: `docs/deDE`, `docs/frFR`, `docs/itIT`,
`docs/jaJP`, `docs/koKR`. Prefer `docs/enUS` and `docs/zhCN`.
