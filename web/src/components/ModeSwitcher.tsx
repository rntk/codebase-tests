import { Link, useLocation } from 'react-router-dom'

const modes = [
  { key: 'files', label: 'Files' },
  { key: 'functions', label: 'Functions' },
  { key: 'tests', label: 'Tests' },
]

export function ModeSwitcher({ projectId }: { projectId: string }) {
  const location = useLocation()
  const parts = location.pathname.split('/')
  const currentMode = parts[3] || 'files'

  return (
    <nav
      style={{
        width: 140,
        minWidth: 140,
        borderRight: '1px solid #ddd',
        background: '#f5f5f5',
        padding: 8,
      }}
      data-testid="mode-switcher"
    >
      <div style={{ fontSize: 12, fontWeight: 700, color: '#666', marginBottom: 8, textTransform: 'uppercase' }}>
        Mode
      </div>
      {modes.map((m) => {
        const active = currentMode === m.key
        return (
          <Link
            key={m.key}
            to={`/projects/${projectId}/${m.key}`}
            style={{
              display: 'block',
              padding: '8px 12px',
              borderRadius: 4,
              textDecoration: 'none',
              color: active ? '#fff' : '#333',
              background: active ? '#1976d2' : 'transparent',
              marginBottom: 4,
              fontWeight: active ? 600 : 400,
            }}
          >
            {m.label}
          </Link>
        )
      })}
    </nav>
  )
}
