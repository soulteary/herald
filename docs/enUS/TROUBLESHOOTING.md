# Herald Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Herald OTP and verification code service.

## Table of Contents

- [Verification Code Not Received](#verification-code-not-received)
- [Verification Code Errors](#verification-code-errors)
- [401 Unauthorized Errors](#401-unauthorized-errors)
- [Rate Limiting Issues](#rate-limiting-issues)
- [Redis Connection Issues](#redis-connection-issues)
- [Provider Send Failures](#provider-send-failures)
- [Performance Issues](#performance-issues)

## Verification Code Not Received

### Symptoms
- User reports not receiving verification code via SMS, email, or DingTalk
- Challenge creation succeeds but no code is delivered

### Diagnostic Steps

1. **Check Provider Connection**
   ```bash
   # Check Herald logs for provider errors
   grep "send_failed\|provider" /var/log/herald.log
   ```

2. **Verify Provider Configuration**
   - Check SMTP settings (for email when using built-in SMTP): `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - For email via herald-smtp: ensure `HERALD_SMTP_API_URL` is set and herald-smtp is reachable; optionally `HERALD_SMTP_API_KEY` if herald-smtp uses API key auth. If emails are not received, check herald-smtp logs and its SMTP configuration (see [herald-smtp Troubleshooting](https://github.com/soulteary/herald-smtp/blob/main/docs/enUS/TROUBLESHOOTING.md)).
   - Check SMS provider settings: `SMS_PROVIDER`, `SMS_API_BASE_URL`, `SMS_API_KEY` (Herald uses HTTP API mode; credentials are held by the SMS gateway, not Herald).
   - For DingTalk: ensure `HERALD_DINGTALK_API_URL` is set and herald-dingtalk is reachable; optionally `HERALD_DINGTALK_API_KEY` if herald-dingtalk uses API key auth
   - Verify provider credentials are correct

3. **Check Prometheus Metrics**
   ```promql
   # Check send failure rate
   sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m])) / sum by (provider) (rate(herald_otp_sends_total[5m]))
   
   # Check send success by provider
   sum by (provider) (rate(herald_otp_sends_total{result="success"}[5m]))
   ```

4. **Check Audit Logs**
   - Review audit logs for `send_failed` events
   - Check bounded provider `reason` codes; raw provider messages are not exported
   - Verify destination addresses are valid

5. **Test Provider Directly**
   - Test SMTP connection manually
   - Test SMS provider API directly
   - Verify network connectivity to provider endpoints

### Solutions

- **Provider Configuration Issues**: Update provider credentials and settings
- **Network Issues**: Check firewall rules and network connectivity
- **Provider Rate Limits**: Check if provider has rate limits that are being exceeded
- **Invalid Destinations**: Verify email addresses and phone numbers are valid

### Local/Testing: Display verification code without provider

For local or integration testing when you do not want to rely on real SMS/email delivery:

1. Set **Herald** `ENVIRONMENT=test`, `HERALD_TEST_MODE=true` and `HERALD_TEST_API_KEY`. Herald will then return the verification code in the create-challenge response (`debug_code`) and expose `GET /v1/test/code/:challenge_id` only on the separate test listener, with test-key authentication. With the non-production `PROVIDER_FAILURE_POLICY=soft` default, a provider failure can still return a verifiable challenge with `delivery_status=failed`. Strict policy requires successful delivery; production forbids soft policy.
2. Set **Stargate** `DEBUG=true`. Stargate will include the verification code in the `/_send_verify_code` response and display it on the login page (e.g. "Verification code (debug): 123456").

See [API.md](API.md) (Create Challenge response, Get Test Code) and [DEPLOYMENT.md](DEPLOYMENT.md) (Test mode and debugging). **Never enable test mode or DEBUG in production.**

## Verification Code Errors

### Symptoms
- User reports "invalid code" errors
- Verification fails even with correct code
- Challenge appears expired or locked

### Diagnostic Steps

1. **Check Challenge Status**
   ```bash
   # Check Redis for challenge data
   redis-cli GET "otp2:ch:{challenge_id}"
   ```

2. **Verify Challenge Expiry**
   - Check `CHALLENGE_EXPIRY` configuration (default: 5 minutes)
   - Verify system time is synchronized (NTP)
   - Check if challenge has expired

3. **Check Attempt Count**
   - Verify `MAX_ATTEMPTS` configuration (default: 5)
   - Check if challenge is locked due to too many attempts
   - Review audit logs for attempt history

4. **Check Prometheus Metrics**
   ```promql
   # Check verification failure reasons
   sum by (reason) (rate(herald_otp_verifications_total{result="failure"}[5m]))
   
   # Check locked challenges
   rate(herald_otp_verifications_total{result="failure",reason="locked"}[5m])
   ```

5. **Review Audit Logs**
   - Check verification attempts and results
   - Verify code format matches expected format
   - Check for timing issues

### Solutions

- **Expired Challenge**: User needs to request a new verification code
- **Too Many Attempts**: Challenge is locked, wait for lockout duration or request new challenge
- **Code Format Mismatch**: Verify code length matches `CODE_LENGTH` configuration
- **Time Synchronization**: Ensure system clocks are synchronized

## 401 Unauthorized Errors

### Symptoms
- API requests return 401 Unauthorized
- Authentication failures in logs
- Service-to-service communication fails

### Diagnostic Steps

1. **Check Authentication Method**
   - Check `REQUEST_AUTH_MODE` (HMAC or API Key); mTLS is a separate transport policy
   - Check if authentication headers are present

2. **For HMAC Authentication**
   ```bash
   # Verify HMAC_SECRET is configured
   test -n "$HMAC_SECRET" || test -n "$HERALD_HMAC_KEYS"
   
   # Check timestamp is within HMAC_MAX_DRIFT window (default 60 seconds)
   # Verify signature calculation matches Herald's implementation
   ```

3. **For API Key Authentication**
   ```bash
   # Verify API_KEY is set
   test -n "$API_KEY"
   
   # Check X-API-Key header matches configured key
   ```

4. **For the Additional mTLS Transport Policy**
   - Verify client certificate is valid
   - Check certificate chain is trusted
   - Verify TLS connection is established
   - A successful client certificate check still requires the selected HMAC or API-key credentials.

5. **Check Herald Logs**
   ```bash
   # Check for authentication errors
   rg '"status":401' /var/log/herald.log
   ```

### Solutions

- **Missing Credentials**: Set `API_KEY` or `HMAC_SECRET` environment variables
- **Invalid Signature**: Verify HMAC signature calculation matches Herald's implementation
- **Timestamp Expired**: Ensure client and server clocks are synchronized (within HMAC_MAX_DRIFT; default 60 seconds)
- **Certificate Issues**: Verify mTLS certificates are valid and trusted

## Rate Limiting Issues

### Symptoms
- Users report "rate limit exceeded" errors
- Legitimate requests are being blocked
- High rate limit hit metrics

### Diagnostic Steps

1. **Check Rate Limit Configuration**
   ```bash
   # Verify rate limit settings
   echo $RATE_LIMIT_PER_USER
   echo $RATE_LIMIT_PER_IP
   echo $RATE_LIMIT_PER_DESTINATION
   echo $RESEND_COOLDOWN
   ```

2. **Check Prometheus Metrics**
   ```promql
   # Check rate limit hits by scope
   sum by (scope) (rate(herald_rate_limit_hits_total[5m]))
   
   # Check rate limit hit rate
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Review Redis Keys**
   ```bash
   # Check rate limit keys
   redis-cli --scan --pattern "ratelimit:*"
   
   # Check specific rate limit key
   redis-cli TYPE "ratelimit:user:{peppered_user_digest}"
   redis-cli GET "ratelimit:user:{peppered_user_digest}"
   redis-cli PTTL "ratelimit:user:{peppered_user_digest}"
   ```

4. **Analyze Usage Patterns**
   - Check if legitimate users are hitting limits
   - Identify potential abuse or bot activity
   - Review challenge creation patterns

### Solutions

- **Adjust Rate Limits**: Increase limits if legitimate users are affected
- **User Education**: Inform users about rate limits and cooldown periods
- **Abuse Prevention**: Implement additional security measures for suspected abuse
- **Resend Cooldown**: Users must wait for `RESEND_COOLDOWN` period (default: 60 seconds)

## Redis Connection Issues

### Symptoms
- Service fails to start
- Health check returns unhealthy
- Challenge creation/verification fails
- High Redis latency metrics

### Diagnostic Steps

1. **Check Redis Connectivity**
   ```bash
   # Replace the host/port below with the separate components of REDIS_ADDR.
   # Use REDISCLI_AUTH for the password; do not print it or put it in a URI.
   # Run redis-cli on the Redis network; Compose does not publish Redis by default.
   REDISCLI_AUTH="$REDIS_PASSWORD" redis-cli -h redis.example.internal -p 6379 -n "${REDIS_DB:-0}" PING
   ```

2. **Verify Configuration**
   ```bash
   # Check Redis configuration
   echo $REDIS_ADDR
   test -n "$REDIS_PASSWORD" && echo "REDIS_PASSWORD is set"
   echo $REDIS_DB
   ```

3. **Check Redis Health**
   ```bash
   # Check Redis info
   redis-cli INFO
   
   # Check memory usage
   redis-cli INFO memory
   ```

4. **Check Prometheus Metrics**
   ```promql
   # Check Redis latency
   histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m]))
   
   # Check Redis operation errors
   sum by (operation) (rate(herald_redis_operations_total{result="failure"}[5m]))
   ```

5. **Review Herald Logs**
   ```bash
   # Check for Redis errors
   grep "Redis\|redis" /var/log/herald.log | grep -i error
   ```

### Solutions

- **Connection Issues**: Verify `REDIS_ADDR` is correct and Redis is accessible
- **Authentication Issues**: Check `REDIS_PASSWORD` is correct
- **Database Selection**: Verify `REDIS_DB` is correct (default: 0)
- **Performance Issues**: Check Redis memory usage and consider scaling
- **Network Issues**: Verify network connectivity and firewall rules

## Provider Send Failures

### Symptoms
- High send failure rate in metrics
- `send_failed` errors in logs
- Bounded provider failure reason codes

### Diagnostic Steps

1. **Check Provider Metrics**
   ```promql
   # Check send failure rate by provider
   sum by (provider) (rate(herald_otp_sends_total{result="failure"}[5m]))
   
   # Check send duration by provider
   histogram_quantile(0.95, sum by (le, provider) (rate(herald_otp_send_duration_seconds_bucket[5m])))
   ```

2. **Review Provider Logs**
   - Check the safe `reason` field; raw provider messages are not exported
   - Review audit logs for provider errors
   - Check provider API response codes

3. **Test Provider Connection**
   - Test SMTP connection manually
   - Test SMS provider API directly
   - Verify provider credentials

4. **Check Provider Status**
   - Check provider status page
   - Verify provider API is operational
   - Check for provider rate limits

### Solutions

- **Provider Outage**: Wait for provider to restore service
- **Credential Issues**: Update provider credentials
- **Rate Limits**: Check if provider rate limits are exceeded
- **Configuration Issues**: Verify provider configuration is correct
- **Network Issues**: Check network connectivity to provider

## Performance Issues

### Symptoms
- Slow response times
- High latency metrics
- Timeout errors
- High resource usage

### Diagnostic Steps

1. **Check Response Times**
   ```promql
   # Check send duration
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m]))
   
   # Check Redis latency
   histogram_quantile(0.99, rate(herald_redis_operation_duration_seconds_bucket[5m]))
   ```

2. **Check Resource Usage**
   ```bash
   # Check CPU and memory
   top -p $(pgrep herald)
   
   # Inspect service metrics; pprof and Go runtime collectors are not mounted
   curl -fsS http://localhost:8082/metrics | rg "^herald_"
   ```

3. **Review Request Patterns**
   - Check request rate
   - Identify peak usage times
   - Check for traffic spikes

4. **Check Redis Performance**
   ```bash
   # Check Redis slow log
   redis-cli SLOWLOG GET 10
   
   # Check Redis memory
   redis-cli INFO memory
   ```

### Solutions

- **Scale Resources**: Increase CPU/memory allocation
- **Optimize Redis**: Use a managed single Redis endpoint and inspect slow operations; native Cluster/Sentinel discovery is not configured
- **Provider Optimization**: Use faster providers or optimize provider calls
- **Caching**: Implement caching for frequently accessed data
- **Load Balancing**: Distribute load across multiple instances

## Getting Help

If you're unable to resolve an issue:

1. **Check Logs**: Review Herald logs for detailed error messages
2. **Check Metrics**: Review Prometheus metrics for patterns
3. **Review Documentation**: Check [API.md](API.md) and [DEPLOYMENT.md](DEPLOYMENT.md)
4. **Open an Issue**: Create an issue with:
   - Error messages and logs
   - Configuration (without secrets)
   - Steps to reproduce
   - Expected vs actual behavior

## Related Documentation

- [API Documentation](API.md) - API endpoint details
- [Deployment Guide](DEPLOYMENT.md) - Deployment and configuration
- [Monitoring Guide](MONITORING.md) - Monitoring and metrics

## Provider failure diagnostics

Delivery failures are logged with a bounded `reason` field, not the upstream
message. Span status and exception events use the same safe reason. Do not
enable raw provider-body logging to investigate failures. Review historical
logs and exported spans for echoed sensitive data and rotate exposed credentials.
See [security](SECURITY.md#provider-failure-diagnostics) for compatibility details.

## HMAC identity and CORS upgrade checks

- `401/invalid_signature` while `herald_auth_failure_total{reason="invalid_key_id"}` increases: the request names a key ID that is not configured (earlier releases answered `401/invalid_key_id`). With only `HMAC_SECRET`, omit the ID or use exactly `default`. Provision known custom IDs in `HERALD_HMAC_KEYS` without renaming callers. Do not change IDs to retry an uncertain send.
- `409/idempotency_namespace_conflict`: an ambiguous historical empty-key-ID record exists. Preserve its bytes and TTL and reconcile it against provider/audit evidence; it is not a retryable delivery failure.
- `409/delivery_unknown`: delivery may already have happened. Keep the same identity and client key; do not delete an unresolved sending record or force a replacement send. The record expires after `IDEMPOTENCY_UNRESOLVED_TTL` (default `24h`).
- A production CORS startup error requires removing every standalone `*` entry from the origin list, or disabling CORS. Unlisted origins must not receive wildcard responses; API Key/HMAC authentication still applies.

Follow the [HMAC upgrade procedure](DEPLOYMENT.md#hmac-identity-upgrade) and [CORS requirements](DEPLOYMENT.md#production-cors-origin-lists), including before rollback.
