import { useState, type CSSProperties, type ReactNode } from 'react'

export interface TreeNodeMarker {
  kind: 'success'
  label: string
  title?: string
}

export interface TreeNode {
  id: string
  label: ReactNode
  marker?: TreeNodeMarker
  children?: TreeNode[]
}

const markerStyleByKind: Record<TreeNodeMarker['kind'], CSSProperties> = {
  success: {
    color: '#2e7d32',
    background: '#e8f5e9',
    border: '1px solid #a5d6a7',
  },
}

function TreeItem({
  node,
  depth,
  selectedId,
  onSelect,
}: {
  node: TreeNode
  depth: number
  selectedId?: string
  onSelect?: (id: string) => void
}) {
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = (node.children?.length ?? 0) > 0
  const isSelected = selectedId === node.id

  return (
    <div>
      <div
        role="button"
        tabIndex={0}
        onClick={() => {
          if (hasChildren) setExpanded((e) => !e)
          onSelect?.(node.id)
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            if (hasChildren) setExpanded((e) => !e)
            onSelect?.(node.id)
          }
        }}
        style={{
          padding: '4px 8px',
          paddingLeft: 8 + depth * 16,
          cursor: 'pointer',
          background: isSelected ? '#e3f2fd' : 'transparent',
          display: 'flex',
          alignItems: 'center',
          gap: 4,
        }}
        data-testid={`tree-item-${node.id}`}
      >
        <span style={{ width: 16, display: 'inline-block', textAlign: 'center' }}>
          {hasChildren ? (expanded ? '▼' : '▶') : ' '}
        </span>
        <span style={{ flex: 1, minWidth: 0 }}>{node.label}</span>
        {node.marker && (
          <span
            aria-label={node.marker.label}
            title={node.marker.title ?? node.marker.label}
            data-testid={`tree-item-marker-${node.id}`}
            style={{
              ...markerStyleByKind[node.marker.kind],
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              width: 18,
              height: 18,
              borderRadius: 999,
              fontSize: 12,
              fontWeight: 700,
              lineHeight: 1,
              flex: '0 0 auto',
            }}
          >
            ✓
          </span>
        )}
      </div>
      {hasChildren && expanded && (
        <div>
          {node.children!.map((child) => (
            <TreeItem
              key={child.id}
              node={child}
              depth={depth + 1}
              selectedId={selectedId}
              onSelect={onSelect}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export function TreeView({
  nodes,
  selectedId,
  onSelect,
}: {
  nodes: TreeNode[]
  selectedId?: string
  onSelect?: (id: string) => void
}) {
  return (
    <div data-testid="tree-view">
      {nodes.map((node) => (
        <TreeItem key={node.id} node={node} depth={0} selectedId={selectedId} onSelect={onSelect} />
      ))}
    </div>
  )
}
