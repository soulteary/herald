# Security Documentation

> 🌐 **Language / 语言**: [English](SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](../koKR/SECURITY.md)

This document explains Herald's security features, security configuration, and best practices.

## Implemented Security Features

1. **Challenge-Based Verification**: Uses challenge-verify model to prevent replay attacks and ensure one-time use of verification codes
2. **Secure Code Storage**: Verification codes are stored as Argon2 hashes, never in plaintext
3. **Multi-Dimensional Rate Limiting**: Rate limiting by user_id, destination (email/phone), and IP address to prevent abuse. The limits are consumed together or not at all, so a request that is rejected (by a limit, a cooldown or a user lock) does not use up another user's or destination's quota
4. **Service Authentication**: Uses HMAC or API-key request authentication, with optional mTLS as an additional transport policy
5. **Idempotency Protection**: Prevents duplicate challenge creation and code sending using idempotency keys
6. **Challenge Expiration**: Automatic expiration of challenges with configurable TTL
7. **Attempt Limiting**: Maximum attempt limits per challenge to prevent brute force attacks
8. **Resend Cooldown**: Prevents rapid resend of verification codes, per user and destination and per destination alone, so changing `user_id` does not bypass it
9. **Audit Logging**: Complete audit trail for all operations including sends, verifications, and failures
10. **Provider Security**: Secure communication with email and SMS providers

## Security Best Practices

### 1. Production Environment Configuration

**Required Configuration**:
- Set `ENVIRONMENT=production`; production validation fails closed on unsafe combinations
- Select `REQUEST_AUTH_MODE=hmac_v2` (recommended) or `api_key`, and configure credentials for that mode
- Keep `HERALD_TEST_MODE=false`; test mode is rejected in production
- Set `PROVIDER_FAILURE_POLICY=strict` and use HTTPS provider URLs
- Protect Redis with `REDIS_PASSWORD` (or explicitly acknowledge the risk with `HERALD_RISK_ACK_PASSWORDLESS_REDIS=true`)
- Use strong, unique HMAC secrets and rotate keys with explicit key IDs

**Configuration Example**:
```bash
export ENVIRONMENT=production
export REQUEST_AUTH_MODE=hmac_v2
# Provision these three values once on a secure administration host, store them
# in a secret manager, and inject the same values into every replica. Do not
# regenerate them in the application entrypoint or on restart:
#   openssl rand -hex 32  # HERALD_HMAC_KEY_1
#   openssl rand -hex 32  # HERALD_HMAC_KEY_2
#   openssl rand -hex 32  # HERALD_IDEMPOTENCY_KEY
: "${HERALD_HMAC_KEY_1:?load HERALD_HMAC_KEY_1 from the secret manager}"
: "${HERALD_HMAC_KEY_2:?load HERALD_HMAC_KEY_2 from the secret manager}"
: "${HERALD_IDEMPOTENCY_KEY:?load HERALD_IDEMPOTENCY_KEY from the secret manager}"
export HERALD_HMAC_KEYS="{\"key-id-1\":\"${HERALD_HMAC_KEY_1}\",\"key-id-2\":\"${HERALD_HMAC_KEY_2}\"}"
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HERALD_IDEMPOTENCY_SECRET="${HERALD_IDEMPOTENCY_KEY}"
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
unset HERALD_HMAC_KEY_1 HERALD_HMAC_KEY_2 HERALD_IDEMPOTENCY_KEY
export HERALD_TEST_MODE=false
export PROVIDER_FAILURE_POLICY=strict
export REDIS_PASSWORD="your-redis-password"
export REDIS_ADDR="redis:6379"
```

### 2. Sensitive Information Management

**Recommended Practices**:
- ✅ Use environment variables to store API keys and secrets
- ✅ Use `REDIS_PASSWORD` or a secrets manager for Redis when needed
- ✅ Use key management services (e.g., HashiCorp Vault) for production secrets
- ✅ Ensure configuration file permissions are set correctly (e.g., `chmod 600`)
- ✅ Never log verification codes or sensitive user data

**Not Recommended**:
- ❌ Hardcode secrets in configuration files
- ❌ Pass secrets via command line arguments (will appear in process list)
- ❌ Commit configuration files containing sensitive information to version control
- ❌ Log verification codes or challenge details in production

### 3. Network Security

**Required Configuration**:
- Production environments must use HTTPS
- Configure firewall rules to restrict access to Herald service
- Use mTLS for inter-service communication (highest security)
- Regularly update dependencies to fix known vulnerabilities

**Recommended Configuration**:
- Use reverse proxy (such as Nginx or Traefik) to handle SSL/TLS
- Ensure your reverse proxy forwards client IP (e.g. X-Forwarded-For) if rate limiting by IP is used
- Use strong HMAC secrets (minimum 32 bytes)
- Disable insecure authentication methods in production (prefer HMAC request authentication and optionally add mTLS transport protection)

### 4. Rate Limiting Configuration

Herald implements multi-dimensional rate limiting to prevent abuse:

**Rate Limit Dimensions**:
- **Per User ID**: Limits challenges per user_id per time window
- **Per Destination**: Limits challenges per email/phone number per time window
- **Per IP Address**: Limits challenges per caller address per time window

The per-IP limit is keyed on the address Herald sees for the caller, not on the
`client_ip` field in the request body. That field is accepted and not stored:
audit records carry the address Herald sees for the caller, and a body
`client_ip` that differs from it only produces a debug log line with a peppered
digest. The `ua` field of a create request is accepted and ignored. When a
gateway such as Stargate calls Herald directly, every end user arrives from the
gateway's address and shares one `RATE_LIMIT_PER_IP` quota. To limit per end
user:

1. Set `HERALD_TRUSTED_PROXY_HEADER` (for example `X-Forwarded-For`) and list
   the gateway's address or CIDR in `HERALD_TRUSTED_PROXIES`.
2. Have the gateway send the end-user address in that header. The Go SDK does
   this when the client is built with `WithClientIPHeader`.

Herald reads the header only from peers in `HERALD_TRUSTED_PROXIES`. Without
this configuration, size `RATE_LIMIT_PER_IP` for the gateway's total request
rate.

**Configuration Example**:
```bash
# Rate limits (requests per hour)
export RATE_LIMIT_PER_USER=10         # Per user_id, per hour
export RATE_LIMIT_PER_DESTINATION=10  # Per email/phone, per hour
export RATE_LIMIT_PER_IP=5            # Per IP address, per minute
```

### 5. Challenge Security

**Challenge Configuration**:
- **TTL**: Challenge expiration time (default: 300 seconds)
- **Max Attempts**: Maximum verification attempts per challenge (default: 5)
- **Resend Cooldown**: Minimum time between resends (default: 60 seconds)

**Configuration Example**:
```bash
export CHALLENGE_EXPIRY=5m
export MAX_ATTEMPTS=5
export RESEND_COOLDOWN=60s
```

## API Security

### Independent Security Layers

Herald configures transport authentication and request authentication independently:

1. **Transport layer (optional mTLS)**
   - `CLIENT_CERT_MODE=off|optional|require`
   - `require` is recommended when every caller has a trusted client certificate
   - mTLS does not replace request-body integrity checks

2. **Request layer**
   - `REQUEST_AUTH_MODE=hmac_v2` (recommended): replay-resistant HMAC-SHA256
   - `REQUEST_AUTH_MODE=api_key`: API key from `X-API-Key` or `Authorization: Bearer ...`
   - `REQUEST_AUTH_MODE=none`: development only; rejected in production

### HMAC v3 Authentication

Challenge creation requires v3 even without an idempotency header. Changing,
adding or removing that header invalidates the signature before nonce use. Older
HMAC callers must upgrade; other verification routes retain v2 compatibility.
See [API authentication](API.md#hmac-v3) for the canonical format and upgrade rules.

HMAC v3 signs the request shape, idempotency header digest and body digest:

```text
HERALD-HMAC-V3
METHOD
PATH
QUERY
TIMESTAMP
NONCE
SERVICE
KEY_ID
SHA256_HEX(IDEMPOTENCY_KEY_OR_EMPTY)
SHA256_HEX(RAW_BODY)
```

**Required request headers**:
- `X-Signature-Version: v3`
- `X-Signature`: lowercase hexadecimal HMAC-SHA256 of the canonical string
- `X-Timestamp`: Unix timestamp in seconds
- `X-Nonce`: unique value for this request
- `X-Service`: caller service identifier
- `X-Key-Id`: required whenever `HERALD_HMAC_KEYS` is configured, including singleton maps; only single-`HMAC_SECRET` mode allows omission

When single-`HMAC_SECRET` mode omits `X-Key-Id`, keep the canonical `KEY_ID` line empty. Key-map requests must sign their explicit ID; `HERALD_HMAC_DEFAULT_KEY_ID` does not replace that field. Configure client IDs before upgrading verifiers, and preserve Redis replay state. Older verifiers can accept an unchanged omitted-ID request again after a same-secret default-ID reassignment; rollback restores that behavior.

**Server configuration**:
```bash
export REQUEST_AUTH_MODE=hmac_v2
: "${HERALD_HMAC_KEYS:?load the key map with production-strength secrets}"
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
```

Herald verifies the signature before atomically consuming the nonce in Redis. Reusing a nonce is rejected. The default timestamp drift is 60 seconds. Nonces are retained for at least twice the allowed drift plus 30 seconds (150 seconds by default), covering the full acceptance window even for future-dated requests. Legacy v1 is disabled by default and should only be enabled temporarily during migration with `HMAC_V1_ENABLED=true`.

### mTLS

To require trusted client certificates:

```bash
export TLS_CERT_FILE=/path/to/herald.crt
export TLS_KEY_FILE=/path/to/herald.key
export TLS_CA_CERT_FILE=/path/to/client-ca.crt
export CLIENT_CERT_MODE=require
```

`TLS_CLIENT_CA_FILE` is accepted as an alias for `TLS_CA_CERT_FILE`. When TLS is enabled in the container, configure the loopback health probe with the relevant `HERALD_HEALTHCHECK_TLS_*` variables.

## Data Security

### Verification Code Storage

- **Never stored in plaintext**: All verification codes are hashed using Argon2
- **One-time use**: Challenges are deleted immediately after successful verification
- **Automatic expiration**: Challenges expire after TTL to prevent stale code usage

### Redis Security

- Redis should be configured with password protection
- Use `REDIS_PASSWORD` environment variable when Redis requires authentication
- Restrict Redis network access (only allow Herald service access)
- Use Redis AUTH for authentication
- Regularly update Redis to fix known vulnerabilities
- Set `REDIS_TLS_ENABLED=true` to encrypt the Redis connection. The server
  certificate is always verified, against `REDIS_TLS_CA_FILE` when set and the
  system roots otherwise; there is no option to skip verification. Client
  certificates for Redis are not supported. TLS is off by default, so without it
  use a protected private network
- Set `REDIS_USERNAME` to authenticate as a Redis ACL user with only the
  permissions Herald needs, instead of the default user
- Production refuses to start when `REDIS_TLS_CA_FILE` or
  `REDIS_TLS_SERVER_NAME` is set without `REDIS_TLS_ENABLED=true`, or
  `REDIS_USERNAME` is set without `REDIS_PASSWORD`, because those settings
  would otherwise be ignored

### Audit Logging

Herald maintains complete audit logs for:
- Challenge creation and sending
- Verification attempts (success and failure)
- Rate limit hits
- Provider communication (success and failure)
- Authentication failures

**Audit Log Fields**:
- Timestamp
- Operation type
- User ID
- Destination (masked)
- Channel (email/sms)
- Result (success/failure)
- Reason (for failures)
- Client IP
- Provider information

## Provider Security

### Email Provider Security

- The built-in SMTP provider requires STARTTLS with a verified server certificate; it does not fall back to an unencrypted connection when STARTTLS is missing or refused, so `SMTP_PASSWORD` and the message are never sent in clear
- Authenticate with provider using secure credentials
- Store provider credentials securely (environment variables or secrets management)
- Monitor provider communication for failures

### SMS Provider Security

- Use HTTPS for API communication
- Authenticate with provider using secure API keys
- Store provider credentials securely
- Monitor provider communication for failures

## Rate Limiting and Abuse Prevention

### Rate Limit Strategy

Herald implements multiple layers of rate limiting:

1. **Per-User Rate Limiting**: Prevents a single user from creating too many challenges
2. **Per-Destination Rate Limiting**: Prevents abuse of a specific email/phone number
3. **Per-IP Rate Limiting**: Prevents abuse from a single IP address
4. **Resend Cooldown**: Prevents rapid resend of codes to the same destination
5. **Attempt Limiting**: Limits verification attempts per challenge

### Configuration

```bash
# Rate limits
export RATE_LIMIT_PER_USER=10         # per hour
export RATE_LIMIT_PER_DESTINATION=10  # per hour
export RATE_LIMIT_PER_IP=5            # per minute

# Challenge settings
export CHALLENGE_EXPIRY=5m
export MAX_ATTEMPTS=5
export RESEND_COOLDOWN=60s
```

### SMS destination canonicalisation

The per-destination rate limit and the resend cooldown are keyed on a canonical
form of the validated destination. For SMS, a leading `+` and an international
`00` dialling prefix are removed, so `+8613800138000`, `8613800138000` and
`008613800138000` share one quota and one cooldown. Spacing and separators were
already ignored.

Limits of this canonicalisation:

- No country code is inferred. A national number (`13800138000`) and its
  international form (`+8613800138000`) remain separate limiter identities.
  Callers should send one consistent format, preferably E.164.
- Only limiter keys change. The stored challenge, the audit record, the
  idempotency fingerprint and the value sent to the provider keep the
  destination as validated.
- Trunk prefixes other than `00` (for example `011`) are not recognised.

After an upgrade, a number previously sent with `+` or `00` starts a new
per-destination counter and cooldown key; the old keys expire with their
original TTL.

## User lockout

A user is locked for `LOCKOUT_DURATION` (default `10m`) in two independent
ways. While locked, verification answers `user_locked` without comparing the
code and challenge creation answers `403/user_locked`.

1. **Exhausted challenges.** `LOCKOUT_CHALLENGE_THRESHOLD` (default `3`)
   challenges each used up all `MAX_ATTEMPTS` within `LOCKOUT_DURATION`.
2. **Wrong codes.** `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` (default `15`) wrong
   codes within `LOCKOUT_FAILED_ATTEMPT_WINDOW` (default `1h`), on any of the
   user's challenges and on either verification route.

The second rule exists because the first can be avoided: a caller who submits
`MAX_ATTEMPTS - 1` wrong codes and then requests a new challenge never exhausts
one, so nothing was recorded and no number of wrong codes locked the user.
Wrong codes are now counted per user. Only a wrong code that used an attempt
counts; an expired or unknown challenge, a context mismatch, a refusal while
the user is locked and a repeated probe of an exhausted challenge do not. A
successful verification does not reset the count. When the threshold is
reached the user is locked and the count starts again from zero. A lock
already in effect is not extended. If the count cannot be stored the
verification answers `503/backend_unavailable`.

What this bounds, with the defaults: the number of codes a user can be issued
(`RATE_LIMIT_PER_USER`, 10 per hour) times `MAX_ATTEMPTS` (5) already caps
wrong codes at about 50 per hour per user, and a 10-minute lock every 15 wrong
codes lowers that only slightly. The count makes such guessing lock the user
and show up in `herald_failed_attempt_lock_total`, and it makes the lock
settings effective: after a lock, at most `LOCKOUT_FAILED_ATTEMPT_THRESHOLD`
further wrong codes are possible before the next one, so a longer
`LOCKOUT_DURATION` or a lower threshold reduces the guess rate, which the
exhausted-challenge rule alone could not do. Lower values also make it easier
for someone who can trigger verifications to lock a user out; choose them with
that in mind. Set `LOCKOUT_FAILED_ATTEMPT_THRESHOLD=0` to disable the count.

The count is stored under `otp:lock:failed:<digest>`, keyed like the other
lock keys by a peppered digest of the user ID.

## Error Handling

### Production Mode

In production (with `ENVIRONMENT=production`; `HERALD_TEST_MODE=true` is rejected):

- Hide detailed error information to prevent information leakage
- Return generic error messages
- Delivery failures expose only bounded reason codes in logs and tracing
- Audit records follow the configured masking and retention policy

### Development Mode

In development mode:

- Generic API errors remain in use; development does not enable plaintext provider diagnostics
- Test code exposure requires the explicit test environment and test-mode settings

## Security Response Headers

Herald does not install a browser security-header middleware by default.
Configure headers such as `X-Content-Type-Options`, `Content-Security-Policy`
and HSTS at the HTTPS reverse proxy where appropriate; do not assume these
headers are emitted by Herald.

## Vulnerability Reporting

If you discover a security vulnerability, please report it through:

1. **GitHub Security Advisory** (Preferred)
   - Go to the [Security tab](https://github.com/soulteary/herald/security) in the repository
   - Click on "Report a vulnerability"
   - Fill out the security advisory form

2. **Email** (If GitHub Security Advisory is not available)
   - Send an email to the project maintainers
   - Include a detailed description of the vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

## Inter-Service Authentication

When integrating with other services (such as Stargate), use secure authentication:

### Recommended: mTLS

Use mutual TLS certificates for the highest security level.

### Alternative: HMAC Signature

Use HMAC-SHA256 signatures with timestamp validation for secure inter-service communication.

### Not Recommended: API Key

API Key authentication is suitable for development but not recommended for production inter-service communication.

## Provider failure diagnostics

Provider error messages can echo OTP message bodies, destinations or credentials.
Herald records only known provider reason codes in delivery-failure logs, span
status and exception events, and audit failure records. Unknown reason codes map
to `send_failed`; deadline failures remain classified as `timeout`. Log consumers
should use the `reason` field rather than the former free-text `error` field.

For the built-in SMTP provider the delivery-failure log line also carries
`failure_category` (`dns`, `connect`, `tls`, `auth`, `rejected`, `connection`,
`timeout`, `validation`, `canceled` or `other`) and, when the server answered
with one, the three-digit SMTP reply code as `smtp_code`. Both are derived from
the types of the error chain, not from its text, and are written to the log
only, not to spans or audit records. See
[troubleshooting](TROUBLESHOOTING.md#provider-send-failures).

This changes diagnostics only. Client responses, strict-policy revocation and
idempotency `delivery_unknown` handling are unchanged. No Redis record migration
or client update is needed. Existing logs and exported spans are not rewritten;
review access and retention for historical diagnostics if a provider previously
echoed sensitive data, and rotate any credentials exposed there.

## Disabling v1 verification

`POST /v1/otp/verifications` accepts a challenge ID and code only. It cannot
check that the challenge belongs to the user, purpose and channel the caller
expects; `POST /v2/otp/verifications` requires all three. A caller that keeps
using v1 does not get the v2 binding even when other callers have migrated.

Once every caller uses v2, set `HERALD_VERIFY_V1_ENABLED=false`. Herald then
does not register the v1 verification route and requests to it receive
`404/not_found`. Challenge creation, revocation, v2 verification and the TOTP
routes are unaffected. The default is `true`, so existing deployments keep the
v1 route until they opt out. The Go SDK method `VerifyChallenge` calls v1; use
`VerifyChallengeV2` before disabling it.

## User identifiers in traces

Challenge-creation and verification spans carry `user_id_digest`, the same
peppered digest used for rate-limit keys, instead of the raw `user_id`. Tracing
backends are often shared more widely than the audit store, so the raw
identifier is kept out of exported spans. Audit records are unchanged and still
contain the user ID. Trace queries or dashboards that filtered on the `user_id`
span attribute must switch to `user_id_digest`. Spans exported before the
upgrade are not rewritten.

The creation span identifies the destination the same way, as
`destination_digest`: the peppered digest that keys the per-destination rate
limit. It replaces the `destination` attribute, which held a partially masked
value (the first characters of an email address and its whole domain). The
request span no longer has an `http.remote_addr` attribute; the client address
remains in audit records. Queries on either removed attribute need updating.

## Related Documentation

- [API Documentation](API.md) - Learn about API security features and authentication
- [Deployment Documentation](DEPLOYMENT.md) - Learn about production environment deployment recommendations
- [Monitoring Documentation](MONITORING.md) - Learn about security monitoring and alerting
- [Troubleshooting Documentation](TROUBLESHOOTING.md) - Learn about security-related troubleshooting

Test-only exception: `ENVIRONMENT=test` plus `HERALD_TEST_MODE=true` stores
plaintext codes in the test-code cache and exposes them through the guarded test
listener and `debug_code`. Production rejects this mode.

## Hash resource limits

Per-user, IP and destination rate limits do not cap aggregate concurrent Argon2
work across distinct identities and destinations. The shared OTP admission budget
also limits creation, which previously had no global work cap. This protects
against memory exhaustion when authenticated traffic is otherwise within the
configured rate limits. It is not a replacement for those limits or for process
resource limits. See [deployment](DEPLOYMENT.md#otp-hash-work-admission).

## Verified HMAC identity

A key-ID spelling is not a separate credential: omitted and explicit default IDs
must share idempotency protection. Herald derives this identity only after valid
HMAC verification and nonce handling; raw signed fields are not rewritten.
Unattributable legacy records fail closed without exposing their response.
This prevents duplicate sends after success-record failures and replay of a
previous credential's response after default-key rotation. It does not involve
HMAC forgery.
See [upgrade requirements](DEPLOYMENT.md#hmac-identity-upgrade).
## Single-secret HMAC key identities

With only `HMAC_SECRET` configured, unknown key IDs previously selected that
secret while acquiring separate nonce and idempotency namespaces. A validly
signed retry with another ID could send twice after successful delivery and a
failed Redis completion write. This requires the shared secret; it is not a
signature forgery.

Single-secret lookup now permits only an omitted ID or exactly `default`.
Unknown IDs fail before nonce consumption or handler side effects. Explicit key
maps retain their configured identities; HMAC v2/v3 wire formats are unchanged.
Old records remain intact. Renaming an old custom ID does not reconcile its
unresolved sends. Follow the [migration and rollback requirements](DEPLOYMENT.md#single-secret-key-id-allowlist).

## CORS wildcard entries

Production deployments must not combine a standalone `*` with other origins in
`HERALD_CORS_ALLOW_ORIGINS`. Older validation accepted those lists and the CORS
middleware returned `Access-Control-Allow-Origin: *` to unlisted origins. This
requires such a configuration; the default remains CORS-disabled. It does not
bypass API Key or HMAC verification, and unsigned business requests still return
401. Startup and router construction now reject wildcard entries anywhere in a
production list. See [deployment requirements](DEPLOYMENT.md#production-cors-origin-lists).

### Limit service actions

The default service credential remains trusted for every OTP and TOTP action.
For callers with different permissions, set `HERALD_SERVICE_POLICIES` to a JSON
map of HMAC key IDs to fixed service names and allowed actions:

```json
{
  "gateway": {
    "service": "stargate",
    "actions": ["otp:create", "otp:verify", "totp:status", "totp:verify"]
  },
  "account-admin": {
    "service": "account-admin",
    "actions": ["totp:enroll", "totp:revoke", "otp:revoke"]
  }
}
```

Configure distinct random secrets for every key in `HERALD_HMAC_KEYS`, including
keys omitted from the policy. Shared secrets are rejected when policies are
enabled: a caller with a shared secret could otherwise sign as a privileged key ID.
Invalid HMAC key maps cannot fall back to a single secret in policy mode. In single-secret mode,
the only key ID is `default`. Policies require `REQUEST_AUTH_MODE=hmac_v2` and
`HMAC_V1_ENABLED=false`. Unknown keys and mismatched signed `X-Service` values
are rejected; callers cannot select their service identity by changing a header.
A denied action returns 403 before the handler or provider runs. Authentication
failures still return 401. Rejected service identities do not consume a nonce;
authenticated requests denied by action policy do consume their nonce.

Supported actions are `otp:create`, `otp:verify`, `otp:revoke`, `totp:status`,
`totp:verify`, `totp:enroll`, and `totp:revoke`. The enrollment action covers both
start and confirm. The verification action covers both OTP verification versions.
An empty policy variable preserves the previous trusted-service behavior; an
explicit empty map, invalid entry, or reference to an unconfigured key prevents
startup. Deploy the policy with all currently required callers before restricting
credentials.

This is a service/action boundary, not tenant or user isolation. An authorized
TOTP action still applies to the shared subject namespace. Use separate instances,
Redis credentials and TOTP backends for callers that must not share that namespace.

## Authenticated upstream requests

The TOTP proxy uses operation-bound HMAC v2 from herald-totp. Upgrade the TOTP
service and Herald together while callers are paused; legacy body-only
signatures are rejected by the updated service. The proxy retains its TLS,
redirect, and response-size checks. HMAC requests omit the optional API key.

The public Herald SDK now rejects redirects, including same-origin redirects.
Configure the final service URL so API keys and request bodies cannot be sent
to an unintended endpoint.

Signed paths and raw queries must reach herald-totp unchanged. Use its internal
service origin when an ingress would strip or rewrite a public URL prefix.
Permit expiring `totp:auth:nonce:*` keys in the upstream Redis ACL.

### Recovery after a failed OTP activation

Strict activation writes the active index before marking the challenge active.
Redis script runtime errors can retain earlier writes: an activation failure may
remove the previous challenge or leave an index pointing at a pending record.
The pending record remains unverifiable. Abandoned pending records and stale
index entries expire with their original TTL; pending cleanup does not delete a
successfully activated record after a lost reply.

This ordering prevents new partial activations. If an older deployment experienced
an activation error after its record was marked active, upgrading does not repair
that record. Wait its original challenge lifetime or explicitly revoke known IDs.
This fix does not require another namespace migration.

TOTP HMAC service names and key IDs have a 128-byte limit, matching the upstream
verifier. The proxy rejects oversized identifiers locally before sending a request.
These limits do not apply to unused HMAC fields in API-key-only mode.
