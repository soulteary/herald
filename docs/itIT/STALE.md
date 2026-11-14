# Translation status: itIT

The documents in this directory and `README.itIT.md` are historical
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
