# Read build identity without starting the service

Builds containing this change accept:

```sh
./herald -version
```

The command writes one JSON object and a newline to stdout, with three string
fields: `version`, `commit`, and `build_date`. The values are the actual
`version-kit/v4` variables supplied by the build's linker flags. Environment
variables do not override them. A development build can report `dev`/`unknown`;
the command does not invent a release identity when build metadata is absent.

`-version=true` has the same behavior. Combining version output with an enabled
`-healthcheck` or with positional arguments is refused with exit 2. Output-write
failure exits 1. Neither failure starts the service. Successful version output
exits 0 before the startup banner, explicit configuration validation, Redis
client construction, router or provider initialization. Normal startup and the
existing healthcheck behavior are unchanged when version output is not enabled.

Go imports and package-global initialization happen before `main`, including
configuration globals that read environment variables. This flag does not claim
that the process performs no environment reads, or replace a dependency-wide
initialization audit. For artifact verification, use a controlled environment
without deployment secrets/configuration, and separately verify artifact hashes,
signatures, source provenance and the runtime isolation you require. Version
JSON alone is not proof of authentication, IM delivery or deployment readiness.

This is additive functionality for a later reviewed build. Published 1.5.1 assets
do not acquire it retroactively and must not be rebuilt or overwritten to add it.
