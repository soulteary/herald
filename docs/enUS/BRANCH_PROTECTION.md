# Branch Protection & Repository Hardening

Recommended GitHub settings for the `main` (and `develop`) branches. These are
not enforceable from within the repo files themselves and must be configured in
repository/organization settings.

## Required settings for `main`

- Require a pull request before merging (no direct pushes).
- Require at least 1 (ideally 2) approving reviews; dismiss stale approvals on
  new commits.
- Require review from Code Owners (add a `CODEOWNERS` file).
- Require status checks to pass before merging, and require branches to be up to
  date. Required checks:
  - `Code Formatting Check`
  - `Code Static Analysis`
  - `Code Testing`
  - `Code Quality Check`
  - `Dependency Security Scan`
  - `Docker Build + Scan`
  - `Compose Smoke + Config Startup Failure`
- Require signed commits.
- Require linear history.
- Require conversation resolution before merging.
- Restrict who can push / force-push; **block force-pushes and deletions**.
- Do not allow bypassing the above for administrators.

## Actions & token hardening

- Set the default `GITHUB_TOKEN` permissions to **read-only** at the repo level;
  workflows opt into more via `permissions:` blocks (already done in CI).
- Restrict which Actions can run to "Allow select actions" and pin all
  third-party actions to full commit SHAs (already done in the workflows).
- Enable Dependabot for `gomod`, `github-actions`, and `docker`.

## Release integrity

- Tag releases as `vX.Y.Z`; the release workflow builds multi-arch binaries,
  generates and **verifies** `checksums.txt`, pushes an image, and optionally
  keyless-signs it with cosign (Sigstore/Fulcio via OIDC).
- Consumers should verify checksums and, when signing is enabled, verify the
  cosign signature by digest.
