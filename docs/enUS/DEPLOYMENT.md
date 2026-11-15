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
| `HERALD_CODE_HASH_KEY` | Secret that keys the digest of each code stored in Redis (HMAC-SHA-256). Empty keeps the unkeyed Argon2 hash. At least 32 bytes in every environment, or startup fails. The same value on every instance; see [Verification code hash key](#verification-code-hash-key) | (empty) | Recommended |
| `HERALD_CODE_HASH_VERIFY_ONLY` | With `HERALD_CODE_HASH_KEY` set: keep storing Argon2 hashes for new challenges while keyed digests are still verified. First stage of a rolling change of the key setting and of its rollback | `false` | No |
| `HERALD_CODE_HASH_KEY_PREVIOUS` | Comma-separated list of up to 4 further keys under which keyed digests are verified but never written. Used to change `HERALD_CODE_HASH_KEY` without refusing outstanding challenges; see [Changing the key](#changing-the-key). Each key at least 32 bytes in every environment, or startup fails. No effect without `HERALD_CODE_HASH_KEY` | (empty) | No |
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
| `SMTP_PORT` | SMTP port; the server must accept STARTTLS on it | `587` | No |
| `SMTP_USER` | SMTP username | (empty) | No |
| `SMTP_PASSWORD` | SMTP password | (empty) | No |
| `SMTP_FROM` | From address | (empty) | Recommended |
| `PROVIDER_FAILURE_POLICY` | On send failure: `soft` (still create challenge) or `strict` (do not create) | `soft`; `strict` when `ENVIRONMENT=production` | No |

The built-in SMTP provider always upgrades the connection with STARTTLS before it authenticates or sends, in every environment, and verifies the server certificate against `SMTP_HOST` (TLS 1.2 or later). There is no setting to turn this off. If the server does not offer STARTTLS, refuses it, or presents a certificate that is not trusted for `SMTP_HOST`, the send fails (`failure_category` `rejected` or `tls`) and neither the password nor the message is sent. Implicit TLS (the SMTPS port 465) and plain SMTP without TLS are not supported by the built-in provider.

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

A send that fails has already passed admission: the request's per-user, per-IP and per-destination quota stays spent in every case. What happens to the two resend cooldowns under `PROVIDER_FAILURE_POLICY=strict` depends on the failure:

- They are released, so a retry is not answered `429/resend_cooldown`, when the failure proves that no provider accepted the message. For the built-in SMTP provider these are the `failure_category` values `dns`, `connect`, `auth` and `validation`, and `tls` when the server certificate failed verification. For the HTTP providers (herald-smtp, herald-dingtalk, the SMS gateway) it is a failure to resolve, connect to or verify the certificate of the provider service, and only when no request was written to it.
- They stay in place for every other failure, because the message may have been accepted: a timeout or cancelled request, a connection that broke after it was established, an SMTP command the server rejected, any answer from an HTTP provider service, and any error that is not recognised. A retry for the same destination within `RESEND_COOLDOWN` then answers `429/resend_cooldown` although no delivery was confirmed.

The failure log line carries `cooldown_released` (`true` or `false`). Retries after a released cooldown still count against `RATE_LIMIT_PER_USER`, `RATE_LIMIT_PER_IP` and `RATE_LIMIT_PER_DESTINATION`. A retry that repeats the `Idempotency-Key` of the failed request is still answered `409/delivery_unknown`; use a new key. With `PROVIDER_FAILURE_POLICY=soft` the response is `200` with `delivery_status=failed` and the cooldown is kept. For the built-in SMTP provider the failure log line names where delivery stopped; see [troubleshooting](TROUBLESHOOTING.md#provider-send-failures).

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
| `config.test_mode` | `HERALD_TEST_MODE=true` while `ENVIRONMENT` is not `test` (production refuses to start instead) | The flag has no effect on its own: no `debug_code` is returned and the test-code endpoint is not started |
| `config.totp` | `HERALD_TOTP_ENABLED=true` and `HERALD_TOTP_BASE_URL` is empty | The TOTP proxy is not configured; every `/v1/totp` request answers `503/totp_not_configured` |
| `config.otlp` | `OTLP_ENABLED=true` and `OTLP_ENDPOINT` is empty | No trace exporter is configured, so no spans are exported |
| `config.code_hash` | `HERALD_CODE_HASH_VERIFY_ONLY=true` and `HERALD_CODE_HASH_KEY` is empty | The flag has no effect on its own: codes are stored with the unkeyed Argon2 hash and keyed digests cannot be verified |
| `config.code_hash_previous` | `HERALD_CODE_HASH_KEY_PREVIOUS` is set and `HERALD_CODE_HASH_KEY` is empty, or one of the listed keys equals `HERALD_CODE_HASH_KEY` | Without a primary key no keyed digest is verified under any key. A listed key equal to the primary key adds nothing; it usually means the two variables were not swapped as intended |

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
doubles from 1 second up to 5 minutes, until it can be read. A pending file
that was deleted from the directory is not retried: its record is lost, it is
reported once (an error log line and `herald_audit_spool_missing_total`) and it
stops counting towards `AUDIT_SPOOL_MAX_BYTES`. Backend delivery
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
| `OTLP_ENDPOINT` | OTLP/HTTP collector: `host:port` (plain HTTP, path `/v1/traces`), or `http://host:port[/path]` / `https://host:port[/path]`. `https` uses TLS and verifies the collector against the system roots, or against the file named by `OTEL_EXPORTER_OTLP_CERTIFICATE` when that is set; see [OTLP exporter environment variables](#otlp-exporter-environment-variables). A path other than `/` is used as the request path as given; without one spans are posted to `/v1/traces`. A value in any other form is reported at startup and nothing is exported | (empty) | When OTLP enabled |

#### OTLP exporter environment variables

Herald builds the OTLP/HTTP exporter with the host, path and scheme of `OTLP_ENDPOINT` and nothing else. The exporter library additionally reads the standard `OTEL_EXPORTER_OTLP_*` variables below from Herald's environment. Each has a trace-specific form with `TRACES_` after `OTLP_` (for example `OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE`); when both are set to a usable value, the trace-specific one is used. An empty value counts as unset.

| Variable | Effect |
|----------|--------|
| `OTEL_EXPORTER_OTLP_CERTIFICATE` | Path of a PEM file with the CA certificates used to verify the collector. The file replaces the system roots, it does not add to them: a collector whose certificate chains only to a public CA is refused unless that CA is in the file. This is how a collector behind a private CA is trusted. A file that cannot be read or holds no certificate is reported on standard error by the exporter and the system roots stay in use |
| `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE`, `OTEL_EXPORTER_OTLP_CLIENT_KEY` | PEM client certificate and private key presented to the collector (mutual TLS). Both must be set; one without the other is ignored |
| `OTEL_EXPORTER_OTLP_HEADERS` | Comma-separated `name=value` pairs added to every export request; values are URL-decoded. The trace-specific variable replaces the whole list. These headers are sent to the collector and usually carry a credential (for example `Authorization=Bearer%20<token>`): treat the variable as a secret, and note that with a `host:port` or `http://` `OTLP_ENDPOINT` they cross the network in clear text |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | Time limit for one export request, in milliseconds. Default 10000 |
| `OTEL_EXPORTER_OTLP_COMPRESSION` | `gzip` compresses request bodies; any other value means no compression (the default) |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/json` sends JSON instead of protobuf. `grpc` does not switch the transport: Herald exports over HTTP only |

Two limits apply:

- `OTEL_EXPORTER_OTLP_ENDPOINT` and `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` do not change where spans go. The host and path always come from `OTLP_ENDPOINT`.
- The certificate and client-certificate variables require an `https://` `OTLP_ENDPOINT`. With a `host:port` or `http://` endpoint and one of them set to a usable file, the exporter is not created: Herald logs `Failed to initialize OpenTelemetry tracing` and exports nothing.

The scheme always comes from `OTLP_ENDPOINT` as well. `OTEL_EXPORTER_OTLP_INSECURE`, `OTEL_EXPORTER_OTLP_TRACES_INSECURE` and an `http://` value of either `..._ENDPOINT` variable do not make an `https://` `OTLP_ENDPOINT` export in plain HTTP, and an `https://` value does not make a `host:port` or `http://` endpoint use TLS.

The exporter also follows `HTTP_PROXY`, `HTTPS_PROXY` and `NO_PROXY`.

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
`Verification Code`. Files may use LF or CRLF line endings. An email template
whose subject line still contains a carriage return (one that is not part of
the line ending) or a NUL character is not loaded, because the SMTP provider
refuses such a subject. A rendered message is limited to 16 KiB.

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
Herald v1.8.0 imports `github.com/soulteary/herald-totp/v2` v2.0.4. SMTP and
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
- `otp:lock:failed:<digest>` - Wrong codes counted towards a lockout across challenges (TTL: `LOCKOUT_FAILED_ATTEMPT_WINDOW` from the first one)

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

## Verification code hash key

By default Herald stores an Argon2 hash of each verification code in the
challenge record (`otp2:ch:*`). Argon2 takes no secret, and a 6-digit code has
10^6 candidates, so anyone who can read the record while the challenge is
outstanding (Redis access, a replica, a snapshot or a backup) can find the code
by trying every candidate offline.

`HERALD_CODE_HASH_KEY` replaces the stored value for new challenges with
`hmac-sha256$v1$<digest>`, an HMAC-SHA-256 over the challenge ID and the code
under that key. Without the key the stored value cannot be tested against
candidate codes. This holds only while the key is unknown to whoever reads
Redis: with both the key and a stored value the code is recovered at once.

- Generate the key once, for example `openssl rand -hex 32`, keep it in a
  secret manager and inject the same value into every instance. Do not store it
  in the Redis instance that holds the challenges or in anything backed up with
  it. Do not reuse another secret for it.
- The key must be at least 32 bytes. This is checked in every environment, not
  only in production: a shorter value stops startup with an error that names
  `HERALD_CODE_HASH_KEY`. There is no `HERALD_CODE_HASH_KEY_FILE`; like the
  other secrets it is read from the environment only.
- Herald never logs the key. The `Configuration initialized` startup line
  reports the scheme in use as `code_hash`: `argon2`, `hmac-sha256`, or
  `argon2 (hmac-sha256 verify-only)`, and the number of keys read from
  `HERALD_CODE_HASH_KEY_PREVIOUS` as `code_hash_previous_keys`.
- Attempt counting, user lockout, rate limits and expiry are unchanged.
  Creation and verification still pass through
  `HERALD_MAX_CONCURRENT_HASH_OPERATIONS`; computing a keyed digest does not
  use the memory an Argon2 operation needs.

### What happens to outstanding challenges

A challenge keeps the stored value it was created with until it is verified or
expires, at most `CHALLENGE_EXPIRY` (default `5m`) after creation.

| Change | Challenges created before the change |
|--------|--------------------------------------|
| Key set for the first time | Keep verifying. A stored Argon2 hash is always compared with Argon2 |
| Key removed, or an instance without the key (including v1.7.0 and earlier) receives the request | The correct code is answered `401/invalid` like a wrong code and consumes an attempt, until the challenge expires |
| Key changed, or instances hold different keys, and the verifying instance does not list the creating key in `HERALD_CODE_HASH_KEY_PREVIOUS` | Same: the correct code is answered `401/invalid` and consumes an attempt, until the challenge expires |
| Key changed, and the verifying instance lists the creating key in `HERALD_CODE_HASH_KEY_PREVIOUS` | Keep verifying |

A code refused this way is counted like any wrong code: it reduces
`remaining_attempts` and counts towards `LOCKOUT_FAILED_ATTEMPT_THRESHOLD`. The
user has to request a new code. Herald writes with one key,
`HERALD_CODE_HASH_KEY`, and verifies with that key and the keys listed in
`HERALD_CODE_HASH_KEY_PREVIOUS`.

### Enabling on one instance

Set `HERALD_CODE_HASH_KEY` and restart. Challenges created before the restart
keep verifying, and new ones are stored with the keyed digest.

### Enabling on several instances

During a rolling restart an instance that does not have the key yet (or still
runs v1.7.0 or earlier) cannot verify a challenge created by an instance that
has it. To avoid refused codes, change in two stages:

1. Deploy this release to every instance with `HERALD_CODE_HASH_KEY` set and
   `HERALD_CODE_HASH_VERIFY_ONLY=true`. New challenges are still stored with
   Argon2, and every instance can verify both forms.
2. When all instances run that way, remove `HERALD_CODE_HASH_VERIFY_ONLY` and
   restart them. New challenges are stored with the keyed digest.

Setting the key on all instances at once without the first stage is also
possible if refused codes for up to `CHALLENGE_EXPIRY` during the restart are
acceptable.

### Changing the key

`HERALD_CODE_HASH_KEY_PREVIOUS` lists keys that are used to verify keyed
digests and never to write them. The stored value does not record which key
wrote it, so Herald compares a submitted code under `HERALD_CODE_HASH_KEY` and
under every listed key; a code that matches under none is a wrong code. This
costs one HMAC-SHA-256 per key on each verification.

- The value is a comma-separated list of at most 4 keys. Whitespace around an
  item is removed and empty items are ignored. Each key must be at least 32
  bytes; a shorter one, or a fifth one, stops startup in every environment
  with an error that gives the position and length and never the key.
- A key that contains a comma, or begins or ends with whitespace, cannot be
  listed. To replace such a key, use the verify-only procedure under
  [Removing the key or rolling back](#removing-the-key-or-rolling-back).
- A listed key is as sensitive as `HERALD_CODE_HASH_KEY` for as long as
  challenges written under it are outstanding. Keep it out of Redis as well,
  and remove it when the procedure below says so.
- Every instance must run a release that has this variable. v1.8.0 and
  earlier ignore it, so upgrade all instances before starting.

Below, `A` is the key in use and `B` the new key.

**One instance, or all instances restarted together**

1. Set `HERALD_CODE_HASH_KEY=B` and `HERALD_CODE_HASH_KEY_PREVIOUS=A`, and
   restart. New challenges are written under `B`; challenges written under `A`
   keep verifying.
2. At least `CHALLENGE_EXPIRY` after the restart, remove
   `HERALD_CODE_HASH_KEY_PREVIOUS` and restart.

**Several instances restarted one at a time**

A verification request can reach any instance, not the one that created the
challenge. If one instance starts writing under `B` while another still knows
only `A`, that other instance answers the correct code `401/invalid` and
consumes an attempt. Listing `A` as previous on the new instance does not help
here: it lets the new instance verify old challenges, not the old instance
verify new ones. Every instance must therefore be able to verify `B` before
any instance writes with it, which makes `B` the "previous" key in the first
step:

1. On every instance set `HERALD_CODE_HASH_KEY_PREVIOUS=B` and keep
   `HERALD_CODE_HASH_KEY=A`; restart them one at a time. Nothing changes in
   what is stored. Wait until every instance runs this way (each reports
   `code_hash_previous_keys: 1` at startup).
2. On every instance set `HERALD_CODE_HASH_KEY=B` and
   `HERALD_CODE_HASH_KEY_PREVIOUS=A`; restart them one at a time. Restarted
   instances write under `B`, which the instances still in step 1 verify
   through their previous key; the instances still in step 1 write under `A`,
   which the restarted ones verify through theirs.
3. Wait at least `CHALLENGE_EXPIRY` after the last instance was restarted in
   step 2. No challenge written under `A` is outstanding after that.
4. Remove `HERALD_CODE_HASH_KEY_PREVIOUS` from every instance and restart them
   one at a time. `A` can be destroyed.

Do not combine steps 1 and 2, and do not start step 4 before the wait in step
3 has passed: either shortcut produces the refusals described under
[What happens to outstanding challenges](#what-happens-to-outstanding-challenges)
for up to `CHALLENGE_EXPIRY`.

To go back during step 2, return every instance to the step 1 settings
(`HERALD_CODE_HASH_KEY=A`, `HERALD_CODE_HASH_KEY_PREVIOUS=B`), one at a time,
and remove `B` no sooner than `CHALLENGE_EXPIRY` after the last instance
stopped writing with it. Going back from step 1 needs no wait: nothing was
written under `B`.

If `A` is being replaced because it may be known to someone else, the first
step delays the switch by one rolling restart. Restarting all instances
together with the single-step settings is faster. In both cases challenges
written under `A` stay searchable by whoever holds `A` and the stored value
until they expire; listing `A` as previous does not change that.

`HERALD_CODE_HASH_VERIFY_ONLY=true` can be combined with a list: new challenges
are stored with Argon2 and keyed digests are verified under every key.

### Removing the key or rolling back

1. Set `HERALD_CODE_HASH_VERIFY_ONLY=true` on every instance, keeping the
   current key, and restart.
2. Wait at least `CHALLENGE_EXPIRY` after the last instance restarted. No keyed
   challenge is outstanding after that.
3. Remove `HERALD_CODE_HASH_KEY` and `HERALD_CODE_HASH_KEY_PREVIOUS` on every
   instance, or downgrade to v1.7.0.

The same steps replace a key that cannot be listed in
`HERALD_CODE_HASH_KEY_PREVIOUS`: in step 3 set the new key instead of removing
it, and remove `HERALD_CODE_HASH_VERIFY_ONLY` once every instance has the new
key.

Skipping these steps does no lasting harm: outstanding keyed challenges are
refused as described above and expire within `CHALLENGE_EXPIRY`. No Redis data
needs to be changed or deleted in either direction.

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

## Upgrade from v1.7.0 to v1.8.0

Herald v1.8.0 adds `HERALD_CODE_HASH_KEY` and `HERALD_CODE_HASH_VERIFY_ONLY`
and moves `challenge-kit` to v1.13.0. HTTP routes, payloads, error reasons,
authentication modes, metrics, the audit spool file format and the Go SDK
(`pkg/herald`) are unchanged. `herald-totp/v2` stays at v2.0.4 and the
provider services do not need to be upgraded. No data conversion or key
migration is required. This is a minor release because it adds configuration.

With `HERALD_CODE_HASH_KEY` unset, v1.8.0 stores and verifies codes exactly as
v1.7.0 does, and v1.7.0 and v1.8.0 instances can serve the same Redis in any
order during a rolling upgrade or rollback.

Check the configuration before deploying:

- Environment variables named `HERALD_CODE_HASH_KEY` or
  `HERALD_CODE_HASH_VERIFY_ONLY` were ignored by v1.7.0 and take effect now.
  Herald refuses to start in every environment when `HERALD_CODE_HASH_KEY` is
  set and shorter than 32 bytes, or when `HERALD_CODE_HASH_VERIFY_ONLY` is not
  a boolean.
- The `Configuration initialized` log line gains the field `code_hash`.

Enabling the key is a separate step after every instance runs v1.8.0. Follow
[Verification code hash key](#verification-code-hash-key): on several
instances, first run all of them with the key and
`HERALD_CODE_HASH_VERIFY_ONLY=true`, then remove
`HERALD_CODE_HASH_VERIFY_ONLY`. An instance without the key, with a different
key, or running v1.7.0 answers the correct code of a keyed challenge
`401/invalid` and consumes an attempt until the challenge expires.

Rollback to v1.7.0:

- If `HERALD_CODE_HASH_KEY` was never set, replace the image; nothing else is
  needed.
- If it is set, first set `HERALD_CODE_HASH_VERIFY_ONLY=true` on every
  instance and wait `CHALLENGE_EXPIRY` (default `5m`) so that no keyed
  challenge is outstanding, then downgrade. v1.7.0 ignores both variables.
  Skipping the wait only causes the refusals described above for up to
  `CHALLENGE_EXPIRY`.
- No Redis data has to be changed or deleted in either direction.

The Go SDK has no changes in this release. An application that selects
`github.com/soulteary/herald@v1.8.0` works with a v1.7.0 service and the
reverse.

## Upgrade from v1.6.0 to v1.7.0

Herald v1.7.0 changes when a user is locked, how challenge creation applies its
limits and cooldowns, several error reasons, and metric, span and log output.
HTTP routes, success payloads, authentication modes, challenge and idempotency
record formats, the audit spool file format and the Go SDK (`pkg/herald`) are
unchanged. No data conversion, key migration or re-enrollment is required.
`challenge-kit` moves to v1.12.0; `herald-totp/v2` stays at v2.0.4 and the
provider services do not need to be upgraded. This is a minor release because
it adds configuration.

Check the configuration before deploying. These values were accepted by v1.6.0
and now stop startup in every environment:

- `IDEMPOTENCY_UNRESOLVED_TTL` (new, default `24h`) must be positive and
  greater than `IDEMPOTENCY_KEY_TTL` (`CHALLENGE_EXPIRY` when that is `0`). A
  deployment whose `IDEMPOTENCY_KEY_TTL` is 24h or longer, or that leaves it at
  `0` with a `CHALLENGE_EXPIRY` of 24h or longer, must set
  `IDEMPOTENCY_UNRESOLVED_TTL` to a larger value before upgrading.
- The new lock settings have their own bounds: `LOCKOUT_CHALLENGE_THRESHOLD`
  must be at least `1`, `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` must not be
  negative, and `LOCKOUT_FAILED_ATTEMPT_WINDOW` must be positive unless the
  threshold is `0`. They did not exist in v1.6.0, so this matters only if the
  environment already defines them.

One value that stopped startup is now accepted: in production an unset or empty
`PROVIDER_FAILURE_POLICY` means `strict`. v1.6.0 refused to start without the
variable. An explicit `soft` is still refused in production.

Upgrade checklist. Each item names what changed, what to check and what to do:

- User lock threshold. One exhausted challenge (`MAX_ATTEMPTS` wrong codes) no
  longer locks its user. The lock needs `LOCKOUT_CHALLENGE_THRESHOLD` (default
  `3`) exhausted challenges within `LOCKOUT_DURATION`. The exhausted challenge
  itself still answers `locked`. Set `LOCKOUT_CHALLENGE_THRESHOLD=1` to keep the
  v1.6.0 behaviour.
- Lock on wrong codes across challenges. A user is also locked after
  `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` (default `15`) wrong codes within
  `LOCKOUT_FAILED_ATTEMPT_WINDOW` (default `1h`), whichever challenges they
  were submitted against. v1.6.0 had no such count. Check that the defaults fit
  callers that legitimately retry often; `0` disables the count. A verification
  whose wrong code cannot be counted because Redis failed is answered 503.
- Lock keys. Locks moved from `otp:lock:<user_id>` to `otp:lock:v2:<digest>`.
  The new keys `otp:lock:exhausted:<digest>` (a set of exhausted challenge IDs)
  and `otp:lock:failed:<digest>` (the wrong-code counter) are added. The digest
  depends on `HERALD_PII_PEPPER`; rotating the pepper drops current locks and
  counts. Old lock keys are no longer read and expire within
  `LOCKOUT_DURATION`. A lock in force at the moment of the upgrade is not
  carried over.
- Creation limits. Challenge creation checks the user lock first, then applies
  the user, IP and destination limits and the cooldowns in one Redis Lua call
  that reads and writes five keys. Herald connects to a single Redis endpoint;
  a proxy that routes keys to different shards must support multi-key scripts.
  A request rejected by a limit, a cooldown or `user_locked` no longer consumes
  quota. A locked user who is also over a limit now receives `403/user_locked`
  instead of `429`. Callers that used the `429` to pace retries must handle the
  `403`.
- Destination cooldown. A second cooldown key,
  `ratelimit:cooldown:dest:<digest>`, applies `RESEND_COOLDOWN` per destination
  regardless of `user_id`. Callers that legitimately send to one destination for
  several user IDs within the cooldown now receive `429/resend_cooldown`.
- Cooldown release. Under `PROVIDER_FAILURE_POLICY=strict`, a create answered
  `500/send_failed` releases the resend cooldowns when the failure proves that
  no provider accepted the message (see
  [channels without a provider](#channels-without-a-provider)), so an immediate
  retry is no longer answered `429/resend_cooldown`. The quotas stay spent. To
  make this possible the cooldown keys hold a random token instead of `1`; only
  the existence and TTL of a cooldown key are read, by both versions.
- Redis permissions. If the Redis ACL user is restricted by command, it needs
  `EVAL` and, for the scripts, `GET`, `SET`, `DEL`, `EXISTS`, `INCR`, `PTTL`,
  `PEXPIRE`, `SADD` and `SCARD`. If it is restricted by key pattern, the
  existing `ratelimit:*` and `otp:lock:*` patterns cover the new keys. Challenge
  creation no longer sends `PING`.
- New and changed error responses. Update clients and alerts that match on
  them:
  - `400/invalid_challenge_id` for a `challenge_id` outside 1-64 bytes of
    `A-Za-z0-9_-`, on both verification routes and on revoke. IDs issued by
    Herald always pass.
  - `503/channel_not_configured` for a create request on a channel without a
    provider under the strict policy, before any quota, cooldown or
    idempotency key is used. Read the provider coverage line Herald logs at
    startup and configure the missing providers.
  - `503/hash_capacity_exceeded` when every hash slot is busy, on creation and
    verification. v1.6.0 answered `503/backend_unavailable`; alerts that treat
    `backend_unavailable` as a Redis outage no longer see this case.
  - `429/rate_limit_exceeded` carries a new `limit` field (`user`, `ip` or
    `destination`). `429/resend_cooldown` has no such field.
  - An unknown HMAC key ID is answered `401/invalid_signature` on the v2/v3
    schemes and on the legacy v1 scheme. The reason `invalid_key_id` is no
    longer returned; use `herald_auth_failure_total{reason="invalid_key_id"}`
    to tell the two cases apart.
  - Create, verify and revoke no longer return `503/audit_unavailable` when
    only the result record fails after the action completed. Alert on
    `herald_audit_post_action_failure_total` instead.
  - Outside production, an HMAC request whose nonce cannot be recorded is
    answered `503/nonce_store_unavailable`, as in production. It was let
    through before.
- Unresolved idempotency records. A `sending` record now expires after
  `IDEMPOTENCY_UNRESOLVED_TTL`. Until then the key answers
  `409/delivery_unknown` as before; afterwards the same key starts a new
  request. Records stored without a TTL by earlier versions are left as they
  are.
- Metrics. `herald_redis_operation_duration_seconds` and
  `herald_redis_operations_total` report the creation limits under
  `operation="ratelimit_create"`. The values `ratelimit_user`, `ratelimit_ip`,
  `ratelimit_dest` and `cooldown` are no longer emitted by challenge creation;
  update dashboards that select them. `operation="cooldown_release"` is new,
  as are the counters `herald_audit_post_action_failure_total{operation}`,
  `herald_audit_quarantined_total`, `herald_audit_spool_missing_total`,
  `herald_auth_failure_total{reason}` and `herald_failed_attempt_lock_total`.
- Traces. Request spans are named `<METHOD> <route template>` (for example
  `POST /v1/otp/challenges`) instead of `/`. `http.target`, the client address
  (`http.remote_addr`) and the masked `destination` attribute are no longer
  exported; the creation span carries `destination_digest`. A request with a
  W3C `traceparent` header now joins the caller's trace. Update trace queries
  and dashboards that use the old name or attributes.
- OTLP endpoint. `OTLP_ENDPOINT` accepts `http://` and `https://` URLs in
  addition to `host:port`. v1.6.0 exported nothing for the URL form, so a
  deployment configured that way starts exporting spans after the upgrade;
  check that the collector is reachable and sized for it. The scheme always
  comes from `OTLP_ENDPOINT`: a deployment that reached a plain-HTTP collector
  through an `https://` endpoint with `OTEL_EXPORTER_OTLP_INSECURE`,
  `OTEL_EXPORTER_OTLP_TRACES_INSECURE` or an `http://`
  `OTEL_EXPORTER_OTLP_ENDPOINT` must set `OTLP_ENDPOINT` to `http://` or
  `host:port`. `OpenTelemetry tracing initialized` is logged only when an
  exporter was set up.
- Startup log. New warnings carry a `check` field: `config.environment`
  (`ENVIRONMENT` unset; a later release will refuse to start without it),
  `config.rate_limit_ip` (per-IP limit without `HERALD_TRUSTED_PROXY_HEADER`
  and `HERALD_TRUSTED_PROXIES`), `config.listener_tls` (production listener
  without TLS), `config.code_length` (`CODE_LENGTH` below 6),
  `config.test_mode`, `config.totp`, `config.otlp`, `config.otlp_endpoint` and
  `config.secrets`. None of them stops startup. Adjust log alerts that fire on
  any warning during startup.
- Delivery failure log. The failure line for the built-in SMTP provider carries
  `failure_category` and, when the server sent one, `smtp_code`; failed sends
  under the strict policy carry `cooldown_released`.
- Audit spool. A pending file that cannot be decoded 5 times in a row is
  renamed to `<id>.json.quarantined` and no longer retried; a file that cannot
  be read is retried with a delay of up to 5 minutes; a pending file deleted
  from `AUDIT_SPOOL_DIR` is dropped and logged once. Alert on
  `herald_audit_quarantined_total` and `herald_audit_spool_missing_total`.
  Concurrent records share spool syncs; each record is still acknowledged only
  after its file and directory entry are synced. The file format is unchanged.
- Templates. Custom templates in region-tagged directories (for example
  `zh-CN`) are now selected; v1.6.0 sent the built-in text for them. Email
  templates saved with CRLF line endings are now sent; with v1.6.0 every
  message built from such a template failed. Review the files under
  `TEMPLATE_DIR` before upgrading, because their content starts to be sent. A
  subject line that still contains a carriage return or NUL is reported at
  startup and the template is not loaded, as are template files that cannot be
  loaded or can never be selected.
- Secrets outside production. The fixed fallback secret is gone. A deployment
  that sets none of `HERALD_IDEMPOTENCY_SECRET`, `HMAC_SECRET` and `API_KEY`
  (possible only outside production) derives the secret from
  `HERALD_HMAC_KEYS`, or generates a random one at each start when there is no
  key map. Idempotency and nonce records keyed by the old value are no longer
  found, and without any secret material they are not shared between instances
  or kept across restarts. Set `HERALD_IDEMPOTENCY_SECRET` to avoid this.
  Deployments that set a secret, and production, are unchanged.

Rolling upgrade. Replicas of both versions can serve traffic at the same time
and no order between replicas is required. Apply the configuration changes
above first, then replace all replicas and keep the mixed period short:

- Shared between versions: challenges, rate-limit counters, the existing
  user+destination cooldown, idempotency and nonce records. Counter and
  cooldown key names, windows and TTLs are unchanged. A cooldown key written by
  v1.7.0 holds a random token and one written by v1.6.0 holds `1`; both
  versions honour either, and v1.7.0 releases only a cooldown that still holds
  its own token.
- Not shared: user locks. v1.6.0 instances read and write
  `otp:lock:<user_id>`, v1.7.0 instances `otp:lock:v2:<digest>`, so a user
  locked through one version can still create and verify challenges through
  the other. Exhausted challenges and wrong codes handled by a v1.6.0 instance
  are not counted towards the v1.7.0 thresholds.
- v1.6.0 instances do not read `ratelimit:cooldown:dest:<digest>`, so the
  destination-only cooldown is complete once every instance is upgraded.
- While both versions run, clients see either set of error reasons and the
  Redis metrics carry either set of `operation` values, depending on the
  instance that served the request.

Rolling back to v1.6.0 keeps all Redis data and the audit spool. Nothing has to
be deleted:

- Before starting v1.6.0 in production, set `PROVIDER_FAILURE_POLICY=strict`
  explicitly; v1.6.0 refuses to start when it is unset. v1.6.0 ignores the new
  `LOCKOUT_*` settings and `IDEMPOTENCY_UNRESOLVED_TTL`. It exports no spans
  for an `OTLP_ENDPOINT` written as a URL; use `host:port` for a plain-HTTP
  collector.
- v1.6.0 does not read `otp:lock:v2:<digest>`, `otp:lock:exhausted:<digest>`,
  `otp:lock:failed:<digest>` or `ratelimit:cooldown:dest:<digest>`. These keys
  expire on their own (`LOCKOUT_DURATION`, `LOCKOUT_FAILED_ATTEMPT_WINDOW` and
  `RESEND_COOLDOWN`). Locks set by v1.7.0 therefore stop applying at the
  rollback, and v1.6.0 again locks a user on the first exhausted challenge
  under `otp:lock:<user_id>`.
- Cooldown keys that hold a token are read by v1.6.0 as cooldowns in force
  until they expire. Rate-limit counters and pending challenges continue to
  apply.
- `sending` idempotency records written by v1.7.0 keep their TTL and still
  expire. Records v1.6.0 writes have no TTL again.
- Pending audit spool files written by v1.7.0 are delivered by v1.6.0.
  `<id>.json.quarantined` files are ignored by v1.6.0 and stay in
  `AUDIT_SPOOL_DIR`; v1.6.0 again retries an undecodable or deleted pending
  file every second.
- Error reasons, metric labels, span names and attributes, and template
  selection return to the v1.6.0 behaviour. Outside production without a
  configured secret, v1.6.0 uses the fixed fallback secret again and does not
  find records keyed by v1.7.0.

The Go SDK has no changes in this release. An application that selects
`github.com/soulteary/herald@v1.7.0` works with a v1.6.0 service and the
reverse, apart from the changed error reasons listed above. Older migration
requirements above still apply.
