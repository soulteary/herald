# ⚠️ These docs are stale (out of date)

The security-hardening release (Herald v2) updated only the **enUS** and
**zhCN** documentation. The docs in this directory were **not** updated and may
be inaccurate regarding:

- HMAC v1 → v2 authentication (canonical string, required headers, nonces)
- Idempotency / concurrency semantics
- Provider timeout / failure policy
- Test-mode limits
- Environment-variable migration and production deployment

Please refer to `docs/enUS/MIGRATION_V2.md` (or `docs/zhCN/MIGRATION_V2.md`)
until this language is retranslated.
