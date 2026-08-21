# Changelog

All notable changes to Herald are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
- **HTTP hardening.** Request body cap (default 32 KiB), Read/Write/Idle
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
  and a hardened `docker-compose.yml` (placeholder secrets, Redis not published,
  healthchecks + `depends_on: service_healthy`).
- CI: pinned tool versions and third-party actions to commit SHAs, least-
  privilege `GITHUB_TOKEN`, go-reportcard no longer auto-commits, and added
  compose smoke / non-root / config-startup-failure checks plus SBOM and
  container vulnerability scanning. Release verifies checksums and optionally
  keyless-signs images with cosign.

### Removed

- Unused `sessionManager` and the `session-kit` dependency.
- Dead `AUDIT_LOKI_URL` config (Loki is not a supported audit backend;
  `AUDIT_STORAGE_TYPE=loki` is now rejected).

### Migration

See `docs/enUS/MIGRATION_V2.md` and `docs/zhCN/MIGRATION_V2.md` for the v1→v2
authentication diff, environment-variable migration, and rollback guidance.
