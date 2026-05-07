import { useState, useMemo } from 'react'
import { useParams } from 'react-router-dom'
import { listFiles } from '../api/client.ts'
import type { FileNode } from '../api/types.ts'
import { useApi } from '../hooks/useApi.ts'
import { TreeView, type TreeNode } from '../components/TreeView.tsx'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import { EmptyState } from '../components/EmptyState.tsx'

function toTreeNodes(nodes: FileNode[]): TreeNode[] {
  return nodes.map((n) => ({
    id: n.path,
    label: (
      <span>
        {n.isDir ? '📁 ' : '📄 '}
        {n.name}
      </span>
    ),
    filterText: n.path,
    children: n.children ? toTreeNodes(n.children) : undefined,
  }))
}

export function FilesPage() {
  const { projectId } = useParams<{ projectId: string }>()
  const { data, loading, error } = useApi(() => listFiles(projectId!), [projectId])
  const [selectedPath, setSelectedPath] = useState<string | undefined>()
  const [filterText, setFilterText] = useState('')

  const tree = useMemo(() => (data ? toTreeNodes(data) : []), [data])

  const selectedNode = useMemo(() => {
    if (!data || !selectedPath) return undefined
    function find(nodes: FileNode[]): FileNode | undefined {
      for (const n of nodes) {
        if (n.path === selectedPath) return n
        if (n.children) {
          const found = find(n.children)
          if (found) return found
        }
      }
      return undefined
    }
    return find(data)
  }, [data, selectedPath])

  if (loading) return <LoadingState />
  if (error) return <ErrorState error={error} />
  if (!data || data.length === 0) return <EmptyState message="No files found." />

  return (
    <div style={{ display: 'flex', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', borderRight: '1px solid #eee' }}>
        <TreeView
          nodes={tree}
          selectedId={selectedPath}
          onSelect={setSelectedPath}
          filter={filterText || undefined}
          onFilterChange={(f) => setFilterText(f)}
        />
      </div>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {selectedNode ? (
          <div>
            <h3 style={{ marginTop: 0 }}>{selectedNode.name}</h3>
            <div style={{ fontSize: 13, color: '#555' }}>
              <div>
                <strong>Path:</strong> {selectedNode.path}
              </div>
              <div>
                <strong>Type:</strong> {selectedNode.isDir ? 'Directory' : 'File'}
              </div>
            </div>
          </div>
        ) : (
          <EmptyState message="Select a file to view details." />
        )}
      </div>
    </div>
  )
}
