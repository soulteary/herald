# Translation status: jaJP

The documents in this directory and `README.jaJP.md` are historical
translations. Their examples are not maintained as current deployment guidance.
Use [English](../enUS/README.md) or [中文](../zhCN/README.md).

Current requirements include HMAC v3 for challenge creation, replay-resistant
nonces, required v2 verification context, non-expiring uncertain-send records,
persistent audit buffering, exact-identity activation indexes, and redacted
provider diagnostics. Builds and validation run on Linux; other hosts should
use Linux containers. See [authentication](../enUS/API.md#authentication),
[deployment](../enUS/DEPLOYMENT.md) and [security](../enUS/SECURITY.md).

The v1.3.13 upgrade also requires verified HMAC key identities, explicit custom
key provisioning in single-secret migrations, reconciliation of ambiguous legacy
records, and rejection of standalone wildcards in production CORS lists. Use the
current English or Chinese upgrade procedures before deployment or rollback.

Changes merged after v1.6.0 are not reflected in these pages either, apart from
a few literal corrections (the `LOCKOUT_FAILED_ATTEMPT_*` settings, `https://`
example URLs for provider and TOTP base URLs, the `503` status of challenge
creation and the status codes of `POST /v1/otp/verifications`). Not translated: the `limit` field of
`429/rate_limit_exceeded`, the reasons `invalid_challenge_id`,
`channel_not_configured` and `hash_capacity_exceeded`, `401/invalid_signature`
for an unknown HMAC key ID (formerly `invalid_key_id`), the production default
`PROVIDER_FAILURE_POLICY=strict`, startup warnings, audit spool quarantine, the
accepted forms of `OTLP_ENDPOINT`, request span naming, message template
directories, the SMTP `failure_category` log field, and the quick start with
`SMTP_*` variables.

Releases v1.7.0, v1.8.0 and v1.9.0 are not covered by these pages. They added
no HTTP endpoint. These settings do not appear anywhere in this directory:
`LOCKOUT_CHALLENGE_THRESHOLD` and `IDEMPOTENCY_UNRESOLVED_TTL` (v1.7.0),
`HERALD_CODE_HASH_KEY` and `HERALD_CODE_HASH_VERIFY_ONLY` (v1.8.0), and
`HERALD_CODE_HASH_KEY_PREVIOUS` (v1.9.0). See their rows in the
[OTP / Challenge settings](../enUS/DEPLOYMENT.md#otp--challenge) and the sections
[Verification code hash key](../enUS/DEPLOYMENT.md#verification-code-hash-key),
[Changing the key](../enUS/DEPLOYMENT.md#changing-the-key) and
[User lockout](../enUS/SECURITY.md#user-lockout). The counters
`herald_audit_post_action_failure_total`, `herald_audit_quarantined_total`,
`herald_audit_spool_missing_total` and `herald_failed_attempt_lock_total`
([monitoring](../enUS/MONITORING.md)) are not listed either.

Two statements in these pages hold only for the defaults or for earlier
versions. Codes are stored as Argon2 hashes only while `HERALD_CODE_HASH_KEY`
is empty or `HERALD_CODE_HASH_VERIFY_ONLY=true`; otherwise new challenges
store a keyed HMAC-SHA-256 digest. The `LOCKOUT_DURATION` row describes a user
lock after the maximum attempts; since v1.7.0 a user is locked after
`LOCKOUT_CHALLENGE_THRESHOLD` (default `3`) exhausted challenges, not after
the first one.

Upgrade and rollback steps for these releases, in English:
[Upgrade from v1.6.0 to v1.7.0](../enUS/DEPLOYMENT.md#upgrade-from-v160-to-v170),
[Upgrade from v1.7.0 to v1.8.0](../enUS/DEPLOYMENT.md#upgrade-from-v170-to-v180)
and
[Upgrade from v1.8.0 to v1.9.0](../enUS/DEPLOYMENT.md#upgrade-from-v180-to-v190).
The Chinese guide has the same sections.
