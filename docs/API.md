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
- `400 Bad Request`: Invalid request
- `401 Unauthorized`: Authentication failed
- `429 Too Many Requests`: Rate limit exceeded
- `403 Forbidden`: User locked

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
  "reason": "expired|invalid|locked|too_many_attempts"
}
```

**Error Responses:**
- `400 Bad Request`: Invalid request
- `401 Unauthorized`: Verification failed
- `403 Forbidden`: User locked

### Revoke Challenge

**POST /v1/otp/challenges/{id}/revoke**

Revoke a challenge (optional).

**Response:**
```json
{
  "ok": true
}
```

## Rate Limiting

Herald implements multi-dimensional rate limiting:

- **Per User**: 10 requests per hour (configurable)
- **Per IP**: 5 requests per minute (configurable)
- **Per Destination**: 10 requests per hour (configurable)
- **Resend Cooldown**: 60 seconds between resends

## Error Codes

- `expired`: Challenge has expired
- `invalid`: Invalid verification code
- `locked`: User is temporarily locked
- `too_many_attempts`: Too many failed attempts
- `rate_limit_exceeded`: Rate limit exceeded
- `user_locked`: User is locked
- `resend_cooldown`: Resend cooldown period not expired
