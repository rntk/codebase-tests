import type { Project } from '../api/types.ts'

export function ProjectHeader({ project }: { project?: Project }) {
  if (!project) return null
  return (
    <header style={{ padding: '12px 16px', borderBottom: '1px solid #ddd', background: '#fafafa' }}>
      <h1 style={{ margin: 0, fontSize: 18 }}>{project.name}</h1>
      <div style={{ fontSize: 12, color: '#666', marginTop: 4 }}>
        {project.path} • {project.plugins.map((plugin) => `${plugin.name} (${plugin.language}: ${plugin.lspServerAddress})`).join(', ')}
      </div>
    </header>
  )
}
