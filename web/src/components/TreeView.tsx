import { useState, useMemo, type CSSProperties, type ReactNode } from 'react'

export interface TreeNodeMarker {
  kind: 'success' | 'changed'
  label: string
  title?: string
  icon?: ReactNode
}

export interface TreeNode {
  id: string
  label: ReactNode
  marker?: TreeNodeMarker
  children?: TreeNode[]
  filterText?: string
}

const markerStyleByKind: Record<TreeNodeMarker['kind'], CSSProperties> = {
  success: {
    color: '#2e7d32',
    background: '#e8f5e9',
    border: '1px solid #a5d6a7',
  },
  changed: {
    color: '#856404',
    background: '#fff3cd',
    border: '1px solid #ffe082',
  },
}

function fuzzyMatch(text: string, query: string): boolean {
  if (!query) return true
  const lowerText = text.toLowerCase()
  const lowerQuery = query.toLowerCase()
  let ti = 0
  for (let qi = 0; qi < lowerQuery.length; qi++) {
    while (ti < lowerText.length && lowerText[ti] !== lowerQuery[qi]) {
      ti++
    }
    if (ti >= lowerText.length) return false
    ti++
  }
  return true
}

function filterTree(nodes: TreeNode[], query: string): TreeNode[] {
  if (!query) return nodes
  return nodes.reduce<TreeNode[]>((acc, node) => {
    const selfMatch = fuzzyMatch(node.filterText ?? node.id, query)
    const filteredChildren = node.children
      ? filterTree(node.children, query)
      : []

    if (selfMatch) {
      acc.push(node)
    } else if (filteredChildren.length > 0) {
      acc.push({ ...node, children: filteredChildren })
    }
    return acc
  }, [])
}

function TreeItem({
  node,
  depth,
  selectedId,
  onSelect,
  expandAll,
}: {
  node: TreeNode
  depth: number
  selectedId?: string
  onSelect?: (id: string) => void
  expandAll?: boolean
}) {
  const [expanded, setExpanded] = useState(depth < 2)
  const hasChildren = (node.children?.length ?? 0) > 0
  const isSelected = selectedId === node.id
  const effectiveExpanded = expandAll ? true : expanded

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
          {hasChildren ? (effectiveExpanded ? '▼' : '▶') : ' '}
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
            {node.marker.icon ?? (node.marker.kind === 'success' ? '✓' : 'Δ')}
          </span>
        )}
      </div>
      {hasChildren && effectiveExpanded && (
        <div>
          {node.children!.map((child) => (
            <TreeItem
              key={child.id}
              node={child}
              depth={depth + 1}
              selectedId={selectedId}
              onSelect={onSelect}
              expandAll={expandAll}
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
  filter,
  onFilterChange,
}: {
  nodes: TreeNode[]
  selectedId?: string
  onSelect?: (id: string) => void
  filter?: string
  onFilterChange?: (filter: string) => void
}) {
  const filteredNodes = useMemo(
    () => filterTree(nodes, filter ?? ''),
    [nodes, filter],
  )
  const expandAll = !!filter

  return (
    <div data-testid="tree-view">
      {onFilterChange && (
        <input
          type="text"
          placeholder="Filter..."
          value={filter ?? ''}
          onChange={(e) => onFilterChange(e.target.value)}
          style={{
            width: '100%',
            boxSizing: 'border-box',
            padding: '6px 8px',
            border: 'none',
            borderBottom: '1px solid #eee',
            outline: 'none',
            fontSize: 13,
            position: 'sticky',
            top: 0,
            background: '#fff',
            zIndex: 1,
          }}
          data-testid="tree-view-filter"
        />
      )}
      {filteredNodes.map((node) => (
        <TreeItem
          key={node.id}
          node={node}
          depth={0}
          selectedId={selectedId}
          onSelect={onSelect}
          expandAll={expandAll}
        />
      ))}
    </div>
  )
}
