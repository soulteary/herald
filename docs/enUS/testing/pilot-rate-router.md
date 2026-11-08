# Synthetic pilot rate-limit router regression

Run the fixed integration matrix with an existing Go toolchain matching `go.mod`:

```sh
python3 -I -B scripts/test-pilot-rate-router.py
```

The runner uses `go` from PATH and normal Go module/cache settings. It installs nothing. A local caller with a complete module cache may explicitly set `GOPROXY=off GOSUMDB=off`; CI downloads the existing module dependencies before running and does not require an offline cache. `--output-parent <directory>` places a **new** temporary evidence directory below the specified parent. Existing evidence is never overwritten.

The build tag `pilot_rate_integration` excludes this file from ordinary `go test ./...`. The runner compiles one race-enabled test binary and starts four selected scenarios in separate processes. Do not run the tagged cases together in one test process: native audit initialization is process-global, and stopping it does not create a fresh audit writer for a later router. Each scenario retains native audit enabled, owns its isolated spool and exits after normal router shutdown. This arrangement changes no application or audit behavior.

## Actual path and controlled collaborators

Each scenario constructs `NewRouterWithClientAndHandlersE`, the production Fiber router, and listens on loopback. Requests pass through native API-key authentication, trusted-proxy configuration and sanitizer, challenge handler, peppered rate manager, audit path and provider-kit HTTP sender. Only Redis (miniredis) and the final successful DingTalk `/v1/send` HTTP response are fixtures. Users, destinations, API keys and OTP messages are synthetic. No real IM, email, remote deployment, private image or application database is contacted.

The four fixed scenarios assert:

1. Configured five-per-minute IP quota aggregates distinct users behind one untrusted source. Five creations reach the provider fixture; the sixth returns 429 with retry metadata. Rotating JSON `client_ip` and untrusted X-Forwarded-For cannot bypass the bucket.
2. With a deliberately trusted loopback proxy, distinct rightmost untrusted client addresses have separate buckets. Forging a leftmost chain element does not reset the same client's bucket.
3. Per-account quota still denies the same account using another receiver and another trusted client IP, while an unrelated account succeeds.
4. Per-receiver quota still denies another account using the same receiver and a different trusted client IP, while an unrelated receiver succeeds.

A positive cooldown is configured; the tests do not disable native resend-cooldown validation. Each scenario requires exactly one PASS record for its fixed full test path, an overall PASS, zero exit and no skipped or failed test. A parent-only PASS does not satisfy the required `/user` or `/destination` child. CI first runs offline parser controls, including parent-only, wrong-child, skip/failure and captured actual-log positives. A missing selection or helper skip cannot produce a passing matrix.

The separate CI job uploads only result JSON, compiler log and test logs. It excludes the executable and synthetic audit files. Evidence records the native test selections and exit codes; it does not contain deployment addresses or credentials.

## Limits of this evidence

This is a small correctness matrix, **not** a 200-user throughput benchmark or real provider acceptance. It uses supported API-key authentication and does not qualify HMAC, service-policy or TLS configuration. Configured values in the fixture do not establish effective limits or proxy trust on any deployed version. JSON `client_ip` remains advisory: do not make it trusted merely to increase throughput. A gateway deployment needs independently verified trust topology and an aggregate quota justified against expected peak traffic, while retaining account and destination limits.

Changing live quota, proxy trust, messaging configuration or identity mapping remains a separate deployment decision. No such setting is changed by these tests.
