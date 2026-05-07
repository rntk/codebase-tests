import { useMemo, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { listTests, listGoldenCases, getTestTestPrompt, getGoldenDiff, mutateGoldenCase } from '../api/client.ts'
import type { TestFunc, TestCase, GoldenCase, TestPrompt, DiffNode, GoldenCaseDiff, MutationResult } from '../api/types.ts'
import { useApi } from '../hooks/useApi.ts'
import { TreeView, type TreeNode } from '../components/TreeView.tsx'
import { languageFromId, languageIcon, languageLabel } from '../api/ids.ts'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import { EmptyState } from '../components/EmptyState.tsx'
import { PromptModal } from '../components/PromptModal.tsx'

function groupByFile(tests: TestFunc[]): Map<string, TestFunc[]> {
  const map = new Map<string, TestFunc[]>()
  for (const t of tests) {
    const list = map.get(t.file) || []
    list.push(t)
    map.set(t.file, list)
  }
  return map
}

function groupByLanguage(tests: TestFunc[]): Map<string, TestFunc[]> {
  const map = new Map<string, TestFunc[]>()
  for (const t of tests) {
    const lang = languageFromId(t.id)
    const list = map.get(lang) || []
    list.push(t)
    map.set(lang, list)
  }
  return map
}

function hasGoldenTestData(test: TestFunc): boolean {
  return test.hasGolden === true || (test.goldenCases?.length ?? 0) > 0
}

function hasDiffNode(node?: DiffNode | null): boolean {
  if (!node) return false
  if (node.kind !== 'unchanged') return true
  return node.children?.some(hasDiffNode) ?? false
}

function hasGoldenDiff(diff?: GoldenCaseDiff | null): boolean {
  return hasDiffNode(diff?.inDiff) || hasDiffNode(diff?.outDiff)
}

function toTreeNodes(tests: TestFunc[]): TreeNode[] {
  const byLanguage = groupByLanguage(tests)
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
      filterText: languageLabel(language),
      children: Array.from(byFile.entries()).map(([file, funcs]) => ({
        id: `file:${language}:${file}`,
        label: <span>📁 {file}</span>,
        filterText: file,
        children: funcs.map((t) => ({
          id: t.id,
          label: <span>🧪 {t.name}</span>,
          filterText: t.name,
          marker: hasGoldenTestData(t)
            ? {
                kind: 'success',
                label: 'Has golden test data',
                title: 'Golden test data found',
              }
            : undefined,
          children: t.subCases?.map((c: TestCase) => ({
            id: c.id,
            label: <span>📂 {c.name}</span>,
            filterText: c.name,
          })),
        })),
      })),
    }
  })
}

function GoldenCaseListItem({
  projectId,
  testId,
  goldenCase,
  refreshToken,
  onMutate,
}: {
  projectId: string
  testId: string
  goldenCase: GoldenCase
  refreshToken: number
  onMutate: () => void
}) {
  const { data: diff, loading: diffLoading } = useApi(
    () => getGoldenDiff(projectId, testId, goldenCase.id),
    [projectId, testId, goldenCase.id, refreshToken]
  )
  const caseUrl = `/projects/${projectId}/tests/${encodeURIComponent(testId)}/golden/${encodeURIComponent(goldenCase.id)}/diff`
  const changed = hasGoldenDiff(diff)

  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, width: '100%' }}>
      <Link to={caseUrl} style={{ color: '#1976d2', textDecoration: 'none', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {goldenCase.name}
      </Link>
      <div style={{ display: 'flex', alignItems: 'center', gap: 4, flexShrink: 0 }}>
        <button
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); onMutate(); }}
          style={{
            padding: '2px 8px',
            cursor: 'pointer',
            background: '#fff',
            border: '1px solid #ccc',
            borderRadius: 4,
            fontSize: 11,
          }}
        >
          Mutate
        </button>
        {changed && (
          <Link
            to={`${caseUrl}?tab=diff`}
            aria-label={`${goldenCase.name} has changes`}
            title="View diff"
            style={{
              color: '#856404',
              background: '#fff3cd',
              border: '1px solid #ffe082',
              width: 18,
              height: 18,
              borderRadius: 999,
              fontSize: 12,
              fontWeight: 700,
              lineHeight: 1,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              textDecoration: 'none',
            }}
          >
            Δ
          </Link>
        )}
        {!changed && diffLoading && (
          <span style={{ fontSize: 12, color: '#888' }}>…</span>
        )}
        <span style={{ fontSize: 11, color: '#888', marginLeft: 4 }}>
          {goldenCase.inExists ? 'in' : 'no-in'} / {goldenCase.outExists ? 'out' : 'no-out'}
        </span>
      </div>
    </div>
  )
}

function GoldenCaseList({
  projectId,
  testId,
  cases,
}: {
  projectId: string
  testId: string
  cases: GoldenCase[]
}) {
  const [refreshToken, setRefreshToken] = useState(0)
  const [mutationResults, setMutationResults] = useState<{caseId: string, results: MutationResult[]} | null>(null)
  const [mutationLoading, setMutationLoading] = useState(false)
  const [mutationError, setMutationError] = useState<Error | null>(null)

  const treeNodes: TreeNode[] = useMemo(() => {
    return cases.map(c => ({
      id: c.id,
      label: (
        <GoldenCaseListItem 
          projectId={projectId} 
          testId={testId} 
          goldenCase={c} 
          refreshToken={refreshToken}
          onMutate={async () => {
            setMutationLoading(true)
            setMutationError(null)
            setMutationResults(null)
            try {
              const res = await mutateGoldenCase(projectId, testId, c.id)
              setMutationResults({ caseId: c.id, results: res })
            } catch (e) {
              setMutationError(e as Error)
            } finally {
              setMutationLoading(false)
            }
          }}
        />
      )
    }))
  }, [cases, projectId, testId, refreshToken])

  return (
    <div style={{ marginTop: 12 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, margin: '8px 0' }}>
        <h4 style={{ margin: 0 }}>Golden Cases</h4>
        {cases.length > 0 && (
          <button
            onClick={() => setRefreshToken((value) => value + 1)}
            style={{
              padding: '2px 8px',
              cursor: 'pointer',
              background: '#fff',
              border: '1px solid #ccc',
              borderRadius: 4,
              fontSize: 12,
            }}
          >
            Refresh
          </button>
        )}
      </div>
      {cases.length === 0 ? (
        <EmptyState message="No golden cases." />
      ) : (
        <div style={{ border: '1px solid #eee', borderRadius: 4 }}>
          <TreeView nodes={treeNodes} />
        </div>
      )}

      {(mutationLoading || mutationResults || mutationError) && (
        <div style={{ 
          marginTop: 16, 
          padding: 12, 
          background: '#f9f9f9', 
          border: '1px solid #ddd', 
          borderRadius: 4,
          fontSize: 13 
        }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <h5 style={{ margin: 0 }}>Mutation Results {mutationResults?.caseId && `for ${mutationResults.caseId}`}</h5>
            <button onClick={() => { setMutationResults(null); setMutationError(null); }} style={{ cursor: 'pointer', fontSize: 11 }}>Close</button>
          </div>
          
          {mutationLoading && <LoadingState message="Running mutation tests..." />}
          {mutationError && <ErrorState error={mutationError} />}
          {mutationResults && (
            <div style={{ maxHeight: 300, overflow: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr style={{ textAlign: 'left', borderBottom: '1px solid #ccc' }}>
                    <th style={{ padding: 4 }}>Mutation</th>
                    <th style={{ padding: 4 }}>Result</th>
                  </tr>
                </thead>
                <tbody>
                  {mutationResults.results.map((r, i) => (
                    <tr key={i} style={{ borderBottom: '1px solid #eee' }}>
                      <td style={{ padding: 4 }}>{r.mutation}</td>
                      <td style={{ padding: 4, color: r.survived ? '#d32f2f' : '#2e7d32' }}>
                        {r.survived ? '❌ Survived (test should have failed)' : '✅ Killed (expected)'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

export function TestsPage() {
  const { projectId, testId } = useParams<{ projectId: string; testId?: string }>()
  const navigate = useNavigate()
  const { data, loading, error } = useApi(() => listTests(projectId!), [projectId])
  const {
    data: golden,
    loading: goldenLoading,
    error: goldenError,
  } = useApi(() => (testId ? listGoldenCases(projectId!, testId) : Promise.resolve([] as GoldenCase[])), [projectId, testId])

  const tree = useMemo(() => (data ? toTreeNodes(data) : []), [data])

  const selected = useMemo(() => {
    if (!data || !testId) return undefined
    return data.find((t) => t.id === testId)
  }, [data, testId])

  const [filterText, setFilterText] = useState('')
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
      setPrompt(await getTestTestPrompt(projectId!, id))
    } catch (e) {
      setPromptError(e as Error)
    } finally {
      setPromptLoading(false)
    }
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState error={error} />
  if (!data || data.length === 0) return <EmptyState message="No tests found." />

  return (
    <div style={{ display: 'flex', height: '100%' }}>
      <div style={{ flex: 1, overflow: 'auto', borderRight: '1px solid #eee' }}>
        <TreeView
          nodes={tree}
          selectedId={testId}
          filter={filterText || undefined}
          onFilterChange={(f) => setFilterText(f)}
          onSelect={(id) => {
            if (!id.startsWith('file:') && !id.startsWith('lang:')) {
              navigate(`/projects/${projectId}/tests/${encodeURIComponent(id)}`)
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
                <strong>File:</strong> {selected.file}:{selected.line}:{selected.column}
              </div>
              <div>
                <strong>Package:</strong> {selected.package}
              </div>
              <div style={{ marginTop: 12 }}>
                <button
                  onClick={() => openPrompt(selected.id)}
                  style={{ padding: '6px 12px', cursor: 'pointer' }}
                >
                  Convert to table + golden
                </button>
              </div>
              {selected.coveredFuncs && selected.coveredFuncs.length > 0 && (
                <div style={{ marginTop: 8 }}>
                  <strong>Functions under test:</strong>
                  <ul style={{ margin: '4px 0', paddingLeft: 20 }}>
                    {selected.coveredFuncs.map((fid) => (
                      <li key={fid}>
                        <a
                          href={`#/projects/${projectId}/functions/${fid}`}
                          style={{ color: '#1976d2' }}
                          onClick={(e) => {
                            e.preventDefault()
                            navigate(`/projects/${projectId}/functions/${encodeURIComponent(fid)}`)
                          }}
                        >
                          {fid}
                        </a>
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
            {goldenLoading && <LoadingState message="Loading golden cases…" />}
            {goldenError && <ErrorState error={goldenError} />}
            {golden && <GoldenCaseList projectId={projectId!} testId={testId!} cases={golden} />}
          </div>
        ) : (
          <EmptyState message="Select a test to view details." />
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
