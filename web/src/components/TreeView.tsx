import { useState, type ReactNode } from 'react'

export interface TreeNode {
  id: string
  label: ReactNode
  children?: TreeNode[]
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
        {node.label}
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
