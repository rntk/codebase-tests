import { useMemo, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { listSymbols, getSymbolTestPrompt } from '../api/client.ts'
import type { Symbol, TestPrompt } from '../api/types.ts'
import { useApi } from '../hooks/useApi.ts'
import { TreeView, type TreeNode } from '../components/TreeView.tsx'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import { EmptyState } from '../components/EmptyState.tsx'
import { PromptModal } from '../components/PromptModal.tsx'

function groupByFile(symbols: Symbol[]): Map<string, Symbol[]> {
  const map = new Map<string, Symbol[]>()
  for (const s of symbols) {
    const list = map.get(s.file) || []
    list.push(s)
    map.set(s.file, list)
  }
  return map
}

function toTreeNodes(symbols: Symbol[]): TreeNode[] {
  const byFile = groupByFile(symbols)
  return Array.from(byFile.entries()).map(([file, syms]) => ({
    id: `file:${file}`,
    label: <span>📁 {file}</span>,
    children: syms.map((s) => ({
      id: s.id,
      label: (
        <span>
          {s.covered ? '✅' : '⚠️'}{' '}
          <span style={{ color: s.covered ? 'inherit' : '#c62828' }}>{s.name}</span>
        </span>
      ),
    })),
  }))
}

export function FunctionsPage() {
  const { projectId, symbolId } = useParams<{ projectId: string; symbolId?: string }>()
  const navigate = useNavigate()
  const { data, loading, error } = useApi(() => listSymbols(projectId!), [projectId])

  const tree = useMemo(() => (data ? toTreeNodes(data) : []), [data])

  const selected = useMemo(() => {
    if (!data || !symbolId) return undefined
    return data.find((s) => s.id === symbolId)
  }, [data, symbolId])

  const [prompt, setPrompt] = useState<TestPrompt | null>(null)
  const [promptOpen, setPromptOpen] = useState(false)
  const [promptLoading, setPromptLoading] = useState(false)
  const [promptError, setPromptError] = useState<Error | null>(null)

  async function openPrompt(id: string) {
    setPromptOpen(true)
    setPromptLoading(true)
    setPromptError(null)
    setPrompt(null)
    try {
      setPrompt(await getSymbolTestPrompt(projectId!, id))
    } catch (e) {
      setPromptError(e as Error)
    } finally {
      setPromptLoading(false)
    }
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState error={error} />
  if (!data || data.length === 0) return <EmptyState message="No functions found." />

  return (
    <div style={{ display: 'flex', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', borderRight: '1px solid #eee' }}>
        <TreeView
          nodes={tree}
          selectedId={symbolId}
          onSelect={(id) => {
            if (!id.startsWith('file:')) {
              navigate(`/projects/${projectId}/functions/${encodeURIComponent(id)}`)
            }
          }}
        />
      </div>
      <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
        {selected ? (
          <div>
            <h3 style={{ marginTop: 0 }}>{selected.name}</h3>
            <div style={{ fontSize: 13, color: '#555' }}>
              <div>
                <strong>Qualified:</strong> {selected.qualifiedName}
              </div>
              <div>
                <strong>Kind:</strong> {selected.kind}
              </div>
              <div>
                <strong>Package:</strong> {selected.package}
              </div>
              <div>
                <strong>Location:</strong> {selected.file}:{selected.line}:{selected.column}
              </div>
              <div style={{ marginTop: 12 }}>
                <button
                  onClick={() => openPrompt(selected.id)}
                  style={{ padding: '6px 12px', cursor: 'pointer' }}
                >
                  Generate tests
                </button>
              </div>
              <div style={{ marginTop: 8 }}>
                <strong>Covered:</strong>{' '}
                {selected.covered ? (
                  <span style={{ color: '#2e7d32' }}>Yes</span>
                ) : (
                  <span style={{ color: '#c62828' }}>No</span>
                )}
              </div>
              {selected.coveredBy && selected.coveredBy.length > 0 && (
                <div style={{ marginTop: 8 }}>
                  <strong>Covered by:</strong>
                  <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                    {selected.coveredBy.map((testId) => (
                      <li key={testId}>
                        <a
                          href={`#/projects/${projectId}/tests/${testId}`}
                          style={{ color: '#1976d2' }}
                          onClick={(e) => {
                            e.preventDefault()
                            navigate(`/projects/${projectId}/tests/${encodeURIComponent(testId)}`)
                          }}
                        >
                          {testId}
                        </a>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          </div>
        ) : (
          <EmptyState message="Select a function to view details." />
        )}
      </div>
      {promptOpen && (
        <PromptModal
          prompt={prompt}
          loading={promptLoading}
          error={promptError}
          onClose={() => setPromptOpen(false)}
        />
      )}
    </div>
  )
}
