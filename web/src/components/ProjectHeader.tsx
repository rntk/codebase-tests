import type { CoverageReport, FileCoverage, PluginSettings, Project } from '../api/types.ts'
import { languageIcon, languageLabel } from '../api/ids.ts'

function formatCoverage(percentage: number): string {
  if (!Number.isFinite(percentage)) return 'n/a'
  return `${percentage.toFixed(1).replace(/\.0$/, '')}%`
}

function pathMatchesLanguage(path: string, language: string): boolean {
  const lower = path.toLowerCase()
  switch (language) {
    case 'go':
      return lower.endsWith('.go')
    case 'python':
      return lower.endsWith('.py')
    case 'javascript':
      return ['.js', '.jsx', '.mjs', '.cjs'].some((ext) => lower.endsWith(ext))
    case 'typescript':
      return ['.ts', '.tsx', '.mts', '.cts'].some((ext) => lower.endsWith(ext))
    default:
      return false
  }
}

function coverageFromFiles(files: FileCoverage[], plugin: PluginSettings): number | undefined {
  const matchingFiles = files.filter(
    (file) => pathMatchesLanguage(file.path, plugin.language) || pathMatchesLanguage(file.path, plugin.name)
  )
  if (matchingFiles.length === 0) return undefined

  let totalLines = 0
  let hitLines = 0
  let percentageTotal = 0
  let percentageCount = 0

  for (const file of matchingFiles) {
    if (file.lines && file.lines.length > 0) {
      for (const line of file.lines) {
        const count = Math.max(line.end - line.start + 1, 0)
        totalLines += count
        if (line.hit) hitLines += count
      }
    } else if (Number.isFinite(file.percentage)) {
      percentageTotal += file.percentage
      percentageCount += 1
    }
  }

  if (totalLines > 0) return (hitLines / totalLines) * 100
  if (percentageCount > 0) return percentageTotal / percentageCount
  return undefined
}

function pluginCoverage(project: Project, plugin: PluginSettings, coverage?: CoverageReport): number | undefined {
  const languageCoverage = coverage?.languages?.find(
    (entry) => entry.language === plugin.language || entry.language === plugin.name
  )
  if (languageCoverage) return languageCoverage.percentage

  if (coverage?.files) {
    const fileCoverage = coverageFromFiles(coverage.files, plugin)
    if (fileCoverage !== undefined) return fileCoverage
  }

  if (project.plugins.length === 1) return coverage?.percentage
  return undefined
}

export function ProjectHeader({
  project,
  coverage,
  coverageLoading = false,
  coverageError,
}: {
  project?: Project
  coverage?: CoverageReport
  coverageLoading?: boolean
  coverageError?: Error
}) {
  if (!project) return null
  return (
    <header style={{ padding: '12px 16px', borderBottom: '1px solid #ddd', background: '#fafafa' }}>
      <h1 style={{ margin: 0, fontSize: 18 }}>{project.name}</h1>
      <div style={{ fontSize: 12, color: '#666', marginTop: 4, display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <span>{project.path}</span>
        {project.plugins.map((plugin) => {
          const percentage = pluginCoverage(project, plugin, coverage)
          const coverageText = coverageLoading
            ? 'coverage...'
            : coverageError
              ? 'coverage n/a'
              : percentage === undefined
                ? 'coverage n/a'
                : `${formatCoverage(percentage)} coverage`
          return (
            <span
              key={plugin.name}
              title={`LSP: ${plugin.lspServerAddress || '(none)'}; total coverage: ${
                percentage === undefined ? 'n/a' : formatCoverage(percentage)
              }`}
              data-testid={`plugin-chip-${plugin.name}`}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 4,
                padding: '2px 8px',
                background: '#eef',
                border: '1px solid #ccd',
                borderRadius: 999,
                fontSize: 11,
                color: '#334',
              }}
            >
              <span aria-hidden>{languageIcon(plugin.language)}</span>
              <span>{languageLabel(plugin.language)}</span>
              <span style={{ color: '#556' }}>· {coverageText}</span>
              {plugin.lspServerAddress && (
                <span style={{ color: '#778', fontFamily: 'monospace' }}>· {plugin.lspServerAddress.split(/\s+/)[0]}</span>
              )}
            </span>
          )
        })}
      </div>
    </header>
  )
}
