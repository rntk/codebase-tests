# Stable IDs

## Symbol ID

```
plugin:path:qualifiedName:startLine:startColumn
```

- `plugin` — plugin name (e.g., `go`, `python`).
- `path` — file path relative to project root.
- `qualifiedName` — package-qualified symbol name.
- `startLine`, `startColumn` — 1-based.

## TestFunc ID

```
plugin:path:qualifiedName
```

- `qualifiedName` for Go is `Package.FuncName`.

## TestCase ID

```
plugin:path:qualifiedName:casePath
```

- `casePath` is the escaped subtest / parametrized case path.

## Escaping Rules

- Replace `:` with `%3A`.
- Replace `/` with `%2F`.
- Replace `%` with `%25`.

These rules apply to every segment so IDs remain unambiguous.

## Duplicate Handling

If two cases would produce the same `casePath`, append a zero-based index suffix `_0`, `_1`, etc.
