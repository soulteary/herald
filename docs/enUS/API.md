# Herald API Documentation

Herald is a verification code and OTP service that handles sending verification codes via SMS and email, with built-in rate limiting and security controls.

## Base URL

```
http://localhost:8082
```

## Authentication

Herald supports two authentication methods:

1. **API Key** (Simple): Set `X-API-Key` header
2. **HMAC Signature** (Secure): Set `X-Signature`, `X-Timestamp`, and `X-Service` headers

### HMAC Signature

The HMAC signature is computed as:
```
HMAC-SHA256(timestamp:service:body, secret)
```

Where:
- `timestamp`: Unix timestamp (seconds)
- `service`: Service identifier (e.g., "stargate")
- `body`: Request body (JSON string)
- `secret`: HMAC secret key

## Endpoints

### Health Check

**GET /health**

Check service health.

**Response:**
```json
{
  "status": "ok",
  "service": "herald"
}
```

### Create Challenge

**POST /v1/otp/challenges**

Create a new verification challenge and send verification code.

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
- `destination_required`: Missing required field `destination`
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired
- `user_locked`: User is temporarily locked
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
- `destination_required`: Missing required field `destination`
- `challenge_id_required`: Missing required field `challenge_id`
- `code_required`: Missing required field `code`
- `invalid_code_format`: Verification code format is invalid

### Authentication Errors
- `authentication_required`: No valid authentication provided
- `invalid_timestamp`: Invalid timestamp format
- `timestamp_expired`: Timestamp is outside the allowed window (5 minutes)
- `invalid_signature`: HMAC signature verification failed

### Challenge Errors
- `expired`: Challenge has expired
- `invalid`: Invalid verification code
- `locked`: Challenge locked due to too many attempts
- `too_many_attempts`: Too many failed attempts (may be included in `locked`)
- `verification_failed`: General verification failure

### Rate Limiting Errors
- `rate_limit_exceeded`: Rate limit exceeded
- `resend_cooldown`: Resend cooldown period not expired

### User Status Errors
- `user_locked`: User is temporarily locked

### System Errors
- `internal_error`: Internal server error
