# HMAC Migration and Security Upgrades (enUS)

The filename is retained for existing links. HMAC v2/v3 and the v2 verification
API are protocol versions; Herald service releases remain on the v1.x line.

> Current security update: the Go SDK now defaults to HMAC v3. Challenge creation
> requires v3, including requests without an idempotency key. The historical v2
> canonical format below is retained for compatible verification routes only.
> Coordinate server/client upgrades; see [API authentication](API.md#hmac-v3).

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
- **Legacy v1** requires `HMAC_V1_ENABLED=true` on compatible non-creation routes.
  Challenge creation always requires `X-Signature-Version: v3` in HMAC mode.
  Other routes accept v2/v3; a missing or unknown version is rejected unless
  the legacy fallback is enabled.

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

The Go SDK now signs v3 by default. Set the key id and (optionally) a fixed clock/
nonce for tests:

```go
c, _ := herald.NewClient(herald.DefaultOptions().
    WithBaseURL(base).
    WithHMACSecret(secret).
    WithKeyID("primary"))
// v3 is the default SignatureVersion; older versions are for compatible routes only.
// Use WithSignatureVersion(herald.SignatureV1)
// only when talking to a legacy server with HMAC_V1_ENABLED=true.
```

---

## 2. Idempotency & concurrency semantics

- Send `Idempotency-Key` on `POST /v1/otp/challenges`.
- Keys are namespaced by the literal `X-Service` and `X-Key-Id` headers, or the
  client IP when both headers are absent. An API key is not itself a namespace.
  Keep caller headers stable across retries; `|kid:` is forbidden in either header.
- The first request **atomically claims** the key (`SET NX`). Concurrent
  duplicates with the **same fingerprinted fields** return `409/idempotency_in_flight` before
  sending, or replay a completed response within its TTL.
- Same key + **different fingerprinted fields** → `409 Conflict`.
- After success, the exact response is stored and **replayed** for retries.
- Before delivery, the claim becomes a non-expiring `sending` record. A crash
  or an uncertain post-send failure leaves the key blocked with
  `409/delivery_unknown`; it cannot be reclaimed by TTL expiry. Failures before
  this boundary can still release or expire the pending claim.
- A failed success-record write returns `503/backend_unavailable`. If Redis
  committed the write but its acknowledgement was lost, a retry replays it.
- Provider errors after this boundary also retain the record, including strict
  mode failures. This trades automatic recovery for avoiding duplicate sends.
  Operators must reconcile unresolved records; do not blindly delete them or
  retry with a new key. Redis data loss, eviction, or key/secret changes can
  defeat this protection. Completed responses still expire normally.

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
- `/v1/test/code/:id` is mounted only on the separate test listener in test mode, requires
  `HERALD_TEST_API_KEY`, and is never present in development or production.
- Production refuses to start if `HERALD_TEST_MODE=true`.

---

## 5. Purpose / context binding

Use `POST /v2/otp/verifications` with all three expected context fields from
trusted application state: `expected_user_id`, `expected_purpose` and
`expected_channel`. The v1 verification route has no caller-supplied context
binding; do not treat it as providing the v2 guarantees.

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
| Privacy | — | `HERALD_PII_PEPPER` (falls back to idempotency/HMAC secret) | Set a dedicated pepper in prod |
| Rate-limit CORS | wildcard allowed | `HERALD_CORS_ALLOW_ORIGINS` | No wildcard in prod |

`HERALD_SESSION_*` variables are inert (session storage was removed) and can be
deleted from your environment.

---

## 7. Production deployment

1. Set `ENVIRONMENT=production`.
2. Configure request auth (`REQUEST_AUTH_MODE=hmac_v2` + keys, or `api_key`).
3. Set a password-protected Redis (or the risk-ack flag on a private network).
4. Set `PROVIDER_FAILURE_POLICY=strict`, `PROVIDER_TIMEOUT`, HTTPS provider URLs.
5. Set `HERALD_PII_PEPPER` and keep `AUDIT_MASK_DESTINATION=true`.
6. Deploy the container (runs as non-root UID/GID 10001) behind TLS or mTLS.
7. Wire `/livez` (liveness) and `/readyz` (readiness) into your orchestrator.

### Rollback

- The change is config-compatible for API-key deployments: set
  `REQUEST_AUTH_MODE=api_key` to keep pre-v2 clients working while you roll SDKs.
- `HMAC_V1_ENABLED=true` permits legacy signatures only on compatible
  non-creation routes. It cannot restore v1/v2 challenge creation. Coordinate
  server/client upgrades as described in [API authentication](API.md#hmac-v3).
- Redis keys are additive (`otp:nonce:*`, `otp:idem2:*`). Nonce and completed
  response keys expire normally; unresolved `sending` records do not. Preserve
  them during rollback and reconcile them before any cleanup.

---

## 8. Stale docs

The following localized docs were **not** updated for this release and may be
inaccurate until retranslated: `docs/deDE`, `docs/frFR`, `docs/itIT`,
`docs/jaJP`, `docs/koKR`. Prefer `docs/enUS` and `docs/zhCN`.

## Required context for v2 OTP verification

`POST /v2/otp/verifications` now requires non-blank `expected_user_id`,
`expected_purpose`, and `expected_channel`. Previously, omitted or empty fields
silently disabled the corresponding binding checks. Missing context now returns
`400/context_required`; mismatched context returns `409/context_mismatch`.
Neither rejection consumes the challenge. Supply all three values from trusted
application state before upgrading existing v2 callers. The v1 verification
endpoint retains its existing unbound semantics.

## Durable audit buffering

Audit-enabled deployments now need a writable, persistent `AUDIT_SPOOL_DIR`.
The Compose file adds an `audit_spool` volume; retain it during upgrade and
rollback, and do not run old and new instances against the same directory.
`AUDIT_WRITER_QUEUE_SIZE` controls notification capacity rather than record
retention. Local acceptance requires fsync; backend delivery is at least once.
A full or unavailable buffer returns `503/audit_unavailable`. See the deployment
guide for capacity, permissions, and duplicate-event handling.

## Activation indexes and provider diagnostics

For upgrades from versions before v1.3.10, follow the [activation index upgrade](API.md#activation-index-upgrade): pause creation, drain requests and replace all instances before resuming. Preserve unresolved sending records and audit volumes. Before rollback, wait the maximum challenge lifetime with creation stopped.

Delivery diagnostics now contain bounded `reason` codes rather than raw provider errors. See [provider failure diagnostics](SECURITY.md#provider-failure-diagnostics) for field compatibility and historical log/trace handling.
