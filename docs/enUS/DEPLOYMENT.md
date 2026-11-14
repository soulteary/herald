# Herald Deployment Guide

## Quick Start

Linux is the build and validation environment. For macOS, Windows, and other environments, use the Docker Compose deployment below to run Herald in a Linux container. Native builds on those platforms are not covered by CI.

### Using Docker Compose

```bash
cd herald
export REDIS_PASSWORD="$(openssl rand -hex 32)"
export HERALD_API_KEY="$(openssl rand -hex 32)"
docker compose up -d
```

The checked-in Compose service sets `PROVIDER_FAILURE_POLICY=strict`. Production requires `strict`; it is also what production uses when the variable is unset.

### Manual Deployment on Linux

```bash
# Build
go build -o herald main.go

# Configure the password-protected Redis endpoint before running.
export ENVIRONMENT=production REQUEST_AUTH_MODE=api_key PROVIDER_FAILURE_POLICY=strict
: "${API_KEY:?load a secret of at least 32 bytes}"
: "${REDIS_PASSWORD:?load the Redis password}"
: "${AUDIT_SPOOL_DIR:?set a private persistent audit directory}"
./herald
```

## Configuration

### Configuration Naming Convention

**Note**: While the specification recommends using `HERALD_*` prefix for all configuration variables, the current implementation uses a mix of naming conventions:
- Some variables use `HERALD_*` prefix (e.g., `HERALD_TEST_MODE`, `HERALD_HMAC_KEYS`)
- Some variables use shorter names (e.g., `REDIS_ADDR`, `API_KEY`, `HMAC_SECRET`)

Use the exact variable names listed below. Prefixes are not interchangeable.
`HERALD_API_KEY` is a Compose input mapped to the process variable `API_KEY`;
`HERALD_REDIS_ADDR` is not read by Herald. Only documented aliases are supported.

### Environment Variables

The following match the implementation in `internal/config/config.go`.

Value formats are checked at startup. A boolean must be one of `1`, `t`, `T`,
`TRUE`, `true`, `True`, `0`, `f`, `F`, `FALSE`, `false` or `False`; `yes`, `on`
and values with surrounding whitespace are not accepted. An integer must be a
plain decimal number. A duration needs a unit, for example `60s` or `5m`; `60`
is not accepted. A value in the wrong format stops startup in production. In
other environments Herald logs a warning and uses the default.

#### Server and Redis

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `ENVIRONMENT` | Runtime policy: `development`/`dev`, `test`/`testing`, `production`/`prod`; unknown values fail startup. When it is unset Herald logs a startup warning, because production validation is then not enforced; a later release will refuse to start without it | `development` | Set it explicitly; `production` in production |
| `PORT` | Server listen port (with or without leading colon, e.g. `8082` or `:8082`) | `:8082` | No |
| `REDIS_ADDR` | Redis address (`host:port`); an invalid value fails startup in production and falls back to the default with a warning elsewhere | `localhost:6379` | No |
| `REDIS_PASSWORD` | Redis password | (empty) | Production, unless explicitly risk-acknowledged |
| `REDIS_DB` | Redis database index | `0` | No |
| `REDIS_USERNAME` | Redis ACL username; empty uses the default user. Requires `REDIS_PASSWORD` | (empty) | No |
| `REDIS_TLS_ENABLED` | Connect to Redis over TLS (1.2 or later); the server certificate is always verified | `false` | No |
| `REDIS_TLS_CA_FILE` | PEM CA bundle used to verify the Redis certificate; empty uses the system roots. Needs `REDIS_TLS_ENABLED=true` | (empty) | No |
| `REDIS_TLS_SERVER_NAME` | Name expected in the Redis certificate; empty uses the `REDIS_ADDR` host. Needs `REDIS_TLS_ENABLED=true` | (empty) | No |
| `LOG_LEVEL` | Log level | `info` | No |
| `SERVICE_NAME` | Service identifier (HMAC, logging, health) | `herald` | No |
| `HERALD_READ_TIMEOUT` | Request read timeout | `15s` | No |
| `HERALD_WRITE_TIMEOUT` | Response write timeout | `15s` | No |
| `HERALD_IDLE_TIMEOUT` | Idle connection timeout | `60s` | No |
| `HERALD_CORS_ALLOW_ORIGINS` | Explicit comma-separated origin allowlist; empty disables CORS, wildcard forbidden in production | (empty) | No |
| `HERALD_RISK_ACK_PASSWORDLESS_REDIS` | Explicit production acknowledgement for passwordless Redis; prefer a password | `false` | Only for that exception |
| `HERALD_NONCE_PREFIX` | Redis nonce key prefix; keep stable across instances/upgrades | `otp:nonce:` | No |
| `PROVIDER_TIMEOUT` | Positive provider request timeout | `10s` | No |
| `PROVIDER_REDIRECT_POLICY` | HTTP provider redirects: `deny` or `same-origin`; TOTP always denies redirects | `deny` | No |
| `HERALD_TRUSTED_PROXY_HEADER` | Forwarded client-IP header; only read from trusted proxies | (empty) | With trusted proxies |
| `HERALD_TRUSTED_PROXIES` | Comma-separated trusted proxy IP/CIDR allowlist | (empty) | With proxy header |

These two settings also decide what `RATE_LIMIT_PER_IP` counts. Without them the limit applies to the direct peer, so a gateway that calls Herald for many end users shares one quota among them. With them, a trusted gateway can forward the end-user address (Go SDK: `WithClientIPHeader`) and the limit applies per end user. A request refused by this quota answers `429/rate_limit_exceeded` with `"limit":"ip"`.

Forwarded IP chains are parsed from right to left. Herald skips only explicitly trusted proxy hops and uses the first untrusted address as the client IP; a caller-controlled leftmost prefix is never trusted automatically.

#### Service-to-service authentication

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `REQUEST_AUTH_MODE` | Request auth: `hmac_v2`, `api_key`, or `none` (dev only) | `hmac_v2` | Yes in production |
| `API_KEY` | API key for `api_key` mode; at least 32 bytes in production | (empty) | API-key mode |
| `HMAC_SECRET` | Single HMAC secret; at least 32 bytes in production | (empty) | HMAC single-key mode |
| `HERALD_HMAC_KEYS` | HMAC key map; IDs/secrets must be non-empty and production secrets at least 32 bytes. A set but malformed map fails startup in production; elsewhere it is ignored with a warning and `HMAC_SECRET` is used | (empty) | HMAC multi-key mode |
| `HERALD_HMAC_DEFAULT_KEY_ID` | Default for configuration lookups; key-map v2/v3 requests still require an explicit `X-Key-Id` | (empty) | Optional |
| `HMAC_MAX_DRIFT` | Accepted HMAC v2/v3 timestamp drift | `60s` | No |
| `HMAC_V1_ENABLED` | Temporarily enable legacy HMAC v1 | `false` | No |
| `HERALD_IDEMPOTENCY_SECRET` | Key for opaque idempotency records; required for production multi-key-only auth. Outside production, when it is unset Herald uses `HMAC_SECRET`, then `API_KEY`, then a value derived from `HERALD_HMAC_KEYS`; with none of them it generates a random value at each start and logs a warning, so idempotency, nonce, rate-limit and cooldown records are not shared between instances and do not survive a restart | (empty) | Conditional |
| `HERALD_PII_PEPPER` | Pepper for PII-derived Redis/metric keys; at least 32 bytes in production | (empty) | Recommended |

A missing credential does not disable authentication. With `REQUEST_AUTH_MODE=hmac_v2` (the default) and no `HMAC_SECRET`/`HERALD_HMAC_KEYS`, or with `api_key` and no `API_KEY`, Herald logs a startup warning outside production and every authenticated endpoint returns 401 until the credential is configured; production refuses to start. Only `REQUEST_AUTH_MODE=none` (dev/test only) accepts unauthenticated requests.

#### OTP / Challenge

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_MAX_CONCURRENT_HASH_OPERATIONS` | Shared creation/v1/v2 verification admission budget per handler instance; positive integer | `4` | No |
| `CHALLENGE_EXPIRY` | Challenge expiry (e.g. `5m`, `300s`). Built-in messages state it in whole minutes, rounded down, and as 1 minute when it is under a minute | `5m` | No |
| `MAX_ATTEMPTS` | Wrong codes a challenge accepts before it is exhausted and answers `locked` | `5` | No |
| `LOCKOUT_DURATION` | How long a locked user stays locked, and the window in which exhausted challenges are counted (e.g. `10m`) | `10m` | No |
| `LOCKOUT_CHALLENGE_THRESHOLD` | Number of challenges a user must exhaust (`MAX_ATTEMPTS` wrong codes each) within `LOCKOUT_DURATION`, counted from the first one, before the user is locked. `1` locks on the first exhausted challenge, as v1.6.0 did. Minimum `1` | `3` | No |
| `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` | Number of wrong codes a user may submit within `LOCKOUT_FAILED_ATTEMPT_WINDOW`, across all of the user's challenges and whether or not a challenge was exhausted, before the user is locked for `LOCKOUT_DURATION`. `0` disables this count. See [security](SECURITY.md#user-lockout) | `15` | No |
| `LOCKOUT_FAILED_ATTEMPT_WINDOW` | Window for `LOCKOUT_FAILED_ATTEMPT_THRESHOLD`, counted from the first wrong code and not extended by later ones; must be positive unless the threshold is `0` | `1h` | No |
| `RESEND_COOLDOWN` | Minimum interval between challenges for the same user and destination, and for the same destination regardless of user; must be positive | `60s` | No |
| `CODE_LENGTH` | Verification code length (digits, minimum `4`; must also fit `HERALD_MAX_BODY_BYTES` after the compact context-bound V2 envelope, currently 136 bytes) | `6` | No |
| `IDEMPOTENCY_KEY_TTL` | Pre-send claim and completed response TTL; unresolved sends use `IDEMPOTENCY_UNRESOLVED_TTL`; `0` = use `CHALLENGE_EXPIRY` | `0` | No |
| `IDEMPOTENCY_UNRESOLVED_TTL` | How long an idempotency key whose send was started but not completed keeps answering `409/delivery_unknown`. Must be positive and greater than `IDEMPOTENCY_KEY_TTL` (`CHALLENGE_EXPIRY` when that is `0`); otherwise startup fails. After it elapses the key can start a new request | `24h` | No |
| `ALLOWED_PURPOSES` | Allowed purposes, comma-separated (e.g. `login,reset,bind,stepup`) | `login` | No |
| `HERALD_VERIFY_V1_ENABLED` | Register `POST /v1/otp/verifications`, which has no expected user/purpose/channel binding. Set `false` once all callers use `POST /v2/otp/verifications`; the v1 route then returns 404 | `true` | No |

#### Rate limiting

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `RATE_LIMIT_PER_USER` | Challenges per user_id per hour | `10` | No |
| `RATE_LIMIT_PER_IP` | Challenges per IP per minute | `5` | No |
| `RATE_LIMIT_PER_DESTINATION` | Challenges per destination (email/phone) per hour | `10` | No |

#### Email channel

**Built-in SMTP** (used when `HERALD_SMTP_API_URL` is not set):

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SMTP_HOST` | SMTP server host | (empty) | When using built-in |
| `SMTP_PORT` | SMTP port | `587` | No |
| `SMTP_USER` | SMTP username | (empty) | No |
| `SMTP_PASSWORD` | SMTP password | (empty) | No |
| `SMTP_FROM` | From address | (empty) | Recommended |
| `PROVIDER_FAILURE_POLICY` | On send failure: `soft` (still create challenge) or `strict` (do not create) | `soft`; `strict` when `ENVIRONMENT=production` | No |

Production forbids `soft`. When the variable is unset or empty in production, Herald uses `strict` and logs `Production default: PROVIDER_FAILURE_POLICY=strict`; an explicit `soft` still fails startup.

Provider and TOTP base URLs (`HERALD_SMTP_API_URL`, `SMS_API_BASE_URL`, `HERALD_DINGTALK_API_URL`, `HERALD_TOTP_BASE_URL`) must use `https://` in production, where a plain `http://` URL fails startup. Other environments accept `http://` and log a warning.

**herald-smtp plugin** (when set, built-in SMTP is not used):

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_SMTP_API_URL` | [herald-smtp](https://github.com/soulteary/herald-smtp) base URL (e.g. `https://herald-smtp.internal:8084`) | (empty) | When using plugin |
| `HERALD_SMTP_API_KEY` | Must match herald-smtp `API_KEY` if set | (empty) | No |

#### SMS channel (HTTP API mode)

The implementation calls an external SMS HTTP API; Aliyun/etc. credentials are held by that gateway, not Herald.

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `SMS_PROVIDER` | Provider name (e.g. `aliyun`, `tencent`, `http`) for logging | (empty) | When using SMS |
| `SMS_API_BASE_URL` | SMS HTTP API base URL | (empty) | When using SMS |
| `SMS_API_KEY` | SMS API auth key if required by gateway | (empty) | As needed |

Both `SMS_PROVIDER` and `SMS_API_BASE_URL` are needed: the provider is registered only when `SMS_PROVIDER` is set, and startup fails if it is set without a usable base URL. A base URL without `SMS_PROVIDER` leaves the channel unconfigured and logs a warning.

#### Channels without a provider

At startup Herald logs one line listing the channels that have a provider and those that do not (`channels_with_provider`, `channels_without_provider`), and a warning when no channel has one. With `PROVIDER_FAILURE_POLICY=strict`, a create request for a channel without a provider returns `503/channel_not_configured` before any quota, cooldown or idempotency key is used. With `soft` the challenge is created and the response carries `delivery_status=failed`, as before.

#### DingTalk channel (herald-dingtalk plugin)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_DINGTALK_API_URL` | [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) base URL (e.g. `https://herald-dingtalk.internal:8083`) | (empty) | When using DingTalk |
| `HERALD_DINGTALK_API_KEY` | Must match herald-dingtalk `API_KEY` if set | (empty) | No |

#### TOTP (herald-totp proxy)

When enabled, Herald proxies TOTP (Authenticator) requests to [herald-totp](https://github.com/soulteary/herald-totp). TOTP routes are available under `/v1/totp/*` (status, verify, enroll/start, enroll/confirm, revoke).

TOTP calls use a 10-second timeout and verified TLS (TLS 1.2 or later). Production
requires HTTPS. Configure the final service URL: redirects are never followed,
including redirects to the same origin. Base URLs may contain a path prefix,
but must not contain user credentials, query parameters, or fragments.

`PROVIDER_MAX_RESPONSE_BYTES` also limits TOTP response bodies (default 1 MiB).
Responses must be complete JSON objects with HTTP 200. Redirects, other HTTP
statuses, oversized bodies, and malformed or interrupted responses return
`502/proxy_failed`; an upstream error body cannot establish successful
verification. Valid HTTP 200 verification failures remain `200/ok:false`, and
enrollment confirmation retains `400/invalid` for complete upstream HTTP 400
responses. API Key and HMAC wire formats are unchanged.

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_TOTP_ENABLED` | Enable TOTP proxy | `false` | No |
| `HERALD_TOTP_BASE_URL` | herald-totp service base URL (e.g. `https://herald-totp.internal:8085`) | (empty) | When TOTP enabled |
| `HERALD_TOTP_API_KEY` | API key for Herald to call herald-totp (if herald-totp requires it) | (empty) | No |
| `HERALD_TOTP_HMAC_SECRET` | HMAC secret for Herald to call herald-totp (if herald-totp uses HMAC) | (empty) | No |
| `HERALD_TOTP_HMAC_KEY_ID` | Explicit TOTP signing key ID; required when the backend accepts multiple keys | (empty; omit ID) | No |
| `HERALD_TOTP_HMAC_SERVICE` | TOTP signing service identity | (empty; retain SDK default `stargate`) | No |

#### TLS / mTLS

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `TLS_CERT_FILE` | Server certificate path | (empty) | When TLS enabled |
| `TLS_KEY_FILE` | Server private key path | (empty) | When TLS enabled |
| `TLS_CA_CERT_FILE` | Client CA cert for mTLS verification | (empty) | optional/require mode |
| `TLS_CLIENT_CA_FILE` | Alias for `TLS_CA_CERT_FILE` | (empty) | Optional |
| `CLIENT_CERT_MODE` | Client cert policy: `off`, `optional`, or `require` | `off` | No |
| `HERALD_HEALTHCHECK_TLS_CA_FILE` | CA used by the binary loopback HTTPS probe | (empty) | Private server CA |
| `HERALD_HEALTHCHECK_TLS_SERVER_NAME` | Server name verified by the loopback HTTPS probe | `localhost` | No |
| `HERALD_HEALTHCHECK_TLS_CLIENT_CERT_FILE` | Client certificate for the loopback probe | (empty) | Client cert required |
| `HERALD_HEALTHCHECK_TLS_CLIENT_KEY_FILE` | Client key for the loopback probe; must accompany its certificate | (empty) | Client cert required |

#### Startup warnings

Some settings are valid but probably not intended. Herald logs one warning for
each at startup and keeps starting. The `check` field names the warning.

| `check` | Logged when | Why it matters |
|---------|-------------|----------------|
| `config.rate_limit_ip` | `HERALD_TRUSTED_PROXY_HEADER` and `HERALD_TRUSTED_PROXIES` are unset | `RATE_LIMIT_PER_IP` counts the direct peer, so a gateway that calls Herald for many end users shares one quota among all of them |
| `config.listener_tls` | `ENVIRONMENT=production` and `TLS_CERT_FILE`/`TLS_KEY_FILE` are unset | Verification requests carry the code in their body, and with `REQUEST_AUTH_MODE=api_key` every request also carries the static key, which can be replayed if captured. If TLS is terminated by a proxy in front of Herald, keep the hop to Herald on a trusted local network |
| `config.code_length` | `CODE_LENGTH` is below `6` | Lengths `4` and `5` are accepted, but each digit removed makes a single guess ten times more likely to succeed while the attempt and rate limits stay the same |

#### Audit logging

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `AUDIT_ENABLED` | Enable audit | `true` | No |
| `AUDIT_MASK_DESTINATION` | Mask destination in audit | `true` in production unless explicitly set; otherwise `false` | No |
| `AUDIT_TTL` | Audit record TTL in Redis (e.g. `168h` = 7 days) | `168h` | No |
| `AUDIT_STORAGE_TYPE` | One backend: `database` (`db` alias), `file`, or `redis`; empty/`none` uses the provided Redis client | (empty) | No |
| `AUDIT_DATABASE_URL` | DB URL when `AUDIT_STORAGE_TYPE` is database/db | (empty) | No |
| `AUDIT_TABLE_NAME` | Audit table name | `audit_logs` | No |
| `AUDIT_FILE_PATH` | Audit file path when using file | (empty) | No |
| `AUDIT_WRITER_QUEUE_SIZE` | Wake-up hint queue size (records are stored on disk) | `1000` | No |
| `AUDIT_WRITER_WORKERS` | Audit writer workers | `2` | No |
| `AUDIT_SPOOL_DIR` | Durable local audit buffer directory | `./audit-spool` (container: `/var/lib/herald/audit`) | Writable when audit is enabled |
| `AUDIT_SPOOL_MAX_BYTES` | Maximum pending JSON payload bytes; filesystem overhead needs additional space | `67108864` | No |

Audit records are written to private files and fsynced before the API accepts
them. A full hint queue does not discard records. Backend writes are retried
from disk; graceful shutdown keeps pending files for replay on the next start.
Queries show delivered backend records, not the local backlog.

A pending file that is read but cannot be decoded is tried 5 times and then
renamed to `<id>.json.quarantined`. It stays in `AUDIT_SPOOL_DIR`, is not
retried or picked up again after a restart, and is excluded from
`AUDIT_SPOOL_MAX_BYTES`; see [monitoring](MONITORING.md#audit-delivery).
A file that cannot be read (an I/O error, no free file descriptor, a permission
problem) is not quarantined: it stays pending and is retried with a delay that
doubles from 1 second up to 5 minutes, until it can be read. Backend delivery
failures are not quarantined either: those records are retried until the
backend accepts them.

Mount `AUDIT_SPOOL_DIR` on persistent local storage, owned by UID/GID 10001 for
the container and preferably mode 0700. The checked-in Compose file mounts the
`audit_spool` volume there, including when the root filesystem is read-only.
Give each running instance its own directory/volume; an exclusive lock prevents
concurrent owners. Preserve this volume across restarts and upgrades. `docker
compose down -v` deletes it and must not be used while records remain pending.
The directory must support file and directory fsync and atomic rename.

An unavailable spool prevents startup. If the buffer fills or a later local
write fails, affected APIs return `503/audit_unavailable` rather than success.
Create requests stop before sending when their initial audit cannot be stored;
verification and revoke attempts are stored before changing challenge state.
A result record that fails after the message was accepted, the code was
consumed or the revoke completed does not change the response: the request
succeeds, `herald_audit_post_action_failure_total{operation}` is incremented and
one error line records the event. Alert on that counter, because those events
are missing from the audit trail.
Use the same idempotency key for uncertain send results rather than blindly
resending. Pending audit files have no automatic expiry and are bounded by the
configured byte limit; monitor disk space and delivery failures.

Delivery is **at least once**. A crash after the backend commits but before the
local file is removed can replay the record with the same `event_id`; consumers
should deduplicate by that ID if needed. Destination masking happens before
local persistence. Protect and back up the volume: filesystem or volume loss
is outside this guarantee. Backend audit TTL applies after delivery; it does
not evict the pending local buffer. `AUDIT_ENABLED=false` explicitly disables
these checks and audit recording.


#### Templates and observability

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `TEMPLATE_DIR` | Optional path to email/SMS template directory; see [message templates](#message-templates) | (empty) | No |
| `OTLP_ENABLED` | Enable OpenTelemetry | `false` | No |
| `OTLP_ENDPOINT` | OTLP endpoint (e.g. `http://localhost:4318`) | (empty) | When OTLP enabled |

#### Test and debug

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `HERALD_TEST_MODE` | Enable test code exposure only with `ENVIRONMENT=test`; forbidden in production | `false` | No |
| `HERALD_TEST_API_KEY` | Dedicated key for the test-code listener | (empty) | Test mode |
| `HERALD_TEST_LISTENER_ADDR` | Separate loopback-only test listener | `127.0.0.1:18082` | No |

### Test mode and debugging

Only when `ENVIRONMENT=test` and `HERALD_TEST_MODE=true`:

- The create-challenge response includes a `debug_code` field (plain verification code) so callers (e.g. Stargate with `DEBUG=true`) can display it on the login page.
- `GET /v1/test/code/:challenge_id` is mounted on the separate
  `HERALD_TEST_LISTENER_ADDR` listener (default `127.0.0.1:18082`), not the main
  listener. Supply `X-Test-Api-Key: $HERALD_TEST_API_KEY` (or Bearer authentication).

**Security:** Always set `HERALD_TEST_MODE=false` in production. Test mode exposes verification codes and must not be used in production environments.

### Message templates

Without `TEMPLATE_DIR` Herald sends built-in text. With it, Herald loads custom
templates once at startup from this layout:

```text
{TEMPLATE_DIR}/{locale}/{channel}/{purpose}.txt

templates/
├── en/
│   ├── email/login.txt
│   └── sms/login.txt
└── zh-CN/
    ├── email/login.txt
    └── sms/reset.txt
```

- `{locale}` is a language or locale tag such as `en`, `zh`, `zh-CN` or
  `pt_BR`; case and `-`/`_` do not matter. Templates are selected by language
  only: `zh-CN`, `zh_TW` and `zh` are the same language, and so is a request
  `locale` of any of them. If two directories name the same language, the
  first in name order is used and the other is reported at startup. A request
  with an unknown or empty locale uses `en`.
- `{channel}` is `email` or `sms`. DingTalk messages use the `sms` templates.
- `{purpose}` is a value from `ALLOWED_PURPOSES`, for example `login`. The
  file extension is ignored.

Files are Go `text/template` sources. The available fields are `{{.Code}}`,
`{{.ExpiresIn}}` (seconds), `{{.Purpose}}` and `{{.Locale}}`; any other field
is an error. For `email`, the first line is the subject and the rest is the
body; a one-line template is used as the body with the subject
`Verification Code`. A rendered message is limited to 16 KiB.

A file that cannot be read or parsed, sits at another depth, or names an
unrecognized locale or channel is not loaded. Herald logs one warning per file
(`check=templates`, message `template not loaded: ...`) and keeps starting. A
missing template, and a template that fails while rendering, fall back to the
built-in text, so a mistake changes the wording of a message but does not stop
delivery. Restart Herald after changing templates.

### Provider deployment versions

Pin `ghcr.io/soulteary/herald-dingtalk:1.2.8`,
`ghcr.io/soulteary/herald-smtp:1.2.9`, and
`ghcr.io/soulteary/herald-totp:2.0.4` (or verified immutable digests).
Herald v1.6.0 imports `github.com/soulteary/herald-totp/v2` v2.0.4. SMTP and
DingTalk retain their existing patch-level interfaces. TOTP keeps its `/v1` HTTP
routes and credential storage, but its HMAC protocol changes to operation-bound
v2 with nonce protection. Upgrade the TOTP service and Herald proxy together;
old HMAC signatures are not accepted. API-key-only mode remains available.
Preserve provider credentials, TOTP encryption keys and Redis data. Before
restoring traffic, check health/readiness and an authenticated operation using
the deployed mode. See [security requirements](SECURITY.md) and the
[pending challenge migration](MIGRATION_V2.md#pending-challenge-verification-protocol).
Real email and DingTalk sends have external effects; rehearse using isolated
destinations.

### Email channel (herald-smtp)

When `HERALD_SMTP_API_URL` is set, Herald does not use built-in SMTP. It forwards email sends to [herald-smtp](https://github.com/soulteary/herald-smtp) over HTTP. All SMTP credentials and sending logic live in herald-smtp; Herald does not store any SMTP credentials for the email channel in this mode.

- Set `HERALD_SMTP_API_URL` to the base URL of your herald-smtp service (e.g. `https://herald-smtp.internal:8084`).
- If herald-smtp is configured with `API_KEY`, set `HERALD_SMTP_API_KEY` to the same value so Herald can authenticate when calling herald-smtp.
- When `HERALD_SMTP_API_URL` is set, Herald ignores `SMTP_HOST` and related built-in SMTP settings (they are not used for the email channel).

### DingTalk channel (herald-dingtalk)

When `channel` is `dingtalk`, Herald does not send messages itself. It forwards the send to [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) over HTTP. All DingTalk credentials and business logic live in herald-dingtalk; Herald does not store any DingTalk credentials.

- Set `HERALD_DINGTALK_API_URL` to the base URL of your herald-dingtalk service (e.g. `https://herald-dingtalk.internal:8083`).
- If herald-dingtalk is configured with `API_KEY`, set `HERALD_DINGTALK_API_KEY` to the same value so Herald can authenticate when calling herald-dingtalk.

### TOTP (herald-totp)

When `HERALD_TOTP_ENABLED=true` and `HERALD_TOTP_BASE_URL` is set, Herald proxies TOTP (Authenticator) operations to [herald-totp](https://github.com/soulteary/herald-totp). Stargate (or other callers) can use a single Herald base URL for both OTP (SMS/email/DingTalk) and TOTP flows.

- Set `HERALD_TOTP_BASE_URL` to the base URL of your herald-totp service (e.g. `https://herald-totp.internal:8085`).
- If herald-totp requires API key or HMAC auth, set `HERALD_TOTP_API_KEY` or `HERALD_TOTP_HMAC_SECRET` accordingly.
- See [API.md](API.md#totp-proxy-optional) for TOTP endpoints (status, verify, enroll/start, enroll/confirm, revoke).

### Redis Configuration

#### Independent Redis Instance

For production deployments, it is **strongly recommended** to use a dedicated Redis instance or a separate database index for Herald to avoid key space conflicts with other services.

**Key Prefixes:**
- `otp2:ch:*` - Challenge data (TTL: challenge expiry time)
- `ratelimit:*` - Rate limiting counters (TTL: rate limit window)
- `otp:idem2:*` - Idempotency records (pending/completed: `IDEMPOTENCY_KEY_TTL`; unresolved sending: `IDEMPOTENCY_UNRESOLVED_TTL`)
- `otp:lock:v2:<digest>` - User lockout records (TTL: lockout duration). The digest is derived from the user ID with `HERALD_PII_PEPPER`
- `otp:lock:exhausted:<digest>` - Exhausted challenges counted towards a lockout (TTL: lockout duration from the first one)

**Unresolved sends:**

Use persistent Redis storage and a `noeviction` policy for idempotency records.
An acknowledged write must survive for as long as retries need to be suppressed;
Herald cannot prevent duplicates after Redis loses or evicts that write. Choose
Redis persistence and failover settings for the durability your deployment needs.
Do not flush the database or rotate `HERALD_IDEMPOTENCY_SECRET` while unresolved
records or retries remain: changing the derived key would bypass an old record.

A `sending` record expires after `IDEMPOTENCY_UNRESOLVED_TTL` (default `24h`)
and includes `challenge_id` for reconciliation. Until then the key answers
`409/delivery_unknown`. Monitor `delivery_unknown` idempotency outcomes and the
number of unresolved records. Check provider delivery logs using that challenge
ID (also the provider idempotency key) before an administrator resolves the
record. Do not delete these records early: the original request may still be
running, or its message may already have been delivered. Establish a separate,
explicit business decision for any replacement send. Once the TTL has elapsed
the record is removed and a request with the same key is treated as new, so
choose a value longer than any client retries a key. Within the TTL this is an
at-most-once send attempt for an unresolved key, not guaranteed delivery.

Records stored without a TTL by versions that predate
`IDEMPOTENCY_UNRESOLVED_TTL` are not modified and do not expire. Reconcile and
remove them as before.

**Capacity Planning:**

Measure payload size and Redis overhead under representative traffic. Include
challenges and activation indexes, expiring completed idempotency responses,
unresolved sends retained for `IDEMPOTENCY_UNRESOLVED_TTL`, rate/cooldown keys, nonces, and audit retention.
A peak-rate × challenge-TTL estimate alone does not bound total storage. Account
separately for persistent audit-buffer disk space and the process hash-work
budget. Use `MEMORY USAGE`, key counts and retained-record growth for sizing.

`REDIS_ADDR` names one Redis endpoint; Herald does not configure native Cluster or Sentinel discovery. Use an externally managed failover endpoint and preserve data. Multi-key activation and idempotency scripts are not Redis Cluster-compatible.

## Integration with Other Services (Optional)

Herald is designed to work independently and can be integrated with other services as needed. If you want to integrate Herald with other authentication or gateway services, you can configure the following:

These are caller-side examples, not additional Herald process variables.
For Herald API-key mode set `REQUEST_AUTH_MODE=api_key` and `API_KEY`.

**Example integration configuration:**
```bash
# Service URL where Herald is accessible
export HERALD_URL=http://herald:8082

# API key for service-to-service authentication
: "${API_KEY:?load the Herald server API key}"
export HERALD_API_KEY="$API_KEY"

# Enable Herald integration (if your service supports it)
export HERALD_ENABLED=true
```

**Note**: Herald can be used standalone without any external service dependencies. Integration with other services is optional and depends on your specific use case.

## Monitoring

### Prometheus Metrics

Herald exposes Prometheus metrics at the `/metrics` endpoint. The following metrics are available:

- `herald_otp_challenges_total{channel,purpose,result}` - Total number of OTP challenges created
- `herald_otp_sends_total{channel,provider,result}` - Total number of OTP sends via providers
- `herald_otp_verifications_total{result,reason}` - Total number of OTP verifications
- `herald_otp_send_duration_seconds{provider}` - Duration of OTP send operations (Histogram)
- `herald_rate_limit_hits_total{scope}` - Total number of rate limit hits (scope: user, ip, destination, resend_cooldown)
- `herald_redis_operation_duration_seconds{operation}` - Redis wrapper latency (rate-limit and cooldown operations; see MONITORING.md)

Configure a separate `HERALD_METRICS_API_KEY` on Herald and mount the same value at `/run/secrets/herald_metrics_key` in Prometheus. Do not reuse business API/HMAC credentials.

**Example Prometheus scrape configuration:**
```yaml
scrape_configs:
  - job_name: 'herald'
    static_configs:
      - targets: ['herald:8082']
    metrics_path: '/metrics'
    authorization:
      type: Bearer
      credentials_file: /run/secrets/herald_metrics_key
```

For detailed monitoring documentation, see [MONITORING.md](MONITORING.md).

## Security

- Use HMAC or API-key request authentication in production; add mTLS where appropriate. A client certificate does not replace the configured request credentials.
- Set strong API keys
- Use TLS/HTTPS in production
- Configure rate limits appropriately
- Monitor Redis for suspicious activity

## Provider failure diagnostics

Delivery failures are logged with a bounded `reason` field, not the upstream
message. Span status and exception events use the same safe reason. Do not
enable raw provider-body logging to investigate failures. Review historical
logs and exported spans for echoed sensitive data and rotate exposed credentials.
See [security](SECURITY.md#provider-failure-diagnostics) for compatibility details.

## OTP hash work admission

`HERALD_MAX_CONCURRENT_HASH_OPERATIONS` defaults to 4 and must be positive.
Earlier releases allowed up to 16 independent verifications while leaving
creation unbounded. The combined default changes burst admission, not code
validity. Creation and both OTP verification routes share this budget within the normal
single-handler service instance. It is local to each instance, not shared across
replicas. It does not change Argon2 parameters or apply to the TOTP proxy.

Each current Argon2 operation uses about 64 MiB. Size the budget for available
memory and CPU, including garbage collection and other process overhead; this
is not a hard RSS limit. Excess requests are rejected without queueing with
`503/hash_capacity_exceeded` and `Retry-After: 1` (`503/backend_unavailable` up to
v1.6.0). A sustained rate of this reason means the budget is too small for the
load; it is unrelated to Redis health. Creation admission precedes the
user-lock check and the rate and cooldown step; the pre-send idempotency claim is released when Redis is
available. Completed idempotency responses remain replayable while capacity is
full. Rejected verifications do not consume codes or increment their attempts.

A contended verification lock gets one immediate attempt and returns the existing
retryable `409/lock_contention` result rather than polling while holding this budget.
Provider delivery starts after the creation slot is released, so a slow provider
does not reserve hash capacity. Retry with the same idempotency key and a fresh
HMAC timestamp/nonce. Existing Redis records and client signatures are unchanged.

## HMAC identity upgrade

For HMAC deployments, pause creation traffic and drain in-flight creation requests,
then replace every old instance before resuming. Mixed versions can still split
implicit and explicit key-ID namespaces because old code does not check the new
canonical record. Keep Redis data, key mappings and `HERALD_IDEMPOTENCY_SECRET`
stable. Changing the derivation secret bypasses all existing idempotency records.

Legacy empty-key-ID records are retained. A matching service/client key returns
`409/idempotency_namespace_conflict`; this is an operator-reconciliation condition,
not permission to retry with another key. Sending records stored without a
TTL by earlier versions keep no TTL. Do not infer the original credential from the current default or
blindly replay/migrate old responses after key rotation. Authorized explicit-key records
continue normally when there is no conflicting legacy record.

Older releases cannot enforce this guard. Before rollback, keep creation disabled
until affected records and caller identities have been reconciled, and do not
resume implicit-key callers on old code. See [API](API.md#hmac-identity-and-legacy-idempotency-records).

### Single-secret key-ID allowlist

Only the implicit `default` identity is authorized by `HMAC_SECRET`. Previously,
any explicit ID selected that secret and created a separate nonce/idempotency
namespace. Unknown IDs now fail with `401/invalid_signature`, like a wrong
signature (releases up to v1.6.0 answered `401/invalid_key_id`).

Before resuming creation, inventory historical explicit IDs and reconcile their
unresolved sends using provider and audit evidence. Redis idempotency keys are
opaque and cannot be reassigned automatically. Preserve all records, the
idempotency derivation secret, and nonce prefix. Do not delete sending records
or switch caller IDs/client idempotency keys to force a retry.

To retain a known custom ID, explicitly provision it in `HERALD_HMAC_KEYS` and
keep the caller ID unchanged. If multiple IDs intentionally share one secret,
they remain distinct configured identities: callers must not rotate between them
to retry a send. Keep `default` explicitly provisioned if existing callers use
it, and configure the map default deliberately. A production key-map deployment
requires `HERALD_IDEMPOTENCY_SECRET`; set it to the previous effective value to
preserve records, rather than generating a new value.

Replace all instances while creation is paused. Older versions accept arbitrary
IDs again; during rollback keep creation disabled until affected identities and
records are reconciled and an external allowlist enforces the same restriction.
The earlier empty-key-ID guard remains in effect, and no record is migrated or
deleted by this fix.

## Production CORS origin lists

CORS is disabled when `HERALD_CORS_ALLOW_ORIGINS` is empty. In production, a
standalone `*` entry is rejected anywhere in the comma-separated list, including
`https://trusted.example,*` and entries surrounded by whitespace. Both startup
validation and router construction reject it before installing middleware.
Previously, only a configuration consisting entirely of `*` was rejected; a
mixed list enabled wildcard responses to unlisted origins.

Before upgrading, remove standalone wildcard entries and retain only the intended
origins, or clear the setting to disable CORS. Correct the configuration before
rolling back too: older releases accept mixed wildcard lists. Development/test
wildcard support, valid explicit lists, and request authentication are unchanged.
No Redis record or client signature migration is required. CORS origin policy
is a browser boundary; API Key/HMAC authentication is still required independently.

### Service permissions and metrics credentials

`HERALD_SERVICE_POLICIES` optionally binds each configured HMAC key ID to a
service and allowed actions. It requires HMAC v2/v3 authentication with legacy
v1 compatibility off (`HMAC_V1_ENABLED=false`); otherwise startup fails. See [service action policies](SECURITY.md#limit-service-actions)
for the policy format and the shared-user-namespace limitation.

Production metrics scraping now requires `HERALD_METRICS_API_KEY` (at least 32
bytes); without it `/metrics` returns 401. Configure a random key distinct from
business credentials and update Prometheus before deploying. See
[metrics authentication](MONITORING.md#metrics-authentication).

Unknown `ENVIRONMENT` and `REQUEST_AUTH_MODE` values are rejected before startup.
Supported authentication aliases are normalized through the same parser used by
the router. Explicit development and test environments retain their existing
policy; an unset environment still defaults to development.


## Patch upgrade from v1.4.1 to v1.4.2

Deploy herald-totp v2.0.2 before Herald v1.4.2. The TOTP service contains the
credential-snapshot commit guard; changing the SDK dependency alone does not
upgrade a running backend. Public SDK signatures, HMAC protocol, HTTP paths,
request payloads and success payloads remain unchanged.

Keep the existing Redis dataset, namespaces, pending enrollments, counters,
idempotency secrets, nonce/replay records, audit state and encryption key.
No additional data conversion is required for these patch versions. Replace all
TOTP instances to obtain the guard; mixed older instances retain the earlier
commit behavior. The guard does not cancel already committed operations.

Rolling Herald back to v1.4.1 retains compatible interfaces and does not roll
back the independent TOTP service. Rolling that backend back to v2.0.1 loses
the snapshot guard. Preserve state in either direction; do not clear records to
work around a rejected verification.


### TOTP proxy signing identity and key rotation

With `HERALD_TOTP_HMAC_SECRET` configured, set `HERALD_TOTP_HMAC_KEY_ID` to the backend's mapped key ID and optionally set `HERALD_TOTP_HMAC_SERVICE`. Both identifiers are limited to the SDK's 128-byte maximum and must not contain control characters or surrounding whitespace. Unset values preserve the existing omitted key ID and SDK service default `stargate`. API-key-only configuration is unchanged; HMAC proxy requests do not include the API key.

Earlier Herald versions cannot select an explicit TOTP signing key. An omitted ID works with a single mapped key, but cannot select between different keys during overlap. First deploy this change on every proxy with the existing key ID and secret; verify requests before adding the next backend key. Then configure proxies with the next ID and matching secret, drain old in-flight callers, and retire the old backend key only when no caller needs it. Do not reuse a nonce or clear replay state to validate rotation.

Rolling a proxy back restores omitted-ID/default-service behavior. Before rollback, reconcile the backend to a compatible single-key configuration matching that proxy's secret and expected service identity, or keep the proxy on the explicit-identity version. Preserve TOTP Redis data, encryption keys, nonce/quota state and Herald idempotency/audit state. This is optional proxy configuration support, not an HTTP protocol or credential-record migration.


## Patch upgrade from v1.4.2 to v1.4.3

Set explicit key IDs on every HMAC v2/v3 caller before upgrading a verifier
configured with `HERALD_HMAC_KEYS`, including a single-entry map. Configure
`Options.KeyID` (`WithKeyID`) in the Go SDK or send the signed `X-Key-Id` header in other clients.
The verifier now rejects omitted IDs with `401/key_id_required` before consuming
a nonce. Single-`HMAC_SECRET` configurations retain implicit `default` behavior.
Legacy v1 remains governed by its existing compatibility option and has no replay
protection; this change does not make it a rotation-safe alternative.

The SDK sends only HMAC credentials when its HMAC secret is set and does not
retry with an API key. For an API-key-only server, clear the caller's HMAC secret
and use its API key. Public Go method signatures, HTTP payloads and HMAC wire
formats remain unchanged. The TOTP proxy's optional key ID and service settings
are described above; deploy and verify explicit identities before key overlap.

Keep key IDs stable for retained keys, and preserve Redis namespaces, nonce
records, the idempotency secret, audit state and TOTP encryption/quota state.
No record conversion or namespace migration is required. Mixed old verifiers
still accept omitted mapped IDs; complete the verifier rollout to enforce the
new requirement. Rolling back a verifier restores omitted-ID acceptance.
Separately, rolling a client back to an older SDK can restore sending both
credentials. Do not change default IDs or rename mapped keys with the same secret
while accepting omitted-ID requests. A proxy rollback also requires the
compatible single-key/backend service configuration described above.


## Patch upgrade from v1.4.3 to v1.4.4

Deploy herald-totp v2.0.3 before Herald v1.4.4. The dependency refresh includes
secure-kit v2.1.4's Unicode masking fix and the matching audit/challenge/TOTP
modules. ASCII output, public SDK methods, HTTP success payloads, HMAC v2/v3
wire and explicit mapped-key identity rules are unchanged. Keep client key IDs
configured before upgrading a key-map verifier; API-key-only clients must keep
their HMAC secret empty.

Preserve Redis records and namespaces, pending challenge and enrollment state,
idempotency secrets and unresolved sends, nonce/quota/counter/backup-code state,
audit buffering and the original TOTP encryption key. No record conversion,
key migration or re-enrollment is required. Existing activation and snapshot
commit guards remain in place. An existing Herald v1.4.3 binary or image uses
its original compiled dependencies. Rollback does not undo stored diagnostic
values or completed requests, and does not roll back the independent TOTP service.

For a Go application rebuilt from source, restore its pre-upgrade `go.mod` and
`go.sum` snapshots and inspect `go list -m all` before building. Downgrading only
Herald may retain newer audit/challenge/TOTP/secure-kit versions through Go
minimal version selection or other direct dependencies. Verify the selected
versions. Retain the earlier proxy key-rotation and rollback requirements when
changing backend keys.

## Retry metadata upgrade and rollback (v1.5.0)

Use Herald v1.5.0 for the optional retry timing added to challenge-creation quota
and cooldown errors. SDK consumers can select `github.com/soulteary/herald@v1.5.0`
and call `CreateChallengeWithMetadata`. The existing `CreateChallenge` signature
and error type remain unchanged. This additive public SDK method is a minor release.

Deploying the SDK does not update the service. A new SDK can call an old service;
missing timing stays unknown. The metadata is an earliest retry opportunity, not
a promise that a later request will be admitted. Do not automatically retry OTP
creation or verification, reset quotas, or invent timing when it is absent.

Dependencies and configuration are unchanged from v1.4.4. Keep Redis namespaces,
pending challenges, idempotency/replay state, audit buffering and provider/TOTP
credentials during upgrade or rollback. No schema conversion or re-enrollment is
required. The TOTP service remains a separate deployment.

Rolling the service back to v1.4.4 removes optional timing fields but retains the
existing failure statuses and state rules. A client using the new SDK method must
revert that call before downgrading its module. Source rollback must restore the
appropriate source and application manifests; inspect effective module versions
before rebuilding. Older migration requirements above still apply.

## Upgrade from v1.5.2 to v1.6.0

Herald v1.6.0 adds optional configuration and one SDK option. Existing SDK
methods, HTTP success payloads, authentication modes, Redis records and
credential formats are unchanged. No data conversion, key migration or
re-enrollment is required. This is a minor release because it adds
configuration and public SDK API.

Check the configuration before deploying. These values were previously accepted
or silently replaced by a default and now stop startup:

- `RESEND_COOLDOWN=0`, in every environment. Use a positive duration.
- In production, a `REDIS_ADDR` that is set but invalid, and a
  `HERALD_HMAC_KEYS` that is set but malformed.
- In production, boolean variables that `strconv.ParseBool` does not accept
  (for example `yes` or `on`), and integer or duration variables that cannot be
  parsed (for example `HMAC_MAX_DRIFT=60` without a unit). Other environments
  log a warning and keep the default.

Other behaviour changes to plan for:

- SMS numbers written as `+86...`, `86...` and `0086...` now share one
  per-destination quota and resend cooldown. Counters recorded under the old key
  for a number written with `+` or `00` are not carried over, so such a number
  starts with a fresh quota. During a rolling upgrade, old and new instances
  count it separately.
- Trace spans carry `user_id_digest` instead of `user_id`. Update trace queries
  and dashboards that filter on `user_id`. Audit records are unchanged.
- Fiber v3.7.0 no longer exposes request trailers as headers. Send
  authentication and idempotency headers in the header section. A chunked body
  that ends before its last chunk is answered with 400 `request_error`.
- The startup warning for missing service credentials has new wording. Update
  log alerts that match the old text.

The new settings are opt-in. `REDIS_USERNAME`, `REDIS_TLS_ENABLED`,
`REDIS_TLS_CA_FILE` and `REDIS_TLS_SERVER_NAME` default to off or empty. Enable
them only after the Redis server accepts TLS or the ACL user. In production,
setting the CA file or server name without `REDIS_TLS_ENABLED=true`, or the
username without `REDIS_PASSWORD`, stops startup. `HERALD_VERIFY_V1_ENABLED`
defaults to `true`; set it to `false` only after every caller uses
`POST /v2/otp/verifications`. The Go SDK method `VerifyChallenge` calls v1; use
`VerifyChallengeV2` first.

SDK consumers selecting `github.com/soulteary/herald@v1.6.0` need Go 1.27.4 or
later. `WithClientIPHeader` forwards the end-user address only when it is set,
and Herald reads the header only from peers listed in `HERALD_TRUSTED_PROXIES`
with `HERALD_TRUSTED_PROXY_HEADER` configured. `TOTPRevoke` now returns an
error with reason `revoke_not_confirmed` when Herald answers 200 without
`ok: true`; handle it as a failed revocation. Deploying the SDK does not update
the service, and the new SDK works with an older service.

Herald imports `github.com/soulteary/herald-totp/v2` v2.0.4, a dependency-only
update of the client module. The TOTP service remains a separate deployment and
its protocol is unchanged from v2.0.3.

Rolling the service back to v1.5.2 retains Redis data, pending challenges,
idempotency and replay state, audit buffering and provider/TOTP credentials.
v1.5.2 ignores the Redis TLS and ACL username settings, so restore a Redis
endpoint it can reach before rolling back. It also registers the v1
verification route again and exports `user_id` on spans. SMS quotas recorded
under the canonical key are not read by v1.5.2 for numbers written with `+` or
`00`. A client using `WithClientIPHeader` must remove that call before
downgrading its module. For a Go application rebuilt from source, restore the
pre-upgrade `go.mod` and `go.sum` and inspect `go list -m all` before building.
Older migration requirements above still apply.

## Upgrade from v1.6.0

No data conversion is required. Rate-limit counters and the existing
user+destination cooldown keep their Redis key names, window and TTL handling,
so counters and cooldowns in progress continue to apply, and old and new
instances share them during a rolling upgrade.

- Challenge creation applies the user, IP and destination limits and the
  cooldowns in one Redis Lua call that reads and writes five keys. Herald
  connects to a single Redis endpoint; a proxy that routes keys to different
  shards must support multi-key scripts.
- A request rejected by a limit, a cooldown or `user_locked` no longer consumes
  quota. A locked user who is also over a limit now receives `403/user_locked`
  instead of `429`.
- A second cooldown key, `ratelimit:cooldown:dest:<digest>`, applies
  `RESEND_COOLDOWN` per destination regardless of `user_id`. Callers that
  legitimately send to one destination for several user IDs within the cooldown
  now receive `429/resend_cooldown`. Instances running v1.6.0 do not read this
  key, so the destination-only cooldown is fully effective once every instance
  is upgraded.
- `herald_redis_operation_duration_seconds` and
  `herald_redis_operations_total` report challenge creation limits under
  `operation="ratelimit_create"`. The `ratelimit_user`, `ratelimit_ip`,
  `ratelimit_dest` and `cooldown` values are no longer emitted by challenge
  creation; update dashboards that select them.
- Set `IDEMPOTENCY_UNRESOLVED_TTL` if `24h` is not suitable. It must exceed
  `IDEMPOTENCY_KEY_TTL` (`CHALLENGE_EXPIRY` when that is `0`); a deployment with
  a pending TTL of 24h or more must set it explicitly or startup fails. New
  unresolved `sending` records expire after it. Records already stored without a
  TTL are left as they are.
- Create, verify and revoke no longer return `503/audit_unavailable` when only
  the result record fails after the action completed. Alert on
  `herald_audit_post_action_failure_total`.

Rolling back to v1.6.0 keeps all Redis data. v1.6.0 ignores the
destination-only cooldown key and `IDEMPOTENCY_UNRESOLVED_TTL`; `sending`
records written by the newer version keep their TTL and still expire.

User lock:

- One exhausted challenge no longer locks its user. The lock needs
  `LOCKOUT_CHALLENGE_THRESHOLD` (default `3`) exhausted challenges within
  `LOCKOUT_DURATION`. Set `LOCKOUT_CHALLENGE_THRESHOLD=1` for the previous
  behaviour.
- Lock keys moved from `otp:lock:<user_id>` to `otp:lock:v2:<digest>`. No
  migration is needed: the old keys expire within `LOCKOUT_DURATION`. A lock in
  force at the moment of the upgrade is not carried over, and instances of the
  two versions do not share locks during a rolling upgrade.
- The digest depends on `HERALD_PII_PEPPER`. Rotating the pepper drops current
  locks and counts.
