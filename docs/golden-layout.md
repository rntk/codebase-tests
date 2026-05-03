# Golden File Layout

## Default Convention

```
tests/golden/<module>/<submodule>/<funcName>/<caseName>.in.json
tests/golden/<module>/<submodule>/<funcName>/<caseName>.out.json
tests/golden/<module>/<submodule>/<funcName>/<caseName>.meta.json  (optional)
```

- `<module>/<submodule>/<funcName>` maps from the test function's qualified name.
- Each segment uses the same escaping as `casePath` in `docs/ids.md`.

## Case Name Mapping

- Go table tests: `tt.Run(name, ...)` → `name` is the case name.
- Go subtests: `t.Run(name, ...)` → `name` is the case name.
- Pytest parametrize: `@pytest.mark.parametrize` argument tuple → joined with `_`.

## Duplicate Case Names

Append `_0`, `_1`, etc.

## Missing Sides

If only `.in.json` or `.out.json` exists, the case is still listed with `inExists`/`outExists` set accordingly.

## Meta File Schema

```json
{
  "version": 1,
  "labels": ["fast", "integration"],
  "slos": {
    "maxDurationMs": 500
  }
}
```

- `version` — reserved for future migration.
- `labels` — arbitrary tags.
- `slos` — reserved for v2 SLO enforcement. v1 reads and returns it but does not enforce.
