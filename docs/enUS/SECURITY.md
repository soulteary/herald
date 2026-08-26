# Security Documentation

> 🌐 **Language / 语言**: [English](SECURITY.md) | [中文](../zhCN/SECURITY.md) | [Français](../frFR/SECURITY.md) | [Italiano](../itIT/SECURITY.md) | [日本語](../jaJP/SECURITY.md) | [Deutsch](../deDE/SECURITY.md) | [한국어](../koKR/SECURITY.md)

This document explains Herald's security features, security configuration, and best practices.

## Implemented Security Features

1. **Challenge-Based Verification**: Uses challenge-verify model to prevent replay attacks and ensure one-time use of verification codes
2. **Secure Code Storage**: Verification codes are stored as Argon2 hashes, never in plaintext
3. **Multi-Dimensional Rate Limiting**: Rate limiting by user_id, destination (email/phone), and IP address to prevent abuse
4. **Service Authentication**: Supports mTLS, HMAC signature, and API Key authentication for inter-service communication
5. **Idempotency Protection**: Prevents duplicate challenge creation and code sending using idempotency keys
6. **Challenge Expiration**: Automatic expiration of challenges with configurable TTL
7. **Attempt Limiting**: Maximum attempt limits per challenge to prevent brute force attacks
8. **Resend Cooldown**: Prevents rapid resend of verification codes
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
export HERALD_HMAC_KEYS='{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}'
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
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
- Use strong HMAC secrets (minimum 32 characters)
- Disable insecure authentication methods in production (use mTLS or HMAC, avoid API Key if possible)

### 4. Rate Limiting Configuration

Herald implements multi-dimensional rate limiting to prevent abuse:

**Rate Limit Dimensions**:
- **Per User ID**: Limits challenges per user_id per time window
- **Per Destination**: Limits challenges per email/phone number per time window
- **Per IP Address**: Limits challenges per client IP per time window

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

### HMAC v2 Authentication

HMAC v2 signs the full request shape and body digest:

```text
HERALD-HMAC-V2
METHOD
PATH
QUERY
TIMESTAMP
NONCE
SERVICE
KEY_ID
SHA256_HEX(RAW_BODY)
```

**Required request headers**:
- `X-Signature-Version: v2`
- `X-Signature`: lowercase hexadecimal HMAC-SHA256 of the canonical string
- `X-Timestamp`: Unix timestamp in seconds
- `X-Nonce`: unique value for this request
- `X-Service`: caller service identifier
- `X-Key-Id`: key ID; it may be omitted only when `HERALD_HMAC_DEFAULT_KEY_ID` is configured

**Server configuration**:
```bash
export REQUEST_AUTH_MODE=hmac_v2
export HERALD_HMAC_KEYS='{"key-id-1":"secret-key-1","key-id-2":"secret-key-2"}'
export HERALD_HMAC_DEFAULT_KEY_ID=key-id-1
export HMAC_MAX_DRIFT=60s
export HMAC_V1_ENABLED=false
```

Herald verifies the signature before atomically consuming the nonce in Redis. Reusing a nonce is rejected. The default timestamp drift is 60 seconds. Legacy v1 is disabled by default and should only be enabled temporarily during migration with `HMAC_V1_ENABLED=true`.

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
- Consider using Redis over TLS for production

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

- Use secure SMTP connections (TLS/SSL)
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
4. **Resend Cooldown**: Prevents rapid resend of codes to the same challenge
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

## Error Handling

### Production Mode

In production (with `ENVIRONMENT=production`; `HERALD_TEST_MODE=true` is rejected):

- Hide detailed error information to prevent information leakage
- Return generic error messages
- Detailed error information is only recorded in logs
- Audit logs contain full details for security analysis

### Development Mode

In development mode:

- Display detailed error information for debugging
- Include stack trace information
- More verbose logging

## Security Response Headers

Herald automatically adds the following security-related HTTP response headers:

- `X-Content-Type-Options: nosniff` - Prevents MIME type sniffing
- `X-Frame-Options: DENY` - Prevents clickjacking
- `X-XSS-Protection: 1; mode=block` - XSS protection

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

## Related Documentation

- [API Documentation](API.md) - Learn about API security features and authentication
- [Deployment Documentation](DEPLOYMENT.md) - Learn about production environment deployment recommendations
- [Monitoring Documentation](MONITORING.md) - Learn about security monitoring and alerting
- [Troubleshooting Documentation](TROUBLESHOOTING.md) - Learn about security-related troubleshooting
