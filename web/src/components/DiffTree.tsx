import { useState } from 'react'
import type { CSSProperties } from 'react'
import type { DiffNode } from '../api/types.ts'

function formatValue(v: unknown): string {
  if (v === undefined) return 'undefined'
  if (v === null) return 'null'
  if (typeof v === 'string') return `"${v}"`
  if (typeof v === 'object') return JSON.stringify(v)
  return String(v)
}

const kindStyles: Record<string, CSSProperties> = {
  added: { background: '#d4edda', color: '#155724' },
  removed: { background: '#f8d7da', color: '#721c24' },
  changed: { background: '#fff3cd', color: '#856404' },
  unchanged: {},
}

function DiffNodeView({ node, depth }: { node: DiffNode; depth: number }) {
  const [expanded, setExpanded] = useState(true)
  const hasChildren = (node.children?.length ?? 0) > 0
  const style = kindStyles[node.kind] || {}

  const label = (
    <span style={{ ...style, padding: '1px 4px', borderRadius: 3 }}>
      {node.key !== undefined && node.key !== null ? `${node.key}: ` : null}
      {hasChildren ? (
        <span style={{ cursor: 'pointer' }} onClick={() => setExpanded((e) => !e)}>
          {expanded ? '▼' : '▶'} {node.kind}
        </span>
      ) : (
        <span>
          {node.kind === 'changed' ? (
            <>
              <span style={{ textDecoration: 'line-through', opacity: 0.7 }}>{formatValue(node.oldValue)}</span>
              {' → '}
              <span style={{ fontWeight: 600 }}>{formatValue(node.newValue)}</span>
            </>
          ) : node.kind === 'added' ? (
            <span style={{ fontWeight: 600 }}>{formatValue(node.newValue)}</span>
          ) : node.kind === 'removed' ? (
            <span style={{ textDecoration: 'line-through', opacity: 0.7 }}>{formatValue(node.oldValue)}</span>
          ) : (
            formatValue(node.oldValue ?? node.newValue)
          )}
        </span>
      )}
    </span>
  )

  return (
    <div style={{ marginLeft: depth * 16, marginTop: 2, fontFamily: 'monospace', fontSize: 13 }}>
      <div data-testid={`diff-node-${node.kind}`}>{label}</div>
      {hasChildren && expanded && (
        <div>
          {node.children!.map((child, i) => (
            <DiffNodeView key={i} node={child} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  )
}

export function DiffTree({ root }: { root?: DiffNode | null }) {
  if (!root) {
    return <div style={{ padding: 8, color: '#888' }}>No diff available.</div>
  }
  return (
    <div data-testid="diff-tree" style={{ padding: 8 }}>
      <DiffNodeView node={root} depth={0} />
    </div>
  )
}
