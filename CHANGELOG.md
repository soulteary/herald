# Changelog

All notable changes to Herald are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add `LOCKOUT_CHALLENGE_THRESHOLD` (integer, default `3`, minimum `1`): the
  number of challenges a user must exhaust within `LOCKOUT_DURATION` before
  the user is locked. `1` keeps the behaviour of v1.6.0.
- Add `IDEMPOTENCY_UNRESOLVED_TTL` (default `24h`). It must be positive and
  greater than `IDEMPOTENCY_KEY_TTL` (`CHALLENGE_EXPIRY` when that is `0`), or
  Herald refuses to start in every environment. A deployment whose pending TTL
  is 24h or longer must set it explicitly.
- Add the counter `herald_audit_post_action_failure_total{operation}`
  (`create`, `verify`, `revoke`) for audit result records that could not be
  stored after the action had completed.
- `429/rate_limit_exceeded` carries `limit` (`user`, `ip` or `destination`),
  naming the limit that refused the request.
- Startup warnings for the per-IP limit without trusted-proxy configuration, a
  production listener without TLS, and `CODE_LENGTH` below 6.
- The delivery-failure log line for the built-in SMTP provider carries
  `failure_category` and, when the server answered with one, `smtp_code`. Both
  are derived from error types; provider text is not logged.
- Startup warnings for `HERALD_TEST_MODE=true` outside the test environment,
  `HERALD_TOTP_ENABLED=true` without `HERALD_TOTP_BASE_URL`, and
  `OTLP_ENABLED=true` without `OTLP_ENDPOINT`.

### Changed

- Update `challenge-kit` to v1.12.0, which provides the lockout threshold, the
  lock key identity and a lock check that returns the Redis error. Challenge
  creation uses that check to answer `503/backend_unavailable` during an
  outage, in place of the extra `PING` it made before.
- Challenge creation checks the user lock first, then applies the user, IP and
  destination limits and the resend cooldowns in one Redis Lua call. Counter
  key names, fixed-window behaviour and TTLs match the previous limiter, so
  existing counters and cooldowns stay valid and mixed versions share them. A
  locked user who is also over a limit now gets `403/user_locked` instead of
  `429`. Redis metrics for this step use `operation="ratelimit_create"`
  instead of `ratelimit_user`, `ratelimit_ip`, `ratelimit_dest` and `cooldown`.
- A request rejected because every hash slot is busy answers
  `503/hash_capacity_exceeded`.
- The Compose file accepts optional `SMTP_*` variables for the built-in email
  provider.
- Request spans are named `<METHOD> <route template>` instead of `/`, and the
  raw request path is no longer exported as `http.target`. Dashboards that
  select spans by the old name need the new one.

### Fixed

- Expire idempotency records left in the `sending` state. They had no TTL, so a
  request interrupted between the send boundary and its result left a permanent
  Redis key that answered `409/delivery_unknown` forever. They now expire after
  `IDEMPOTENCY_UNRESOLVED_TTL`; until then behaviour is unchanged and no second
  send happens. After expiry the same key starts a new request. Records already
  stored without a TTL by earlier versions are not modified.
- Stop returning `503/audit_unavailable` when only the audit result record
  fails after the action can no longer be undone: the provider accepted the
  message, a v1 or v2 verification consumed the code, or a revoke completed.
  The request now returns its normal response, the idempotency record is
  completed instead of staying unresolved, the new counter is incremented and
  one error line records the event without the code or destination. In soft
  provider-failure mode the same applies to the `delivery_status=failed`
  response. Audit records required before the action still refuse the request
  with 503.
- An audit spool file that is read and cannot be decoded 5 times in a row is
  renamed to `<id>.json.quarantined` instead of being retried every second
  without end; `herald_audit_quarantined_total` counts such files. A file that
  cannot be read stays in the spool and is retried with a delay of up to 5
  minutes.
- Under the strict policy, answer `503/channel_not_configured` for a channel
  without a provider before any quota or cooldown is used, and log provider
  coverage at startup.
- Production uses `PROVIDER_FAILURE_POLICY=strict` when the variable is unset;
  an explicit `soft` is still rejected.
- Built-in messages state a lifetime under one minute as 1 minute.
- Custom templates in region-tagged directories are selected; template files
  that cannot be loaded are logged at startup.
- Join the caller's trace when a request carries a W3C `traceparent` header,
  whatever the header's case.
- `OTLP_ENDPOINT` accepts `http://` and `https://` URLs as documented, in
  addition to `host:port`. The URL form exported nothing. An unusable value is
  reported at startup, and `OpenTelemetry tracing initialized` is logged only
  when an exporter was set up.
- Custom email templates saved with CRLF line endings are sent. A subject line
  that still contains a carriage return or NUL is reported at startup and the
  template is not loaded.

### Security

- Lock a user only after `LOCKOUT_CHALLENGE_THRESHOLD` exhausted challenges
  within `LOCKOUT_DURATION`, not after the first one. `MAX_ATTEMPTS` wrong
  codes on a single challenge used to lock the user for every challenge,
  purpose and channel, so one challenge per `LOCKOUT_DURATION` kept a user
  locked out. An exhausted challenge still answers `locked`; the user's other
  challenges keep working until the threshold is reached. Guesses per user
  remain bounded by `RATE_LIMIT_PER_USER` multiplied by `MAX_ATTEMPTS` per
  hour, as before.
- Keep the raw `user_id` out of the user lock key. Locks are stored under
  `otp:lock:v2:<digest>`, with the digest keyed by `HERALD_PII_PEPPER`, and
  exhausted challenges under `otp:lock:exhausted:<digest>`.
  `otp:lock:<user_id>` keys from earlier versions are no longer read and
  expire within `LOCKOUT_DURATION`. A lock in force during the upgrade is not
  carried over, and old and new instances do not share locks during a rolling
  upgrade.
- Stop using the fixed secret `herald-idempotency-fallback` when none of
  `HERALD_IDEMPOTENCY_SECRET`, `HMAC_SECRET` and `API_KEY` is set, which is
  possible only outside production. The value is public in this repository,
  and it keyed idempotency records, HMAC nonce records and PII digests. With
  `HERALD_HMAC_KEYS` configured the secret is now derived from the key map, so
  instances that share the map still agree. With no secret material at all
  Herald generates a random secret at each start and logs a warning; such a
  deployment does not share those records between instances or keep them
  across restarts. Existing records keyed by the old value are no longer
  found. Deployments that set a secret, and production, are unchanged.
- Reject HMAC requests when the nonce cannot be recorded, in every
  environment. Outside production a Redis failure let the request through, so
  replays were accepted while Redis was down. Development and test now answer
  `503/nonce_store_unavailable`, as production does.
- Log a startup warning when `ENVIRONMENT` is unset or empty. It defaults to
  `development`, where most configuration problems are warnings and not
  startup errors. The default is unchanged in this release; a later release
  will refuse to start without the variable. Setting it to `development` or
  `test` silences the warning.
- Do not spend rate-limit quota on challenge requests that are rejected. A
  request denied by the resend cooldown, by `user_locked` or by a later limit
  previously still incremented the per-user, per-IP and per-destination
  counters, so a caller could exhaust another user's or destination's quota
  without any message being sent. The counters are now incremented together
  or not at all. A provider failure still keeps the quota spent.
- Apply `RESEND_COOLDOWN` per destination as well as per user and destination.
  Rotating `user_id` against one destination no longer bypasses the cooldown.
  The existing cooldown key is unchanged; the additional key
  `ratelimit:cooldown:dest:<digest>` is not read by v1.6.0 instances, so the
  destination-only cooldown is complete once every instance is upgraded.
  Callers that send to one destination for several user IDs within the
  cooldown now receive `429/resend_cooldown`.
- Refuse a `challenge_id` outside 1-64 bytes of `A-Za-z0-9_-` with
  `400/invalid_challenge_id`, before it reaches Redis, the audit log or spans.
- Lock a user after `LOCKOUT_FAILED_ATTEMPT_THRESHOLD` (default `15`) wrong
  codes within `LOCKOUT_FAILED_ATTEMPT_WINDOW` (default `1h`) across all
  challenges; `0` disables the counter. Wrong codes spread over many challenges
  did not lead to a lock before.
- Answer an unknown HMAC key ID with `401/invalid_signature`, the same as a
  wrong signature, on the v2/v3 schemes and on the legacy v1 scheme
  (`HMAC_V1_ENABLED=true`). The reason `invalid_key_id` is no longer returned.
- Stop exporting the client address and a partially masked destination on
  spans; the creation span carries `destination_digest`.

## [1.6.0] - 2026-11-13

### Added

- Add `HERALD_VERIFY_V1_ENABLED` (default `true`). Set it to `false` to stop
  registering `POST /v1/otp/verifications`, which has no expected
  user/purpose/channel binding, once all callers use the v2 route. The default
  keeps the v1 route available.
- Add `REDIS_USERNAME` (Redis ACL user) and `REDIS_TLS_ENABLED`, with optional
  `REDIS_TLS_CA_FILE` and `REDIS_TLS_SERVER_NAME`, for the Redis connection.
  All default to off or empty, so existing deployments connect as before. The
  server certificate is always verified when TLS is enabled.
- Add the SDK option `ClientIPHeader` (`WithClientIPHeader`). When set, challenge
  and verification requests carry their `ClientIP` in that header, so a gateway
  listed in `HERALD_TRUSTED_PROXIES` can have `RATE_LIMIT_PER_IP` applied per end
  user. The default sends no header and server behaviour is unchanged.

### Fixed

- Refuse to start when `RESEND_COOLDOWN` is `0`. The value passed validation,
  but the limiter rejects a zero cooldown, so every challenge request answered
  503 `backend_unavailable` after spending its rate-limit quota.
- Correct the startup warning for missing service credentials. It said
  authentication "will be disabled", but `hmac_v2` and `api_key` reject every
  request with 401 until a credential is set. The warning now names the
  configured `REQUEST_AUTH_MODE` and its effect. Request handling is unchanged.
- Document that `RATE_LIMIT_PER_IP` counts the caller's address, not the
  `client_ip` body field, and how to forward the end-user address.

### Security

- Stop exporting raw user IDs on trace spans. The create and verify spans now
  carry `user_id_digest` (peppered digest) instead of `user_id`. Audit records
  are unchanged. Trace queries on the `user_id` attribute need updating.
- Refuse to start in production when `REDIS_ADDR` is set to an invalid address
  or `HERALD_HMAC_KEYS` is set but malformed. Herald previously continued with
  `localhost:6379` or with `HMAC_SECRET` after a warning. Other environments
  keep the fallback, and unset variables behave as before.
- Key the per-destination rate limit and resend cooldown on a canonical SMS
  number. `+8613800138000`, `8613800138000` and `008613800138000` previously
  had separate quotas; they now share one. Delivery and stored challenges are
  unchanged, and national numbers are not matched to their international form.
  Counters and cooldowns recorded under the old key for a number written with
  `+` or `00` are not carried over: such a number starts with a fresh quota
  after the upgrade, and old and new instances count it separately while both
  are running.
- Update Go to 1.27.4, which fixes GO-2026-6859 in `net/http`. The build image is
  now `golang:1.27.4-alpine3.24`.
- SDK: `TOTPRevoke` returns an error with reason `revoke_not_confirmed` when
  Herald answers 200 without `ok: true`. It previously returned no error.
- SDK: an undecodable TOTP response is no longer quoted in the error message.
  Enrollment responses carry the shared secret.
- Report boolean environment variables whose value `strconv.ParseBool` does not
  accept (for example `AUDIT_MASK_DESTINATION=yes`). They previously fell back to
  the default without notice. This is fatal in production and a warning elsewhere.
  Accepted spellings are `1`, `t`, `T`, `TRUE`, `true`, `True`, `0`, `f`, `F`,
  `FALSE`, `false` and `False`, with no surrounding whitespace.
- Report integer and duration environment variables that cannot be parsed, such
  as `HMAC_MAX_DRIFT=60` (no unit) or `RATE_LIMIT_PER_IP=5/m`. They also fell
  back to the default without notice. This is fatal in production and a warning
  elsewhere.

### Changed

- Update Fiber to v3.7.0 and fasthttp to v1.78.0. Request trailers are no longer
  readable as headers, so authentication and idempotency headers must be sent in
  the header section. A request that announces more than 16 trailer names is
  answered with a plain-text 431 before Herald's JSON error handling. A chunked
  body that ends before its last chunk is answered with 400 `request_error`; it
  was previously processed as a shorter body.
- Update `middleware-kit/v3` to v3.0.6, `audit-kit/v2` to v2.1.6 (audit log
  files are created with mode 0600 since v2.1.5), `provider-kit` to v1.7.5 (an
  SMTP QUIT failure after the message was accepted is no longer a failed send),
  `challenge-kit` to v1.11.2, `redis-kit` to v1.8.2, `cli-kit` to v1.9.4,
  `health-kit/v4` to v4.0.7, `http-kit/v2` to v2.1.1 and the other kits to
  their current releases, and refresh indirect dependencies.
- Update `herald-totp/v2` to v2.0.4, a dependency-only release of the TOTP
  client module. The TOTP proxy protocol and SDK methods are unchanged.
- The module requires Go 1.27.4. SDK consumers on an older toolchain need to
  update it.

### Compatibility and operations

- Existing SDK methods and signatures, HTTP success payloads, authentication
  modes, Redis records and credential formats are unchanged. The new settings
  and the SDK option `WithClientIPHeader` are opt-in; their defaults keep the
  v1.5.2 behaviour. No data conversion, key migration or re-enrollment is
  required.
- Check the configuration before upgrading. `RESEND_COOLDOWN=0` now fails
  startup in every environment. In production, an invalid `REDIS_ADDR`, a
  malformed `HERALD_HMAC_KEYS`, and boolean, integer or duration variables that
  cannot be parsed also fail startup instead of falling back to a default.
- SDK consumers need Go 1.27.4 or later. `TOTPRevoke` can now return
  `revoke_not_confirmed` where it previously returned no error. Upgrading the
  SDK does not replace a deployed service, and the SDK continues to work with
  an older service.
- Rollback to v1.5.2 retains Redis data and credentials. Remove the Redis TLS
  and ACL username settings first if the Redis server requires them, because
  v1.5.2 ignores them. Source consumers must remove `WithClientIPHeader` calls
  before downgrading the module. See the
  [upgrade and rollback guidance](docs/enUS/DEPLOYMENT.md#upgrade-from-v152-to-v160).

## [1.5.2] - 2026-11-09

### Added

- Add `-version`, which writes the build's `version`, `commit` and `build_date`
  as one JSON object and exits before configuration is validated or any
  dependency is contacted. See [VERSION_CLI](docs/enUS/VERSION_CLI.md).

## [1.5.1] - 2026-11-09

### Fixed

- Preserve DingTalk recipient IDs as given, apart from surrounding whitespace.
  They were lowercased before delivery, fingerprinting and rate limiting,
  although corporate user IDs are opaque and can be case-sensitive.

### Changed

- The release workflow binds each release to its reviewed revision and refuses
  to overwrite a published version.

## [1.5.0] - 2026-11-08

### Added

- Include optional retry timing on challenge-creation quota and cooldown errors.
  Add `CreateChallengeWithMetadata` to read that timing without changing the
  existing SDK method, error type, quota policy or request retry behavior.


### Compatibility and operations

- Existing `CreateChallenge` callers and the public error type remain compatible.
  Retry timing is optional, bounded and only accepted from usable HTTP 429
  metadata. Missing, malformed or conflicting timing remains unknown.
- Timing indicates the earliest retry opportunity, not guaranteed admission.
  No automatic retry, quota reset or verification-consumption change is added.
  Authentication, dependencies, Redis namespaces and credential formats are unchanged.
- Upgrade the Herald service to receive the new optional timing fields and use
  SDK v1.5.0 to consume them. The SDK also works with an older service, with timing
  absent. SDK upgrades do not replace a deployed service.
- Rollback to v1.4.4 retains Redis and credentials but removes the new server timing
  and SDK method. Source consumers using the new method must revert those calls
  before downgrading the module. Preserve pending, idempotency and replay state.

## [1.4.4] - 2026-11-08

### Dependencies

- Update secure-kit/v2 and audit-kit/v2 to v2.1.4, challenge-kit to v1.11.1,
  and herald-totp/v2 to v2.0.3. Unicode masking preserves complete code points;
  ASCII output and cryptographic operations remain unchanged.

### Documentation

- Correct TOTP status fields and verification-proxy error handling. Clarify
  request authentication versus optional mTLS, protected Prometheus examples,
  and non-production soft-delivery behavior.

### Compatibility and operations

- Keep SDK methods, HTTP success payloads, HMAC/key-ID rules, Redis records and
  namespaces, quota policy, activation and credential-snapshot guards unchanged.
  No data conversion, key migration or re-enrollment is required.
- Preserve Redis, pending/idempotency and replay state, audit buffering and
  the original TOTP encryption key. Deploy the TOTP service separately; updating
  its SDK module does not replace a running backend. An existing v1.4.3 binary
  or image restores its compiled dependency behavior without rolling back TOTP.
  Go source consumers must restore pre-upgrade manifests and inspect selected
  module versions; a parent-module downgrade alone may retain newer dependencies.

## [1.4.3] - 2026-11-07

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

[Unreleased]: https://github.com/soulteary/herald/compare/v1.6.0...HEAD
[1.6.0]: https://github.com/soulteary/herald/compare/v1.5.2...v1.6.0
[1.5.2]: https://github.com/soulteary/herald/compare/v1.5.1...v1.5.2
[1.5.1]: https://github.com/soulteary/herald/compare/v1.5.0...v1.5.1
[1.5.0]: https://github.com/soulteary/herald/compare/v1.4.4...v1.5.0
[1.4.4]: https://github.com/soulteary/herald/compare/v1.4.3...v1.4.4
[1.4.3]: https://github.com/soulteary/herald/compare/v1.4.2...v1.4.3
[1.4.2]: https://github.com/soulteary/herald/compare/v1.4.1...v1.4.2
[1.4.1]: https://github.com/soulteary/herald/compare/v1.4.0...v1.4.1
[1.4.0]: https://github.com/soulteary/herald/compare/v1.3.13...v1.4.0
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
