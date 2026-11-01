# Changelog

All notable changes to Herald are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Revoke the superseded OTP and publish its replacement active index in one
  Redis Lua operation. A failed standalone revocation or a Herald process exit
  can no longer leave both codes redeemable after a successful resend. Existing
  active-index encodings, challenge records, expiry settings and idempotency
  delivery-unknown protection are retained.

- Bind the idempotency header (including its absence) in HMAC v3. HMAC challenge
  creation requires v3 so changing or stripping the header cannot redirect a
  signed request into another idempotency record and cause a duplicate send.
  Go SDK signing defaults to v3. Older HMAC creation callers must upgrade;
  v2 remains accepted on other routes. API key mode and Redis records are unchanged.

- Reject `X-Service` and `X-Key-Id` values containing the reserved `|kid:`
  delimiter before authentication, nonce consumption, or business writes.
  Otherwise different service/key pairs could share an idempotency namespace
  and replay a response across HMAC credentials. Rejected values return
  `401/invalid_auth_headers`; ordinary namespaces and existing Redis records
  retain their format, contents, and TTL.
- Use length-framed, versioned idempotency fingerprints for requests containing
  embedded NUL characters. Moving separator bytes between user and locale
  fields can no longer replay a response for a different request. Ordinary
  request fingerprints and Redis keys remain unchanged. Existing legacy NUL
  records return `409/idempotency_conflict` and retain their contents and TTL;
  non-expiring sending records must be reconciled rather than cleared to retry.
- Harden all TOTP proxy calls with a redirect-denying HTTP client, verified
  TLS, bounded response reads, and strict HTTP 200 response handling. Failed
  or incomplete upstream responses cannot become successful verification
  results, and redirects cannot forward TOTP codes or service credentials.
- Apply `PROVIDER_MAX_RESPONSE_BYTES` to TOTP responses as well. Redirects,
  non-200 responses, oversized bodies, and malformed or interrupted responses
  now return `502/proxy_failed`. Valid `200/ok:false` verification results and
  complete enrollment-confirm HTTP 400 responses retain their existing contract.

## [1.3.9] - 2026-11-01

### Changed

- Publish the maintained Go module dependency graph used by the bundled SDK
  (`github.com/soulteary/herald/pkg/herald`) and the Herald service. Upgrade
  `challenge-kit` to v1.10.4 and `provider-kit` to v1.7.4, together with the
  released audit, cli, health, i18n, logger, metrics, middleware, redis, secure,
  tracing and version kit patches. Keep `http-kit/v2` at v2.1.0.
- Refresh module checksums, including the health-kit test graph's SQLite and
  libc patches. Retain Go 1.27.3 and the SDK's public API. No SDK or service
  production source changes are introduced after v1.3.8.

### Compatibility

- SDK and service share the root module version; consumers import
  `github.com/soulteary/herald/pkg/herald` from module
  `github.com/soulteary/herald@v1.3.9`.
- Upgrading from v1.3.8 keeps its behavior and configuration. Consumers upgrading
  from v1.3.6 or earlier also inherit the v1.3.7 HMAC v3 default and idempotency
  header binding. HMAC challenge creation requires a v3-capable Herald server;
  upgrade the server before routing new SDK challenge-creation calls to it.
  Explicit v2 signing remains available for compatible non-creation routes;
  API-key authentication is unchanged.
- The release retains v1.3.8's atomic resend activation and its existing Redis
  formats and TTLs. No new storage migration is required. Preserve pending
  idempotency records and the per-instance audit spool during deployment.

## [1.3.3] - 2026-11-01

### Fixed

- Retain HMAC nonces for the full accepted timestamp window, including
  future-dated requests, so an accepted request cannot be replayed early.
- Require complete user, purpose, and channel binding on v2 verification.
- Persist a sending state before provider calls. Retries cannot automatically
  resend after an uncertain delivery or a failed completion write to Redis.
- Persist audit records locally before acceptance, retry backend delivery, and
  replay pending records after restart instead of dropping them on queue or
  backend failures.

### Compatibility

- v2 verification requires `expected_user_id`, `expected_purpose`, and
  `expected_channel`; v1 verification retains its existing request format.
- Uncertain sends return `409/delivery_unknown` for the same idempotency key
  until reconciled. Sending records do not expire automatically.
- Audit-enabled deployments require a writable persistent `AUDIT_SPOOL_DIR`,
  one directory per instance. Preserve pending records across upgrades. Audit
  delivery is at least once; consumers can deduplicate by `event_id`.
- Unavailable audit storage prevents startup; runtime local acceptance failures
  return `503/audit_unavailable` on affected operations.

### Changed

- Build, validate, and publish native binaries for Linux amd64 and arm64.
  Other environments should use the Linux container image.
- Upgrade `challenge-kit` from v1.10.2 to v1.10.3 and `provider-kit` from
  v1.7.2 to v1.7.3. Verification commits now compare the stored challenge
  snapshot and preserve its remaining TTL. HTTP providers retain their initial
  configuration and require a complete, size-bounded 2xx response for success.
  Public APIs and stored hash formats are unchanged; no data migration is needed.
- Upgrade `health-kit/v4` from v4.0.2 to v4.0.4 and `middleware-kit/v3`
  from v3.0.2 to v3.0.3; refresh the required Fiber/network
  dependency graph and module checksums.

## [1.3.2] - 2026-10-22

### Changed

- Require Go 1.27.3 and align build images and CI toolchains.
- Update compatible Go dependencies and published Kit versions; refresh module checksums.
- Preserve the project toolchain after quality report generation.
- Align the TOTP API module with the released herald-totp v1.2.5 service.


## [1.3.0] - 2026-09-21

### Changed

- Move every kit dependency to its current major version and update
  `herald-totp` to v1.2.3. The kits that split framework- and backend-specific
  code into subpackages are now imported that way, so Herald links only the
  adapters it uses:
  `audit-kit` v1.9.0 -> v2.1.0, `health-kit` v2.3.0 -> v4.0.0,
  `http-kit` v1.5.0 -> v2.0.0, `i18n-kit` v2.2.0 -> v4.0.1,
  `logger-kit` v2.3.0 -> v3.0.0, `metrics-kit` v2.2.0 -> v3.0.0,
  `middleware-kit` v2.2.0 -> v3.0.0, `secure-kit` v1.6.0 -> v2.1.0,
  `tracing-kit` v1.5.0 -> v2.0.0, `version-kit` v2.2.0 -> v4.0.0,
  `challenge-kit` v1.8.0 -> v1.10.0, and `redis-kit` v1.6.0 -> v1.7.0.
- Build the Redis audit store explicitly and hand it to `StorageOptions`,
  replacing the `RedisClient`/`RedisPrefix`/`RedisTTL` fields removed in
  `audit-kit` v2. The store keeps the `otp:audit:` key prefix and the
  `AUDIT_TTL` retention window, and it no longer closes the Redis client that
  routing, rate limiting and caching share when the audit writer stops.
- Register the `mysql` and `postgres` database drivers in `main.go`.
  `audit-kit` v2 no longer blank-imports them from its root package, so
  **deployments using `AUDIT_STORAGE_TYPE=database` need no change: Herald
  registers the drivers itself.** A test pins both registrations so they cannot
  be dropped unnoticed.
- Configure the outbound trace propagator once on the SDK's HTTP client,
  replacing the per-request `InjectTraceContext` calls removed in `http-kit`
  v2. Outbound trace headers are unchanged.
- Point the release `-X` build flags at `version-kit/v4` so built binaries keep
  reporting their real version instead of `dev`.

Herald's external behaviour is unchanged: routes, authentication outcomes, the
`/healthz` and `/metrics` responses, log fields, metric names, and the format
and Redis key prefix of audit records all stay as they were.

### Fixed

- Audit storage configured with a `postgres://` URL now reaches the database.
  `audit-kit` v1 compared the URL scheme against a mismatched literal and
  rejected every PostgreSQL URL as "unsupported database URL format"; v2 fixes
  the comparison.

## [1.2.1] - 2026-09-14

### Changed

- Update `herald-totp` to v1.2.2.
- Refresh the kit modules used by the service: `audit-kit` v1.9.0,
  `challenge-kit` v1.8.0, `health-kit` v2.3.0, and `logger-kit` v2.3.0.
- Raise the indirect terminal dependencies pulled in by `pterm`
  (`go-runewidth` v0.0.30, `terminfo` v1.2.0).
- Align the container baseline with Stargate.

## [1.2.0] - 2026-08-31

### Security

- Reject every supported no-auth alias in production and apply the normalized
  provider failure policy consistently during validation and delivery.
- Parse trusted forwarded-IP chains from the trusted proxy boundary, rejecting
  malformed or all-proxy chains instead of accepting caller-controlled prefixes.
- Return a stable JSON `413 payload_too_large` response for declared and streamed
  oversized request bodies, close rejected streaming connections, and enforce the
  same limit on the dedicated test listener.
- Apply available Alpine runtime security updates, including the fix for
  CVE-2026-14456, while keeping the container non-root.

### Fixed

- Give the test-only listener a discoverable loopback default
  (`127.0.0.1:18082`) instead of an ephemeral port.
- Generate correct SemVer Docker aliases for manually dispatched releases and
  update `latest` only for stable versions.
- Correct the production HMAC examples so generated secrets are provisioned
  once, shared by all replicas, and the canonical key-ID field remains empty
  when `X-Key-Id` is omitted.

### Changed

- Remove the arbitrary ten-digit OTP ceiling. Codes must be at least four digits
  and fit within `HERALD_MAX_BODY_BYTES` together with the verification envelope.
- Update `herald-totp` to v1.2.0 and refresh Prometheus and compression
  dependencies.

## [1.1.0] - 2026-08-26

### Changed

- Update the project and container build to Go 1.27 and refresh the lint tooling.
- Update `govulncheck` to v1.7.0 for Go 1.27 compatibility.

### Fixed

- Remove data races from the affected tests.

## [1.0.0] - 2026-08-26

### Security (breaking)

- **Production safety baseline.** A typed `ENVIRONMENT` (development|test|production)
  enum plus a fail-closed `config.Validate()` now refuse to start in production
  when: no request auth is configured; `HERALD_TEST_MODE=true`; TLS is
  half-configured (cert without key or vice versa); a client CA is set without a
  server cert/key; `PROVIDER_FAILURE_POLICY=soft`; a provider URL is plaintext or
  has no timeout; or Redis is passwordless on a non-loopback address (unless the
  explicit risk-ack flag is set).
- **Test-mode isolation.** `debug_code` is off by default and only exposed under
  `ENVIRONMENT=test` + `HERALD_TEST_MODE=true`. The `/v1/test/code` endpoint is
  now behind dedicated test auth (`HERALD_TEST_API_KEY`) and is not mounted in
  development or production.
- **HTTP hardening.** Request body cap (default 64 KiB), Read/Write/Idle
  timeouts, strict JSON content-type with rejection of unknown/trailing fields on
  core routes, and CORS disabled by default (wildcard origins refused in
  production).
- **Atomic single-active challenge.** Challenge creation uses a two-phase
  activate (pending → swap-active) so an older, still-live code cannot be
  redeemed after a new one is issued; activation failure fails closed.
- **Atomic idempotency store.** `Idempotency-Key` requests are namespaced per
  principal and claimed atomically (`SET NX`); concurrent duplicates result in
  exactly one provider send, conflicting bodies return `409`, and replays return
  the original response. Backend unavailability returns `503`.
- **Privacy-preserving rate limits.** User/IP/destination rate-limit and
  idempotency keys are peppered HMAC digests; raw PII no longer lands in Redis
  keys. Destinations are canonicalized and validated (email/SMS/DingTalk) to
  close dedup/limit-bypass gaps.
- **Reliable provider delivery.** `provider-kit` HTTP transport now sets an
  explicit timeout, bounds response bytes (`io.LimitReader`), validates URLs via
  `url.Parse`/`ResolveReference`, denies cross-host/scheme redirects, never
  forwards credentials across redirects, and enforces HTTPS in production. A
  configured-but-unconstructable provider is a startup error, not a silent
  disable.
- **HMAC v2 (replay-resistant).** New canonical signature binds
  method/path/query/timestamp/nonce/service/key-id/sha256(body) with a 60s
  default drift window. Nonces are single-use via Redis `SET NX EX` (keyed
  digest, verify-before-consume). `X-Nonce`/`X-Service` and an explicit key id
  are required; there is no random default key and no HMAC→API-key downgrade.
  Legacy v1 is available only behind `HMAC_V1_ENABLED`. Auth mode and client-cert
  mode are split (`REQUEST_AUTH_MODE` vs `CLIENT_CERT_MODE`).
- **Client IP trust split.** Rate limiting uses the trusted connection peer IP;
  a body-reported `client_ip` is advisory only and can no longer be used to evade
  IP limits.
- **Audit privacy.** In production `AUDIT_MASK_DESTINATION` defaults to true;
  dropped/failed audit records increment a metric and are logged instead of being
  lost silently; a broken audit backend is a hard startup error in production.

### Added

- Server lifecycle package unifying TLS/plaintext startup and ordered graceful
  drain/shutdown; startup errors are returned to `main` instead of `log.Fatal`
  deep in the stack.
- `/livez` (process liveness) and `/readyz` (Redis-backed readiness) endpoints,
  and a `-healthcheck` binary flag used by the container healthcheck.
- Low-cardinality reliability/security metrics: idempotency outcomes, provider
  timeouts, verification contention, replayed nonces, challenge state
  transitions, and dropped audit records.
- SDK: HMAC v2 signing by default with injectable clock/nonce, explicit auth
  mode, and `WithIdempotencyKey` helper.
- Bounded, strict template rendering (`missingkey=error`, size cap) and locale
  alias normalization.

### Changed

- Tracing spans use templated route names and omit full URL/query and user-agent
  to avoid high cardinality and PII.
- Containers: hardened `docker/Dockerfile` (pinned base, non-root fixed
  UID/GID 10001, UPX off by default, no gcc/musl-dev/curl), added `.dockerignore`,
  and a hardened `docker-compose.yml` (required explicit secrets, Redis not published,
  healthchecks + `depends_on: service_healthy`).
- CI: pinned tool versions and third-party actions to commit SHAs, least-
  privilege `GITHUB_TOKEN`, go-reportcard no longer auto-commits, and added
  compose smoke / non-root / config-startup-failure checks plus SBOM and
  container vulnerability scanning. Release verifies checksums and attempts
  optional keyless image signing with cosign; signing failures do not currently block publishing.

### Removed

- Unused `sessionManager` and the `session-kit` dependency.
- Dead `AUDIT_LOKI_URL` config (Loki is not a supported audit backend;
  `AUDIT_STORAGE_TYPE=loki` is now rejected).

### Migration

See `docs/enUS/MIGRATION_V2.md` and `docs/zhCN/MIGRATION_V2.md` for the v1→v2
authentication diff, environment-variable migration, and rollback guidance.

[Unreleased]: https://github.com/soulteary/herald/compare/v1.2.0...HEAD
[1.2.0]: https://github.com/soulteary/herald/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/soulteary/herald/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/soulteary/herald/releases/tag/v1.0.0
