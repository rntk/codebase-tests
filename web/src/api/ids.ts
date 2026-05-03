// Utilities for parsing the language-prefixed IDs the backend emits,
// e.g. "go:rel/file.go:pkg.Func", "python:rel.py:func", "javascript:src/x.ts:func".

export function languageFromId(id: string): string {
  const idx = id.indexOf(':')
  if (idx <= 0) return 'unknown'
  return id.slice(0, idx)
}

export function languageLabel(language: string): string {
  switch (language) {
    case 'go':
      return 'Go'
    case 'python':
      return 'Python'
    case 'javascript':
      return 'JavaScript / TypeScript'
    default:
      return language.charAt(0).toUpperCase() + language.slice(1)
  }
}

export function languageIcon(language: string): string {
  switch (language) {
    case 'go':
      return '🐹'
    case 'python':
      return '🐍'
    case 'javascript':
      return '🟨'
    default:
      return '📦'
  }
}
