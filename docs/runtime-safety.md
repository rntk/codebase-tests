# Runtime Safety

## Subprocess Timeouts

Every plugin call that starts an external tool must honor `context.Context` cancellation. `RunOptions.Timeout` is the default if no context deadline is set.

## Output Limits

`RunOptions.MaxOutputBytes` defaults to 1 MB. Stderr/stdout beyond this limit is truncated.

## Environment Handling

Only variables in `EnvAllowlist` are forwarded to subprocesses. This prevents leaking sensitive env vars.

## Dependency Readiness Checks

Before starting a plugin, verify required binaries exist in `PATH`:

- Go plugin: `go`, `gopls`
- Python plugin: selected LSP (`pylsp` or `pyright-langserver`), `pytest`, `coverage.py`
- Git: required for diff features

If a dependency is missing, return a clear error during `Initialize`.
