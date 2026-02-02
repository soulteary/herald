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
   - Check SMTP settings (for email): `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`
   - Check SMS provider settings: `SMS_PROVIDER`, `ALIYUN_ACCESS_KEY`, etc.
   - For DingTalk: ensure `HERALD_DINGTALK_API_URL` is set and herald-dingtalk is reachable; optionally `HERALD_DINGTALK_API_KEY` if herald-dingtalk uses API key auth
   - Verify provider credentials are correct

3. **Check Prometheus Metrics**
   ```promql
   # Check send failure rate
   rate(herald_otp_sends_total{result="failed"}[5m]) / rate(herald_otp_sends_total[5m])
   
   # Check send success by provider
   rate(herald_otp_sends_total{result="success"}[5m]) by (provider)
   ```

4. **Check Audit Logs**
   - Review audit logs for `send_failed` events
   - Check provider error codes and messages
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

## Verification Code Errors

### Symptoms
- User reports "invalid code" errors
- Verification fails even with correct code
- Challenge appears expired or locked

### Diagnostic Steps

1. **Check Challenge Status**
   ```bash
   # Check Redis for challenge data
   redis-cli GET "otp:ch:{challenge_id}"
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
   rate(herald_otp_verifications_total{result="failed"}[5m]) by (reason)
   
   # Check locked challenges
   rate(herald_otp_verifications_total{result="failed",reason="locked"}[5m])
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
   - Verify which authentication method is being used (mTLS, HMAC, API Key)
   - Check if authentication headers are present

2. **For HMAC Authentication**
   ```bash
   # Verify HMAC_SECRET is configured
   echo $HMAC_SECRET
   
   # Check timestamp is within 5-minute window
   # Verify signature calculation matches Herald's implementation
   ```

3. **For API Key Authentication**
   ```bash
   # Verify API_KEY is set
   echo $API_KEY
   
   # Check X-API-Key header matches configured key
   ```

4. **For mTLS Authentication**
   - Verify client certificate is valid
   - Check certificate chain is trusted
   - Verify TLS connection is established

5. **Check Herald Logs**
   ```bash
   # Check for authentication errors
   grep "unauthorized\|invalid_signature\|timestamp_expired" /var/log/herald.log
   ```

### Solutions

- **Missing Credentials**: Set `API_KEY` or `HMAC_SECRET` environment variables
- **Invalid Signature**: Verify HMAC signature calculation matches Herald's implementation
- **Timestamp Expired**: Ensure client and server clocks are synchronized (within 5 minutes)
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
   rate(herald_rate_limit_hits_total[5m]) by (scope)
   
   # Check rate limit hit rate
   rate(herald_rate_limit_hits_total[5m])
   ```

3. **Review Redis Keys**
   ```bash
   # Check rate limit keys
   redis-cli KEYS "otp:rate:*"
   
   # Check specific rate limit key
   redis-cli GET "otp:rate:user:{user_id}"
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
   # Test Redis connection
   redis-cli -h $REDIS_ADDR -p 6379 PING
   ```

2. **Verify Configuration**
   ```bash
   # Check Redis configuration
   echo $REDIS_ADDR
   echo $REDIS_PASSWORD
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
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   
   # Check Redis operation errors
   rate(herald_redis_latency_seconds_count[5m])
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
- Provider-specific error messages

### Diagnostic Steps

1. **Check Provider Metrics**
   ```promql
   # Check send failure rate by provider
   rate(herald_otp_sends_total{result="failed"}[5m]) by (provider)
   
   # Check send duration by provider
   histogram_quantile(0.95, rate(herald_otp_send_duration_seconds_bucket[5m])) by (provider)
   ```

2. **Review Provider Logs**
   - Check provider-specific error messages
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
   histogram_quantile(0.99, rate(herald_redis_latency_seconds_bucket[5m]))
   ```

2. **Check Resource Usage**
   ```bash
   # Check CPU and memory
   top -p $(pgrep herald)
   
   # Check Goroutine count
   curl http://localhost:8082/debug/pprof/goroutine?debug=1
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
- **Optimize Redis**: Use Redis Cluster or optimize queries
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
