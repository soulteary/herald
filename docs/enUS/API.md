# Herald API Documentation

Herald is a verification code and OTP service that handles sending verification codes via SMS and email, with built-in rate limiting and security controls.

## Base URL

```
http://localhost:8082
```

## Authentication

Herald supports three authentication methods with the following priority order:

1. **mTLS** (Most Secure): Mutual TLS with client certificate verification (highest priority)
2. **HMAC Signature** (Secure): Set `X-Signature`, `X-Timestamp`, and `X-Service` headers
3. **API Key** (Simple): Set `X-API-Key` header (lowest priority)

### mTLS Authentication

When using HTTPS with a verified client certificate, Herald will automatically authenticate the request via mTLS. This is the most secure method and takes priority over other authentication methods.

### HMAC Signature

The HMAC signature is computed as:
```
HMAC-SHA256(timestamp:service:body, secret)
```

Where:
- `timestamp`: Unix timestamp (seconds)
- `service`: Service identifier (e.g., "my-service", "api-gateway")
- `body`: Request body (JSON string)
- `secret`: HMAC secret key

**Note**: The timestamp must be within 5 minutes (300 seconds) of the server time to prevent replay attacks. The timestamp window is configurable but defaults to 5 minutes.

**Note**: `X-Key-Id` header is supported for key rotation. When using `HERALD_HMAC_KEYS` with multiple keys, you can specify which key to use via the `X-Key-Id` header. If not provided, the default key (first key in the map) will be used.

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

**Response:**
```json
{
  "challenge_id": "ch_7f9b...",
  "expires_in": 300,
  "next_resend_in": 60
}
```

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
- `invalid_channel`: Invalid channel type (must be "sms" or "email")
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
- `invalid_channel`: Invalid channel type (must be "sms" or "email")
- `invalid_purpose`: Invalid purpose value (must be one of the allowed purposes)
- `destination_required`: Missing required field `destination`
- `challenge_id_required`: Missing required field `challenge_id`
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid

### Authentication Errors
- `authentication_required`: No valid authentication provided
- `invalid_timestamp`: Invalid timestamp format
- `timestamp_expired`: Timestamp is outside the allowed window (5 minutes)
- `invalid_signature`: HMAC signature verification failed
- `unauthorized`: Authentication failed (generic authentication error)

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
