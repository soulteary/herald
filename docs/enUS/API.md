# Herald API Documentation

Herald is a verification code and OTP service that handles sending verification codes via SMS, email, and DingTalk (email via built-in SMTP or [herald-smtp](https://github.com/soulteary/herald-smtp) when `HERALD_SMTP_API_URL` is set, DingTalk via [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)), with built-in rate limiting and security controls.

## Base URL

```
http://localhost:8082
```

## Authentication

Request authentication and client-certificate policy are independent:

- `REQUEST_AUTH_MODE=hmac_v2` (default) selects HMAC authentication (the configuration name is retained); challenge creation requires v3.
- `REQUEST_AUTH_MODE=api_key` requires `X-API-Key` or `Authorization: Bearer …`.
- `REQUEST_AUTH_MODE=none` is for development only and is rejected in production.
- `CLIENT_CERT_MODE=off|optional|require` controls TLS client certificates. A verified certificate does not bypass the selected request-auth mode.

### HMAC v3

Send `X-Signature-Version: v3`, `X-Signature`, `X-Timestamp`, `X-Nonce`, `X-Service`, and `X-Key-Id` whenever `HERALD_HMAC_KEYS` is configured, including a single-entry map.

`X-Service` and `X-Key-Id` must not contain the reserved text `|kid:`. Such
values return `401/invalid_auth_headers` before authentication, nonce use, or
business writes, in every request-auth mode (including legacy v1 fallback).
The delimiter separates the service and key components of the idempotency
namespace. Ordinary namespace encodings and existing Redis records remain
unchanged. If an integration used the reserved text, update its service label
or configured key ID; reconcile unresolved sends before changing their
namespace or retrying them with a new idempotency key.

The signed canonical value is newline-delimited:

```text
HERALD-HMAC-V3
METHOD
/path
raw=query
unix_timestamp
single_use_nonce
service_name
key_id
sha256_hex_of_idempotency_key
sha256_hex_of_raw_body
```

The idempotency line is the lowercase SHA-256 hex digest of the exact
`Idempotency-Key` header value. Hash an empty byte string when it is absent or
empty. There is no trailing newline after the body digest. Version 3 binds both
the value and its absence, so adding, removing, or changing the header invalidates
the signature before nonce consumption.

`POST /v1/otp/challenges` requires v3 in HMAC mode, with or without an idempotency
key. It rejects v1/v2 with `401/signature_version_required`, even when
`HMAC_V1_ENABLED=true`; allowing v2 without a key would let an intermediary strip
the header from an old signed request. Other routes retain v2 compatibility.
API key and development-only unauthenticated modes are unchanged.

The Go SDK defaults to v3. Coordinate server and caller upgrades: older servers
cannot verify v3, and updated servers reject older HMAC signatures for creation.
Do not switch idempotency keys or identity namespaces while upgrading. Existing
Redis records, response replays, TTLs and unresolved sending records are unchanged.
Retry the same signed key with a fresh timestamp/nonce; reconcile uncertain sends.

The `key_id` line is the literal `X-Key-Id` header value. Only single-`HMAC_SECRET` mode allows an omitted header: sign an empty `key_id` line in that case, rather than `default`. Compute the lowercase hexadecimal HMAC-SHA256 of that value. The default timestamp drift is 60 seconds (`HMAC_MAX_DRIFT`), and each valid nonce is consumed once in Redis. A configured `HERALD_HMAC_DEFAULT_KEY_ID` does not permit omitted wire IDs in key-map v2/v3 requests; they return `401/key_id_required` before nonce consumption. Legacy v1 is disabled unless `HMAC_V1_ENABLED=true` and does not gain nonce protection from this requirement.

### API key

For `REQUEST_AUTH_MODE=api_key`, send `X-API-Key: <key>` or `Authorization: Bearer <key>`.

## Endpoints

### Health Check

**GET /healthz**

Check service health. This endpoint also verifies Redis connectivity.

**Response (Success):**
```json
{
  "status": "ok",
  "service": "herald"
}
```

**Response (Failure - Redis unavailable):**
```json
{
  "status": "unhealthy",
  "error": "Redis connection failed"
}
```

**Note**: The actual response format uses `status` and `service` fields, which differs from the specification's `{ "ok": true }` format. This is the current implementation and is maintained for backward compatibility.

### Create Challenge

**POST /v1/otp/challenges**

Create a new verification challenge and send verification code.


After provider delivery, activation deletes the superseded challenge and updates
its active index in a single Redis Lua operation. A successful resend cannot
leave the previous code redeemable because a separate revoke request failed or
the Herald process exited between those steps. A missing replacement challenge
fails activation without discarding the current one.

An activation error still fails the creation request. If an idempotency key was
used, the sending record remains until `IDEMPOTENCY_UNRESOLVED_TTL` elapses:
retrying the same key returns `delivery_unknown`, rather than sending another code. A lost activation reply
can mean the replacement is already active and the previous code was invalidated;
reconcile delivery before retrying with another key.

Challenge record keys and lifetimes are preserved; activation indexes migrate as
described under "Activation index upgrade" below. No new configuration is required. This fix does not scan for
or remove unidentified orphaned codes left by an earlier revocation failure.
They retain their original expiry; revoke known affected challenge IDs when
reconciling an earlier incident. Keep the existing Redis persistence and audit
volume requirements during upgrades.


**Headers:**
- `Idempotency-Key` (optional): Reuse this key with the same request when retrying. Completed requests replay their response within `IDEMPOTENCY_KEY_TTL`; pre-send concurrent requests return `409/idempotency_in_flight`. Once sending starts, an incomplete request returns `409/delivery_unknown` rather than sending again, for `IDEMPOTENCY_UNRESOLVED_TTL` (default `24h`); after that the record expires and the key can start a new request. A different request with the same key returns `409/idempotency_conflict`.

Fingerprints preserve field boundaries even when JSON strings contain embedded
NUL characters (`\u0000`). Ordinary request fingerprints and Redis key names
remain compatible with earlier versions. Requests containing NUL characters
use a versioned, length-framed fingerprint. If their key already has a legacy
record, retries return `409/idempotency_conflict` without changing or deleting
that record. Normal retention still applies to completed records; unresolved
sending records require delivery reconciliation while they exist. Do not clear them or switch
keys to retry an uncertain send without checking the provider outcome.

Before calling the provider, Herald persists a send record that expires after
`IDEMPOTENCY_UNRESOLVED_TTL` (default `24h`). If the
process exits, the provider reports an uncertain result, or a later Redis write
fails, that record prevents retries from sending another code. A failed final
success write returns `503/backend_unavailable`. Retry with the same key: a
committed result replays, while an unresolved send returns `delivery_unknown`.
Do not automatically switch to a new key on that response; reconcile delivery
with the provider first. A crash immediately before the provider call may also
leave an unresolved record even though nothing was sent. Once the unresolved
TTL has elapsed the record is gone, and a request with the same key is treated
as new and can send a code. Records written without a TTL by versions before
this setting existed are not modified and do not expire.

This protection requires an `Idempotency-Key` and retention of the Redis record.
It does not make the provider and Redis a distributed transaction. A completed
response expires after the configured TTL; using its key after that TTL starts
a new request. See the deployment guide for persistence and reconciliation.

**Request:**
```json
{
  "user_id": "u_123",
  "channel": "sms",
  "destination": "+8613800138000",
  "purpose": "login",
  "locale": "zh-CN",
  "client_ip": "192.168.1.1",
  "ua": "Mozilla/5.0..."
}
```

**`client_ip` and `ua`:** both are optional and neither is stored. Rate limits
and audit records use the address Herald sees for the caller: the direct peer,
or the address in `HERALD_TRUSTED_PROXY_HEADER` when the peer is listed in
`HERALD_TRUSTED_PROXIES`. A `client_ip` that differs from that address only
produces a debug log line with a peppered digest, and `ua` is ignored. To have
the end-user address counted and recorded, send it in the trusted proxy header
(Go SDK: `WithClientIPHeader`); see [security](SECURITY.md#4-rate-limiting-configuration).
The same applies to `client_ip` in both verification requests.

**Channel:** `channel` must be `"sms"`, `"email"`, or `"dingtalk"`. When `channel` is `"email"` and `HERALD_SMTP_API_URL` is set, Herald forwards the send to [herald-smtp](https://github.com/soulteary/herald-smtp); `destination` is the email address. When `channel` is `"dingtalk"`, Herald forwards the send to [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (configure `HERALD_DINGTALK_API_URL`); `destination` is the DingTalk userid (or 11-digit mobile when herald-dingtalk is in mobile lookup mode). Herald does not store any SMTP or DingTalk credentials.

**Response:**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

When `ENVIRONMENT=test` and `HERALD_TEST_MODE=true`, the response also includes `debug_code` (the plain verification code) so callers (e.g. Stargate in debug mode) can display it for local/testing. **Do not enable test mode in production.**

**Error Responses:**

All error responses follow this format:
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "optional error message"
}
```

For challenge-creation `429` responses, a known blocking-window deadline is
reported as optional `next_resend_in` integer seconds and a matching `Retry-After`
header. Missing or expired timing is omitted. This is a retry hint for the gate
that rejected the request; other limits can still reject a later request. A
rejected request consumes no quota and starts no cooldown. Do not retry automatically.

A `429/rate_limit_exceeded` response also carries `limit`, naming the quota
that refused the request: `user` (`RATE_LIMIT_PER_USER`), `ip`
(`RATE_LIMIT_PER_IP`) or `destination` (`RATE_LIMIT_PER_DESTINATION`). A
`429/resend_cooldown` response has no `limit` field. Servers up to v1.6.0 do
not send the field, so treat it as optional.

```json
{
  "ok": false,
  "reason": "rate_limit_exceeded",
  "limit": "ip",
  "next_resend_in": 42
}
```

The Go SDK's `CreateChallengeWithMetadata` method returns the existing response,
a `CreateChallengeMetadata` value and the existing error. Its optional
`RetryAfterSeconds` is nil when timing is absent, malformed, conflicting, outside
1–86400 seconds, or the status is not 429. `CreateChallenge` keeps its original
signature and `*HeraldError` behavior. Older servers remain supported without
inventing a countdown; no SDK method retries challenge creation.

Possible error codes:
- `invalid_request`: Request body parsing failed
- `user_id_required`: Missing required field `user_id`
- `invalid_channel`: Invalid channel type (must be "sms", "email", or "dingtalk")
- `invalid_purpose`: Invalid purpose value (must be one of the allowed purposes)
- `destination_required`: Missing required field `destination`
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired
- `user_locked`: User is temporarily locked: `LOCKOUT_CHALLENGE_THRESHOLD` challenges were exhausted within `LOCKOUT_DURATION`, or `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` wrong codes were submitted within `LOCKOUT_FAILED_ATTEMPT_WINDOW`. An exhausted challenge alone answers `locked`
- `send_failed`: Failed to send verification code via provider. The request's quota stays spent. An immediate retry is answered `429/resend_cooldown` unless the failure proved that nothing was accepted; see [Rate Limiting](#rate-limiting)
- `channel_not_configured`: No provider is configured for the requested channel and `PROVIDER_FAILURE_POLICY=strict`. The request is refused before the idempotency claim, the limits and the cooldowns, so nothing is spent; retrying does not help until the operator configures a provider
- `internal_error`: Internal server error

HTTP Status Codes:
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Authentication failed
- `403 Forbidden`: User locked
- `429 Too Many Requests`: Rate limit exceeded
- `503 Service Unavailable`: Channel not configured (`channel_not_configured`), or a backend is unavailable
- `500 Internal Server Error`: Internal server error

### Get Test Code (Test Mode Only)

**GET /v1/test/code/:challenge_id**

Returns the verification code for a challenge. **Requires `ENVIRONMENT=test`, `HERALD_TEST_MODE=true`, and the dedicated
`HERALD_TEST_API_KEY`. Use `X-Test-Api-Key` or Bearer authentication on the
separate test listener (default `http://127.0.0.1:18082`); this route is absent
from the main service listener.** Used by integration tests or by Stargate in debug mode when the create-challenge response did not include `debug_code` (e.g. when using an older Herald client). Do not enable test mode in production.

**Response (200 OK):**
```json
{
  "ok": true,
  "challenge_id": "ch_7f9b...",
  "code": "123456"
}
```

**Error:** `404 Not Found` when test mode is off, or when the challenge does not exist or the code is not stored.

### Verify Challenge

**POST /v1/otp/verifications**

Verify a challenge code. This route has no expected user, purpose or channel
binding; prefer `POST /v2/otp/verifications`. When the server sets
`HERALD_VERIFY_V1_ENABLED=false` this route is not registered and returns
`404 Not Found`.

**Request:**
```json
{
  "challenge_id": "ch_7f9b...",
  "code": "123456",
  "client_ip": "192.168.1.1"
}
```

**Response (Success):**
```json
{
  "ok": true,
  "user_id": "u_123",
  "amr": ["otp"],
  "issued_at": 1730000000
}
```

**Response (Failure):**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Error Responses:**

Possible error codes:
- `invalid_request`: Request body parsing failed
- `challenge_id_required`: Missing required field `challenge_id`
- `invalid_challenge_id`: `challenge_id` is longer than 64 bytes or contains a character other than `A-Z`, `a-z`, `0-9`, `-` and `_`. IDs returned by challenge creation (`ch_` and 22 such characters) always pass
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid
- `expired`: Challenge has expired, was already used or does not exist
- `invalid`: Invalid verification code
- `locked`: Challenge locked due to too many attempts
- `user_locked`: The challenge's user is temporarily locked; the code was not compared
- `not_active`: The challenge was created but its delivery step has not completed; no attempt is consumed
- `verification_failed`: General verification failure
- `lock_contention`: Another verification of the same challenge is in progress; no attempt is consumed
- `backend_unavailable`: Verification could not be carried out, for example because Redis is unavailable; no attempt is consumed
- `audit_unavailable`: An audit record this request requires could not be stored

A response for a wrong code also carries `remaining_attempts`.

HTTP Status Codes:
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Verification failed: `expired`, `invalid`, `locked`, `user_locked`, `not_active` or `verification_failed`. A locked user is reported with 401 here; only challenge creation answers `403/user_locked`
- `409 Conflict`: `lock_contention`, with `Retry-After: 1`
- `415 Unsupported Media Type`: `Content-Type` is not `application/json` (`unsupported_media_type`)
- `503 Service Unavailable`: `backend_unavailable` (with `Retry-After: 1`) or `audit_unavailable`. This route does not answer 500 for a backend failure

### Verify Challenge with Context (v2)

**POST /v2/otp/verifications**

All three context fields are required and must be non-blank strings:

```json
{
  "challenge_id": "ch_example",
  "code": "123456",
  "expected_user_id": "u_123",
  "expected_purpose": "login",
  "expected_channel": "email"
}
```

Derive the expected user, purpose, and channel from trusted application state.
Do not forward these fields directly from an end user's request. Request
signature authentication identifies the calling service; it does not establish
which user or operation that service should authorize.

Missing, null, empty, or whitespace-only context fields return
`400/context_required` before verification. A supplied context that differs from
the challenge returns `409/context_mismatch` without consuming the code. On
success, the response includes `ok`, `user_id`, `purpose`, `channel`,
`challenge_id`, `amr`, `verified_at`, and `issued_at`. A successful verification
consumes the challenge.

Compatibility: v2 no longer accepts unbound or partially bound requests. Update
existing v2 callers to supply all three fields. The v1 verification endpoint
retains its existing behavior and does not require expected context fields.

A `challenge_id` that is not a possible challenge ID returns
`400/invalid_challenge_id` on both verification routes and on revoke, before
it is used as a Redis key, written to the audit log or attached to a span.

### Revoke Challenge

**POST /v1/otp/challenges/{id}/revoke**

Revoke a challenge (optional).

**Response (Success):**
```json
{
  "ok": true
}
```

**Response (Failure):**
```json
{
  "ok": false,
  "reason": "error_code"
}
```

**Error Responses:**

Possible error codes:
- `challenge_id_required`: Missing challenge ID in URL parameter
- `invalid_challenge_id`: The challenge ID in the URL is longer than 64 bytes or contains a character other than `A-Z`, `a-z`, `0-9`, `-` and `_`. IDs returned by challenge creation (`ch_` and 22 such characters) always pass
- `internal_error`: Internal server error

HTTP Status Codes:
- `400 Bad Request`: Invalid request
- `500 Internal Server Error`: Internal server error

### TOTP Proxy (Optional)

When `HERALD_TOTP_ENABLED=true` and `HERALD_TOTP_BASE_URL` is set, Herald proxies TOTP (Authenticator) operations to [herald-totp](https://github.com/soulteary/herald-totp). All TOTP routes use the same `REQUEST_AUTH_MODE` as OTP routes: HMAC or API key. Optional mTLS is an additional transport policy and does not replace request authentication.

#### Get TOTP Status

**GET /v1/totp/status**

Query whether the subject has TOTP enrolled.

**Query:**
- `subject` (required): User identifier (e.g. user_id)

**Response (Success):** `{"subject":"u_123","totp_enabled":true}` with `totp_enabled` set to `false` when no enabled credential exists.

**Error Responses:**
- `400 Bad Request`: Missing `subject` (`invalid_request`, `subject required`)
- `503 Service Unavailable`: TOTP not configured (`totp_not_configured`)
- `502 Bad Gateway`: Proxy to herald-totp failed (`proxy_failed`)

#### Verify TOTP

**POST /v1/totp/verify**

Verify a TOTP code.

**Request:**
```json
{
  "subject": "u_123",
  "code": "123456"
}
```

**Response:** A successful upstream HTTP 200 returns the TOTP verification result (including `ok`, `subject`, `amr`, and `issued_at`). Upstream non-200 responses, including invalid-code, replay and rate-limit responses, return fixed `502/proxy_failed`. Transport failures and invalid, incomplete or oversized upstream responses also return `502/proxy_failed`; upstream error bodies and headers are not forwarded. The enrollment-confirmation 400/429 handling below is specific to that endpoint.

#### Start TOTP Enrollment

**POST /v1/totp/enroll/start**

Start TOTP enrollment; returns secret/QR for the authenticator app.

**Request:**
```json
{
  "subject": "u_123"
}
```

**Response:** Proxied from herald-totp (e.g. `enroll_id`, QR/secret). On proxy error, Herald returns `502` with `proxy_failed`.

#### Confirm TOTP Enrollment

**POST /v1/totp/enroll/confirm**

Confirm TOTP enrollment with a code from the app.

**Request:**
```json
{
  "enroll_id": "enroll_xxx",
  "code": "123456"
}
```

**Response:** Proxied from herald-totp. A complete upstream `400` returns `400/invalid`; a complete upstream `429` returns `429/rate_limited`. Other upstream failures, incomplete responses and oversized bodies return `502/proxy_failed`. Upstream error bodies and headers are not forwarded.

#### Revoke TOTP

**POST /v1/totp/revoke**

Revoke TOTP for a subject.

**Request:**
```json
{
  "subject": "u_123"
}
```

**Response:** Proxied from herald-totp. On proxy error, Herald returns `502` with `proxy_failed`.

A `200` response confirms the revocation only when its body has `"ok": true`.
The Go SDK method `TOTPRevoke` returns an error with reason
`revoke_not_confirmed` for a `200` response without it.

**TOTP error codes (from Herald proxy):**
- `totp_not_configured`: TOTP not enabled or herald-totp URL not set
- `proxy_failed`: Request to herald-totp failed
- `invalid_request`: Missing or invalid request body/query

## Rate Limiting

Herald implements multi-dimensional rate limiting:

- **Per User**: 10 requests per hour (configurable)
- **Per IP**: 5 requests per minute (configurable)
- **Per Destination**: 10 requests per hour (configurable)
- **Resend Cooldown**: 60 seconds between resends, applied both to the same
  user and destination and to the destination alone, so a different `user_id`
  cannot shorten the wait for one destination

Order of checks for challenge creation: the user lock is checked first and does
not change any counter. The three limits and both cooldowns are then applied as
one atomic step. If any of them rejects the request, none of the counters is
incremented and no cooldown starts; the response reports the gate that rejected
it (`rate_limit_exceeded` for a limit, `resend_cooldown` for a cooldown; user,
IP and destination limits take precedence in that order). A request that passes
this step keeps its quota spent even if the provider then fails. The cooldown
it started is released only when the answer is `500/send_failed` and the
failure proves that no provider accepted the message (for example the provider
could not be connected to); after any other failed send it stays in place. See
[channels without a provider](DEPLOYMENT.md#channels-without-a-provider).

## Error Codes

This section lists all possible error codes returned by the API.

### Request Validation Errors
- `invalid_request`: Request body parsing failed or invalid JSON
- `user_id_required`: Missing required field `user_id`
- `invalid_channel`: Invalid channel type (must be "sms", "email", or "dingtalk")
- `invalid_purpose`: Invalid purpose value (must be one of the allowed purposes)
- `destination_required`: Missing required field `destination`
- `challenge_id_required`: Missing required field `challenge_id`
- `invalid_challenge_id`: `challenge_id` is longer than 64 bytes or contains a character other than `A-Z`, `a-z`, `0-9`, `-` and `_`. IDs returned by challenge creation (`ch_` and 22 such characters) always pass
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid

### Authentication Errors
HMAC/API-key failures use a JSON `reason` value:
- `signature_version_required`: Challenge creation requires `X-Signature-Version: v3`; other HMAC routes accept v2/v3
- `missing_auth_headers`: One or more HMAC headers are missing
- `key_id_required`: The `X-Key-Id` header is missing where one is required
- `timestamp_out_of_range`: Timestamp is outside the configured drift window
- `invalid_signature`: HMAC signature verification failed, or `X-Key-Id` names a key that is not configured. The two cases get the same response so that it does not reveal which key IDs exist; `herald_auth_failure_total{reason}` tells them apart
- `replayed_nonce`: The nonce was already consumed
- `nonce_store_unavailable`: Redis nonce storage failed. The request is rejected in every environment
- `unauthorized`: Authentication failed

### Idempotency and verification coordination errors
- `idempotency_in_flight` (409): A pre-send claim is still in progress.
- `delivery_unknown` (409): Sending started; do not resend automatically.
- `idempotency_conflict` (409): The same key has different fingerprinted fields.
- `idempotency_namespace_conflict` (409): A legacy empty-key-ID record needs operator reconciliation.
- `context_required` (400) / `context_mismatch` (409): Required v2 context is missing or does not match.
- `lock_contention` (409): Verification lock is contended; retry with a fresh nonce.
- `backend_unavailable` / `audit_unavailable` (503): Backend or durable audit acceptance is unavailable. A failure after a side effect does not undo that side effect.
- `hash_capacity_exceeded` (503, `Retry-After: 1`): Every hash slot of this instance (`HERALD_MAX_CONCURRENT_HASH_OPERATIONS`) is in use. Nothing was consumed; retry shortly. It indicates load on this instance, not a backend failure.

### Challenge Errors
- `expired`: Challenge has expired, was already used or does not exist
- `invalid`: Invalid verification code
- `locked`: Challenge locked due to too many attempts
- `verification_failed`: General verification failure
- `send_failed`: Failed to send verification code via provider (only during challenge creation)
- `channel_not_configured` (503): No provider is configured for the requested channel (strict provider policy, challenge creation only)

### Rate Limiting Errors
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired

### User Status Errors
- `user_locked`: User is temporarily locked: `LOCKOUT_CHALLENGE_THRESHOLD` challenges were exhausted within `LOCKOUT_DURATION`, or `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` wrong codes were submitted within `LOCKOUT_FAILED_ATTEMPT_WINDOW`. An exhausted challenge alone answers `locked`

### Additional transport and storage reasons
- `invalid_destination`: Destination validation failed.
- `invalid_auth_headers`: A reserved namespace delimiter is present in an identity header.
- `empty_body` / `unsupported_media_type`: Empty JSON body or unsupported request content type.
- `payload_too_large` (413) / `request_error`: Request exceeds the body limit or another framework-level 4xx error occurs.
- `not_found` (404) / `method_not_allowed` (405): No matching route or method.
- `code_not_found` (404): The separate test listener has no cached test code.
- `idempotency_ownership_lost` (409): The pre-send or activation ownership check failed; do not force a new send.
- `redis_unreachable`: Readiness failed; `/readyz` returns 503.

### System Errors
- `internal_error`: Internal server error

## Audit acceptance failures

With audit enabled, required events must be accepted into the durable local
buffer. A full or unwritable buffer returns `503` with
`{"ok":false,"reason":"audit_unavailable"}`. Backend delivery outages alone do
not reject requests while buffer capacity remains. Verification and revoke
attempts are recorded before the operation; result events follow it. These are
not a distributed transaction.

A record that must be accepted before the action (challenge created,
verification attempt, revoke requested) still refuses the request with
`audit_unavailable` when it cannot be stored, and the action does not happen.
A result record written after the action can no longer be undone does not
change the response: when the provider has accepted the message, a
verification has consumed the code, or a revoke has completed, the request
returns its normal success response and the idempotency record is completed
even if that record cannot be stored. In soft provider-failure mode the same
applies to the `delivery_status=failed` response. The lost record is counted in
`herald_audit_post_action_failure_total{operation}` and logged once at error
level with its event type, challenge ID, result and reason, and the user only
as a digest.

Two result records keep the earlier behaviour: a failed verification, and a
provider failure under the strict policy. Both already return an error; if
their audit record cannot be stored the reason is `audit_unavailable` instead.
The verification attempt has still been counted and the provider has still
been called, so such a 503 does not imply that nothing happened. Follow the
idempotency retry rules above.

### Activation index upgrade

Activation indexes preserve the exact user ID, purpose, channel and destination,
using a versioned encoding with explicit field boundaries. User IDs such as
`Alice` and `alice` no longer share an activation index. This prevents a request
for one identity from revoking another identity's code when the remaining fields
match. It does not allow one user to verify another user's code.

Existing challenge records remain verifiable until their original expiry. On
resend, an old index is migrated atomically only when its stored challenge has
exactly the same four identity fields. A colliding legacy record is preserved.
Previously overwritten legacy indexes cannot be reconstructed automatically;
unindexed codes expire normally and known affected challenge IDs can be revoked.

Stop creation traffic and replace all old instances before resuming creation.
Mixed versions can use different indexes and leave superseded codes valid.
Rollback requires stopping creation and waiting the maximum challenge lifetime
before resuming on the old version. No client API or challenge record format
changes are required.

## Hash capacity overload

Creation and v1/v2 OTP verification share a local hash-work admission budget
(default 4). When it is full, requests return `503/hash_capacity_exceeded` with
`Retry-After: 1` before hashing. Releases up to v1.6.0 answered
`503/backend_unavailable` here, the same reason as a Redis outage; the status and
header are unchanged. Overloaded creation requests do not reserve rate
or cooldown capacity, and rejected verifications consume no attempt or code.
Completed idempotency responses can still replay. Lock contention is reported
immediately as the existing `409/lock_contention` with `Retry-After: 1`.
See [deployment](DEPLOYMENT.md#otp-hash-work-admission) for sizing and retry rules.

## HMAC identity and legacy idempotency records

With only `HMAC_SECRET` configured, omit `X-Key-Id` or send exactly `default`.
Other IDs return `401/invalid_signature`, the same answer as a wrong signature,
before nonce consumption or handler side effects. A single secret no longer implicitly authorizes arbitrary IDs. HMAC
v2/v3 signing is unchanged. Custom IDs require an explicit `HERALD_HMAC_KEYS`
entry; do not rename an existing caller or retry an uncertain send under another
identity during migration. See the deployment upgrade requirements below.

Idempotency uses the signed service and the actual key ID selected for HMAC
verification. In single-secret mode, omitting `X-Key-Id` and sending `default` use
the same record. This does not change the signature: an omitted wire header still
has an empty canonical key-ID line. No HMAC version or SDK API change is required.

Existing explicit-key records retain their keys, contents, ownership and TTLs;
replaying them requires that the key ID remains authorized.
Old empty-key-ID records do not identify the credential that originally used a
possibly rotated default. When such a record exists for the same service and
client key, creation returns `409/idempotency_namespace_conflict` before hashing,
rate/cooldown writes or delivery. It is not copied, replayed or deleted, even when
its fingerprint matches. This guard applies to every verified key ID so default
rotation cannot expose an old response under another credential. API-key and
unauthenticated development namespaces are unchanged.

Preserve unresolved sending records and the idempotency derivation secret.
Reconcile legacy conflicts using delivery/audit evidence; do not switch client
keys or clear unresolved records to force a resend. Completed legacy records
keep their ordinary expiry. See [upgrade requirements](DEPLOYMENT.md#hmac-identity-upgrade).

### Pending challenge state

New challenges cannot be verified before activation. If activation and pending
cleanup both fail, the original pending record may remain until expiry but
verification returns `not_active` without consuming an attempt. A successful
activation preserves the record's remaining lifetime. If its reply is lost, the
record may already be active; pending cleanup leaves that committed state intact.
Creation still reports an error and an unresolved idempotency key still returns
`delivery_unknown`. Follow the pending-protocol migration procedure in
[MIGRATION_V2.md](MIGRATION_V2.md#pending-challenge-verification-protocol) before
upgrading or rolling back a running cluster.
