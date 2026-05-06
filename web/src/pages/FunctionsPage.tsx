import { useMemo, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { listSymbols, getSymbolTestPrompt, listMutators, runMutationTesting } from '../api/client.ts'
import type { Symbol, TestPrompt, Mutator, MutationRunReport } from '../api/types.ts'
import { useApi } from '../hooks/useApi.ts'
import { TreeView, type TreeNode } from '../components/TreeView.tsx'
import { languageFromId, languageIcon, languageLabel } from '../api/ids.ts'
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

function groupByLanguage(symbols: Symbol[]): Map<string, Symbol[]> {
  const map = new Map<string, Symbol[]>()
  for (const s of symbols) {
    const lang = languageFromId(s.id)
    const list = map.get(lang) || []
    list.push(s)
    map.set(lang, list)
  }
  return map
}

function toTreeNodes(symbols: Symbol[]): TreeNode[] {
  const byLanguage = groupByLanguage(symbols)
  const languages = Array.from(byLanguage.keys()).sort()
  return languages.map((language) => {
    const byFile = groupByFile(byLanguage.get(language)!)
    return {
      id: `lang:${language}`,
      label: (
        <span>
          {languageIcon(language)} {languageLabel(language)}
        </span>
      ),
      children: Array.from(byFile.entries()).map(([file, syms]) => ({
        id: `file:${language}:${file}`,
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
      })),
    }
  })
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

  const [mutators, setMutators] = useState<Mutator[]>([])
  const [mutatorsLoading, setMutatorsLoading] = useState(false)
  const [mutationReport, setMutationReport] = useState<MutationRunReport | null>(null)
  const [mutationLoading, setMutationLoading] = useState(false)
  const [mutationError, setMutationError] = useState<Error | null>(null)

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

  async function loadMutators() {
    setMutatorsLoading(true)
    try {
      const all = await listMutators(projectId!)
      const lang = selected ? languageFromId(selected.id) : ''
      setMutators(all.filter((m) => m.language === lang))
    } catch (e) {
      // ignore
    } finally {
      setMutatorsLoading(false)
    }
  }

  async function runMutator(mutator: Mutator) {
    if (!selected) return
    setMutationLoading(true)
    setMutationError(null)
    setMutationReport(null)
    try {
      const report = await runMutationTesting(projectId!, {
        language: mutator.language,
        files: [selected.file],
      })
      setMutationReport(report)
    } catch (e) {
      setMutationError(e as Error)
    } finally {
      setMutationLoading(false)
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
            if (!id.startsWith('file:') && !id.startsWith('lang:')) {
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
              <div style={{ marginTop: 16 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
                  <h4 style={{ margin: 0 }}>Codebase Mutators</h4>
                  <button
                    onClick={loadMutators}
                    style={{ padding: '2px 8px', cursor: 'pointer', fontSize: 12 }}
                  >
                    {mutatorsLoading ? 'Loading…' : 'Load'}
                  </button>
                </div>
                {mutators.length === 0 && !mutatorsLoading && (
                  <span style={{ fontSize: 12, color: '#888' }}>No mutators loaded.</span>
                )}
                {mutators.map((m) => (
                  <div key={m.id} style={{ marginBottom: 6 }}>
                    <button
                      onClick={() => runMutator(m)}
                      disabled={mutationLoading}
                      style={{ padding: '4px 10px', cursor: 'pointer', fontSize: 12 }}
                    >
                      {mutationLoading ? 'Running…' : m.name}
                    </button>
                    <span style={{ fontSize: 11, color: '#666', marginLeft: 6 }}>{m.description}</span>
                  </div>
                ))}
                {(mutationReport || mutationError) && (
                  <div style={{ marginTop: 8, padding: 8, background: '#f9f9f9', border: '1px solid #ddd', borderRadius: 4, fontSize: 12 }}>
                    {mutationReport && (
                      <div>
                        <div>
                          <strong>{mutationReport.tool}</strong> &mdash; score{' '}
                          <span style={{ color: mutationReport.score >= 0.8 ? '#2e7d32' : '#d32f2f' }}>
                            {(mutationReport.score * 100).toFixed(1)}%
                          </span>{' '}
                          ({mutationReport.killed} killed, {mutationReport.survived} survived,{' '}
                          {mutationReport.noCoverage} no-coverage, {mutationReport.timedOut} timeout,{' '}
                          {mutationReport.errored} errored, total {mutationReport.total})
                        </div>
                        {mutationReport.mutants.length > 0 && (
                          <table style={{ marginTop: 6, fontSize: 11, width: '100%', borderCollapse: 'collapse' }}>
                            <thead>
                              <tr style={{ textAlign: 'left', borderBottom: '1px solid #ddd' }}>
                                <th>File</th>
                                <th>Line</th>
                                <th>Operator</th>
                                <th>Status</th>
                              </tr>
                            </thead>
                            <tbody>
                              {mutationReport.mutants.slice(0, 50).map((m, i) => (
                                <tr key={i} style={{ borderBottom: '1px solid #f0f0f0' }}>
                                  <td>{m.file}</td>
                                  <td>{m.line}</td>
                                  <td>{m.operator}</td>
                                  <td style={{ color: m.status === 'killed' ? '#2e7d32' : m.status === 'survived' ? '#d32f2f' : '#888' }}>
                                    {m.status}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        )}
                        {mutationReport.output && (
                          <details style={{ marginTop: 6 }}>
                            <summary style={{ cursor: 'pointer' }}>Tool output</summary>
                            <pre style={{ maxHeight: 160, overflow: 'auto', fontSize: 11, background: '#fff', padding: 4 }}>
                              {mutationReport.output}
                            </pre>
                          </details>
                        )}
                      </div>
                    )}
                    {mutationError && <span style={{ color: '#d32f2f' }}>Error: {mutationError.message}</span>}
                  </div>
                )}
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
