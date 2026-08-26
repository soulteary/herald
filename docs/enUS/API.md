# Herald API Documentation

Herald is a verification code and OTP service that handles sending verification codes via SMS, email, and DingTalk (email via built-in SMTP or [herald-smtp](https://github.com/soulteary/herald-smtp) when `HERALD_SMTP_API_URL` is set, DingTalk via [herald-dingtalk](https://github.com/soulteary/herald-dingtalk)), with built-in rate limiting and security controls.

## Base URL

```
http://localhost:8082
```

## Authentication

Request authentication and client-certificate policy are independent:

- `REQUEST_AUTH_MODE=hmac_v2` (default) requires replay-resistant HMAC v2.
- `REQUEST_AUTH_MODE=api_key` requires `X-API-Key` or `Authorization: Bearer …`.
- `REQUEST_AUTH_MODE=none` is for development only and is rejected in production.
- `CLIENT_CERT_MODE=off|optional|require` controls TLS client certificates. A verified certificate does not bypass the selected request-auth mode.

### HMAC v2

Send `X-Signature-Version: v2`, `X-Signature`, `X-Timestamp`, `X-Nonce`, `X-Service`, and, for multi-key configurations without a configured default, `X-Key-Id`.

The signed canonical value is newline-delimited:

```text
HERALD-HMAC-V2
METHOD
/path
raw=query
unix_timestamp
single_use_nonce
service_name
key_id
sha256_hex_of_raw_body
```

Compute the lowercase hexadecimal HMAC-SHA256 of that value. The default timestamp drift is 60 seconds (`HMAC_MAX_DRIFT`), and each valid nonce is consumed once in Redis. With multiple `HERALD_HMAC_KEYS`, set `HERALD_HMAC_DEFAULT_KEY_ID` or send `X-Key-Id`; Herald never selects the first Go map entry. Legacy v1 is disabled unless `HMAC_V1_ENABLED=true`.

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

**Headers:**
- `Idempotency-Key` (optional): A unique key to ensure idempotent requests. If provided, duplicate requests with the same key within the TTL will return the same challenge response without creating a new challenge or sending a new code.

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

**Channel:** `channel` must be `"sms"`, `"email"`, or `"dingtalk"`. When `channel` is `"email"` and `HERALD_SMTP_API_URL` is set, Herald forwards the send to [herald-smtp](https://github.com/soulteary/herald-smtp); `destination` is the email address. When `channel` is `"dingtalk"`, Herald forwards the send to [herald-dingtalk](https://github.com/soulteary/herald-dingtalk) (configure `HERALD_DINGTALK_API_URL`); `destination` is the DingTalk userid (or 11-digit mobile when herald-dingtalk is in mobile lookup mode). Herald does not store any SMTP or DingTalk credentials.

**Response:**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

When `HERALD_TEST_MODE=true`, the response also includes `debug_code` (the plain verification code) so callers (e.g. Stargate in debug mode) can display it for local/testing. **Do not enable test mode in production.**

**Error Responses:**

All error responses follow this format:
```json
{
  "ok": false,
  "reason": "error_code",
  "error": "optional error message"
}
```

Possible error codes:
- `invalid_request`: Request body parsing failed
- `user_id_required`: Missing required field `user_id`
- `invalid_channel`: Invalid channel type (must be "sms", "email", or "dingtalk")
- `invalid_purpose`: Invalid purpose value (must be one of the allowed purposes)
- `destination_required`: Missing required field `destination`
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired
- `user_locked`: User is temporarily locked
- `send_failed`: Failed to send verification code via provider
- `internal_error`: Internal server error

HTTP Status Codes:
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Authentication failed
- `403 Forbidden`: User locked
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Internal server error

### Get Test Code (Test Mode Only)

**GET /v1/test/code/:challenge_id**

Returns the verification code for a challenge. **Only available when `HERALD_TEST_MODE=true`.** Used by integration tests or by Stargate in debug mode when the create-challenge response did not include `debug_code` (e.g. when using an older Herald client). Do not enable test mode in production.

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

Verify a challenge code.

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
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid
- `expired`: Challenge has expired
- `invalid`: Invalid verification code
- `locked`: Challenge locked due to too many attempts
- `verification_failed`: General verification failure
- `internal_error`: Internal server error

HTTP Status Codes:
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Verification failed
- `403 Forbidden`: User locked
- `500 Internal Server Error`: Internal server error

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
- `internal_error`: Internal server error

HTTP Status Codes:
- `400 Bad Request`: Invalid request
- `500 Internal Server Error`: Internal server error

### TOTP Proxy (Optional)

When `HERALD_TOTP_ENABLED=true` and `HERALD_TOTP_BASE_URL` is set, Herald proxies TOTP (Authenticator) operations to [herald-totp](https://github.com/soulteary/herald-totp). All TOTP routes require the same authentication as OTP routes (mTLS, HMAC, or API Key).

#### Get TOTP Status

**GET /v1/totp/status**

Query whether the subject has TOTP enrolled.

**Query:**
- `subject` (required): User identifier (e.g. user_id)

**Response (Success):** Proxied from herald-totp (e.g. `{"enrolled": true}` or `{"enrolled": false}`).

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

**Response:** Proxied from herald-totp (e.g. `{"ok": true}` on success, or `{"ok": false, "reason": "..."}` on failure). On proxy error, Herald returns `502` with `proxy_failed`.

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

**Response:** Proxied from herald-totp. On failure, Herald may return `400` with `invalid` or proxy error.

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

**TOTP error codes (from Herald proxy):**
- `totp_not_configured`: TOTP not enabled or herald-totp URL not set
- `proxy_failed`: Request to herald-totp failed
- `invalid_request`: Missing or invalid request body/query

## Rate Limiting

Herald implements multi-dimensional rate limiting:

- **Per User**: 10 requests per hour (configurable)
- **Per IP**: 5 requests per minute (configurable)
- **Per Destination**: 10 requests per hour (configurable)
- **Resend Cooldown**: 60 seconds between resends

## Error Codes

This section lists all possible error codes returned by the API.

### Request Validation Errors
- `invalid_request`: Request body parsing failed or invalid JSON
- `user_id_required`: Missing required field `user_id`
- `invalid_channel`: Invalid channel type (must be "sms", "email", or "dingtalk")
- `invalid_purpose`: Invalid purpose value (must be one of the allowed purposes)
- `destination_required`: Missing required field `destination`
- `challenge_id_required`: Missing required field `challenge_id`
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid

### Authentication Errors
HMAC/API-key failures use a JSON `reason` value:
- `signature_version_required`: HMAC v2 requires `X-Signature-Version: v2`
- `missing_auth_headers`: One or more HMAC v2 headers are missing
- `key_id_required` / `invalid_key_id`: A usable HMAC key ID was not supplied
- `timestamp_out_of_range`: Timestamp is outside the configured drift window
- `invalid_signature`: HMAC signature verification failed
- `replayed_nonce`: The nonce was already consumed
- `nonce_store_unavailable`: Redis nonce storage failed in fail-closed mode
- `unauthorized`: Authentication failed

### Challenge Errors
- `expired`: Challenge has expired
- `invalid`: Invalid verification code
- `locked`: Challenge locked due to too many attempts
- `too_many_attempts`: Too many failed attempts (may be included in `locked`)
- `verification_failed`: General verification failure
- `send_failed`: Failed to send verification code via provider (only during challenge creation)

### Rate Limiting Errors
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired

### User Status Errors
- `user_locked`: User is temporarily locked

### System Errors
- `internal_error`: Internal server error
