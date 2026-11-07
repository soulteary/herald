# Changelog

All notable changes to Herald are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Allow the TOTP proxy to select a mapped HMAC key and service identity with
  `HERALD_TOTP_HMAC_KEY_ID` and `HERALD_TOTP_HMAC_SERVICE`. Unset values retain
  the SDK defaults. This supports explicit key selection during overlap; it
  does not change the authentication protocol or upgrade the TOTP backend.
  Reconcile single-key selection before rolling back to an older proxy.

### Security hardening

- Omit the configured API key from SDK requests when an HMAC secret is set.
  HMAC authentication failures do not retry with the API key. Clients using
  `REQUEST_AUTH_MODE=api_key` must leave the SDK HMAC secret empty. HMAC signature
  formats and API-key-only requests are unchanged.

- Require an explicit signed `X-Key-Id` for HMAC v2/v3 requests when
  `HERALD_HMAC_KEYS` is configured, including singleton maps. An omitted ID
  previously let the same signed request enter authentication again after a
  same-secret default-ID change. Set client IDs before upgrading verifiers.
  Single-`HMAC_SECRET` defaults and existing Redis record formats are unchanged.

## [1.4.2] - 2026-11-07

### Dependencies

- Update `herald-totp/v2` to v2.0.2. Its service binds TOTP validation commits
  to the credential snapshot used for validation. Deploy that service separately;
  the SDK dependency update does not replace a running backend.

### Compatibility and operations

- Keep public SDK signatures, HTTP request and success payloads unchanged.
  No Redis schema, namespace or encryption-key conversion is required between
  these patch versions. Preserve pending enrollments, counters, idempotency,
  replay and audit state during upgrade and rollback.
- Replace all TOTP instances to obtain the snapshot guard. Mixed older instances
  retain the earlier commit behavior; rolling the backend back to v2.0.1 loses
  the guard. Herald rollback alone does not change the deployed backend.

## [1.4.1] - 2026-11-07

### Fixed

- Preserve a complete upstream enrollment-confirmation rate-limit response as
  `429/rate_limited` instead of masking it as a proxy failure. Return fixed error
  fields without forwarding upstream bodies or headers; truncated or oversized
  responses still fail as `502/proxy_failed`.


### Dependencies

- Replace the temporary herald-totp/v2 confirmation-quota pseudo-version with
  the released v2.0.1 module. Deploy the matching TOTP service separately; an SDK
  dependency update does not change an independently deployed backend.

### Compatibility and operations

- Keep Herald's public Go API, external HTTP routes and successful enrollment
  payload unchanged. The confirmation quota response is now observable as 429
  rather than a masked 502; callers must retain enrollment context for retry.
- Reserve shared TOTP subject quota for enrollment start and confirmation plus
  failed attempts. A subject limit of one blocks confirmation after start until
  the hourly reset; a pending enrollment may expire before then.
- No Redis schema or encryption-key conversion is required for these patch
  versions. Preserve Redis data, idempotency and nonce records, audit state and
  the original TOTP encryption key during upgrade and rollback. Older major
  versions still require their existing coordinated migration rules.
- Rolling Herald back to v1.4.0 masks upstream confirmation 429 as 502 again.
  Rolling TOTP back to v2.0.0 removes its confirmation quota. Do not clear
  persistent state or independently downgrade providers to work around a 429.

## [1.4.0] - 2026-11-07

### Security changes

- Add optional HMAC service/action policies binding each key ID to a service
  principal and explicit OTP/TOTP actions. Policy mode rejects shared HMAC
  secrets and legacy v1 signatures; it does not isolate tenants or subjects.
- Separate metrics credentials from API and HMAC credentials. Production metrics
  require a dedicated random key; provision the collector before upgrading.
- Require explicit activation before OTP verification. Pending records remain
  unverifiable if delivery, activation, or cleanup fails. Activation and final
  verification compare stored snapshots, preserving revocation and replacement.
- Write the activation index before marking a record active, preventing new
  redeemable orphan records after a partial Redis script failure. Idempotent
  activation retries never identify the new record as a previous ID to revoke.
- Use operation-bound TOTP HMAC v2, covering method, escaped path, raw query,
  key ID, timestamp, service, nonce, and body digest. Nonce protection unifies
  aliases of the same secret. Invalid HMAC cannot fall back to API-key auth.
- Reject oversized TOTP service/key identifiers locally before sending. HMAC
  requests omit the static API key; API-key-only operation remains available.
- Reject redirects in the public Herald SDK and TOTP proxy to prevent credential
  forwarding. Keep bounded TOTP response decoding and production HTTPS checks.

### Dependencies

- Use released `challenge-kit` v1.11.0 and `herald-totp/v2` v2.0.0. The TOTP
  module-path migration is internal; Herald's public Go module and HTTP paths,
  request payloads, and response types remain unchanged.
- Retain the provider patch updates for herald-smtp and herald-dingtalk v1.2.6.

### Upgrade requirements

- This release requires operational migration; it is not a drop-in upgrade.
  Pause OTP creation, drain requests, wait the previous maximum challenge
  lifetime, and replace every Herald verifier before resuming. Strict record,
  verification-lock, and active-index namespaces are disjoint from historical
  keys. Existing user lockouts remain intact; never fall back to old records.
- Preserve unresolved idempotency records, their derivation secret, nonce state,
  audit volumes, and provider credentials. Reconcile `delivery_unknown` with the
  provider before resending. A lost activation reply may represent a committed
  active challenge; pending cleanup deliberately preserves it.
- Partial Redis errors can remove the previous active challenge or leave stale
  indexes, reducing availability. They do not roll back preceding writes. Let
  abandoned pending records and stale indexes expire with their original TTL.
  Older active orphan records are not repaired automatically: affected operators
  must wait the original TTL or explicitly revoke known IDs.
- Upgrade the TOTP service and this proxy together for HMAC v2; old HMAC signatures
  are rejected without a legacy fallback. Preserve TOTP encryption keys and Redis
  credentials. HTTP `/v1` routes remain, but their HMAC wire protocol changes.
- Configure the metrics collector with a separate credential and prepare service
  policies before enabling them. See the current security and migration guides
  for key rotation, strict namespaces, and rollback procedures.

## [1.3.13] - 2026-11-02

### Fixed

- Use the verified HMAC key identity for idempotency. Omitted and explicit
  default IDs no longer split send records; rotating the default cannot replay
  an ambiguous legacy credential's response. See PR #108.
- Reject unconfigured key IDs in single-secret HMAC mode, preventing arbitrary
  aliases from selecting independent nonce and idempotency namespaces. See PR #109.
- Reject standalone wildcard entries anywhere in production CORS origin lists,
  during both startup validation and router construction. See PR #110.
- Refresh current operational documentation, error references, Redis diagnostics,
  metric names, capacity assumptions and upgrade entry points.

### Compatibility and operations

- HMAC v2/v3 signing is unchanged; creation still requires v3. Single-secret mode
  accepts only an omitted key ID or `default`. Provision known custom IDs in an
  explicit key map without changing caller identities or client keys.
- Historical empty-key-ID records remain intact and block creation with
  `409/idempotency_namespace_conflict`; no response is copied or replayed across
  ambiguous credentials. Existing authorized explicit-key records keep their
  keys, contents, owners and TTLs. Preserve non-expiring sending records.
- Pause creation, drain in-flight requests, reconcile old identities and replace
  all instances. Preserve Redis data, audit volumes, nonce prefixes and the
  effective idempotency secret. Older releases need an external identity
  allowlist before creation can resume safely after rollback.
- Remove standalone `*` entries from production CORS lists before upgrade or
  rollback. Default CORS-disabled behavior and development/test support remain
  unchanged; this does not bypass API Key/HMAC authentication.
- Provider diagnostics retain the v1.3.11 `reason` field and sanitized span errors;
  this release adds no provider diagnostic field change. Historical logs and
  exported spans are not rewritten. Review retained copies and access/retention
  policies, and rotate credentials confirmed to have been exposed.

### Validation

- Linux reproductions confirmed duplicate sends after failed Redis completion
  writes, credential-response replay after default rotation, and unlisted-origin
  CORS preflights receiving wildcard responses. Patched regressions passed.
- Full Linux race suite passed with 95.7% statement coverage. Real Redis 7.4.12
  namespace tests, vet, lint and Linux amd64/arm64 builds passed. Go Report Card:
  A+, 100%, zero issues. Current documentation links and examples were checked.

## [1.3.12] - 2026-11-01

### Fixed

- Bound OTP creation and v1/v2 verification with a shared, non-queueing hash
  work budget. Creation previously had no aggregate Argon2 concurrency cap;
  authenticated requests for distinct identities could exhaust memory when
  permitted by their configured rate limits.
- Release capacity before provider delivery and after completed, failed or
  panicking operations. Canceled work retains its slot until it actually ends.
  Attempt verification locks once rather than polling with shared capacity held.

### Configuration and compatibility

- `HERALD_MAX_CONCURRENT_HASH_OPERATIONS` is a positive integer, default `4`,
  local to each handler instance. Size it for the current roughly 64 MiB Argon2
  cost per operation plus garbage collection and process overhead; it is not
  a hard RSS limit or a budget shared across replicas.
- Overload returns `503/backend_unavailable` and `Retry-After: 1`. Rejected
  creation requests do not reserve rate/cooldown capacity; their pre-send
  idempotency claims are released when Redis is available. Completed responses
  remain replayable. Rejected verifications consume no code or attempt.
- Lock contention immediately returns the existing `409/lock_contention` with
  `Retry-After: 1`. Retry with the same idempotency key and fresh HMAC timestamp
  and nonce. Redis records, signature formats and Argon2 parameters are unchanged.

### Validation

- Linux regressions cover shared admission, safe retries/replays, unchanged
  attempts, slow-provider isolation, lock contention, cancellation and error/panic
  release. Full race suite passed with 95.7% statement coverage; vet, lint and
  Linux amd64/arm64 builds passed.

## [1.3.11] - 2026-11-01

### Fixed

- Stop exporting raw provider errors to delivery-failure logs and tracing.
  An upstream error echoing an OTP, destination or credential could previously
  persist those values in logs, span status and exception events. Retain only
  known reason codes; normalize unknown reasons to `send_failed`.
- Align operational documentation with current authentication, environment
  variables, metrics, provider interfaces, test listeners and Linux validation.
  Correct local links and expose the historical status of older translations.

### Compatibility

- Delivery-failure log consumers must use `reason` instead of free-text `error`.
  Span descriptions use `provider send failed: <reason>`. Client responses,
  strict-policy revocation, idempotency handling and Redis records are unchanged.
- Existing logs and exported spans are not rewritten. Review historical stores
  and exported copies for sensitive provider echoes, apply access/retention
  policies and rotate exposed credentials. No client or record migration is needed.

## [1.3.10] - 2026-11-01

### Fixed

- Preserve exact identity fields and boundaries in versioned activation indexes.
  Activating `alice` can no longer revoke `Alice` when other fields match.
  Migrate legacy indexes only when the stored challenge matches all four fields.
- Correct release download links to use published image tags and a versioned
  changelog URL.

### Compatibility

- Old challenge records retain their verification format and expiry. Overwritten
  legacy indexes cannot be reconstructed automatically. Pause creation, drain
  requests and replace all old instances before resuming; mixed versions can
  leave superseded codes valid. Before rollback, stop creation and wait the
  maximum challenge lifetime. Preserve Redis data and audit volumes.

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

## [1.3.8] - 2026-11-01

### Fixed

- Revoke the superseded OTP and publish its replacement active index in one
  Redis Lua operation. A failed standalone revocation or a Herald process exit
  can no longer leave both codes redeemable after a successful resend. Existing
  active-index encodings, challenge records, expiry settings and idempotency
  delivery-unknown protection are retained.

## [1.3.7] - 2026-11-01

### Fixed

- Bind the idempotency header (including its absence) in HMAC v3. HMAC challenge
  creation requires v3 so changing or stripping the header cannot redirect a
  signed request into another idempotency record and cause a duplicate send.
  Go SDK signing defaults to v3. Older HMAC creation callers must upgrade;
  v2 remains accepted on other routes. API key mode and Redis records are unchanged.

- Refactor complex paths and correct Go Report Card spelling scope.

## [1.3.6] - 2026-11-01

### Fixed

- Reject `X-Service` and `X-Key-Id` values containing the reserved `|kid:`
  delimiter before authentication, nonce consumption, or business writes.
  Otherwise different service/key pairs could share an idempotency namespace
  and replay a response across HMAC credentials. Rejected values return
  `401/invalid_auth_headers`; ordinary namespaces and existing Redis records
  retain their format, contents, and TTL.

- Reuse keyed digest state and simplify template formatting; raise the enforced
  statement coverage floor to 95%.

## [1.3.5] - 2026-11-01

### Fixed

- Use length-framed, versioned idempotency fingerprints for requests containing
  embedded NUL characters. Moving separator bytes between user and locale
  fields can no longer replay a response for a different request. Ordinary
  request fingerprints and Redis keys remain unchanged. Existing legacy NUL
  records return `409/idempotency_conflict` and retain their contents and TTL;
  non-expiring sending records must be reconciled rather than cleared to retry.

## [1.3.4] - 2026-11-01

### Fixed

- Harden all TOTP proxy calls with a redirect-denying HTTP client, verified
  TLS, bounded response reads, and strict HTTP 200 response handling. Failed
  or incomplete upstream responses cannot become successful verification
  results, and redirects cannot forward TOTP codes or service credentials.
- Apply `PROVIDER_MAX_RESPONSE_BYTES` to TOTP responses as well. Redirects,
  non-200 responses, oversized bodies, and malformed or interrupted responses
  now return `502/proxy_failed`. Valid `200/ok:false` verification results and
  complete enrollment-confirm HTTP 400 responses retain their existing contract.

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

[Unreleased]: https://github.com/soulteary/herald/compare/v1.3.13...HEAD
[1.3.13]: https://github.com/soulteary/herald/compare/v1.3.12...v1.3.13
[1.3.12]: https://github.com/soulteary/herald/compare/v1.3.11...v1.3.12
[1.3.11]: https://github.com/soulteary/herald/compare/v1.3.10...v1.3.11
[1.3.10]: https://github.com/soulteary/herald/compare/v1.3.9...v1.3.10
[1.3.9]: https://github.com/soulteary/herald/compare/v1.3.8...v1.3.9
[1.3.8]: https://github.com/soulteary/herald/compare/v1.3.7...v1.3.8
[1.3.7]: https://github.com/soulteary/herald/compare/v1.3.6...v1.3.7
[1.3.6]: https://github.com/soulteary/herald/compare/v1.3.5...v1.3.6
[1.3.5]: https://github.com/soulteary/herald/compare/v1.3.4...v1.3.5
[1.3.4]: https://github.com/soulteary/herald/compare/v1.3.3...v1.3.4
[1.3.3]: https://github.com/soulteary/herald/compare/v1.3.2...v1.3.3
[1.3.2]: https://github.com/soulteary/herald/compare/v1.3.1...v1.3.2
[1.3.0]: https://github.com/soulteary/herald/compare/v1.2.1...v1.3.0
[1.2.1]: https://github.com/soulteary/herald/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/soulteary/herald/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/soulteary/herald/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/soulteary/herald/compare/v0.11.0...v1.0.0
