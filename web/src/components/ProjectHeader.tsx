import type { Project } from '../api/types.ts'
import { languageIcon, languageLabel } from '../api/ids.ts'

export function ProjectHeader({ project }: { project?: Project }) {
  if (!project) return null
  return (
    <header style={{ padding: '12px 16px', borderBottom: '1px solid #ddd', background: '#fafafa' }}>
      <h1 style={{ margin: 0, fontSize: 18 }}>{project.name}</h1>
      <div style={{ fontSize: 12, color: '#666', marginTop: 4, display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
        <span>{project.path}</span>
        {project.plugins.map((plugin) => (
          <span
            key={plugin.name}
            title={`LSP: ${plugin.lspServerAddress || '(none)'}`}
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
            {plugin.lspServerAddress && (
              <span style={{ color: '#778', fontFamily: 'monospace' }}>· {plugin.lspServerAddress.split(/\s+/)[0]}</span>
            )}
          </span>
        ))}
      </div>
    </header>
  )
}
