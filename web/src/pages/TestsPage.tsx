import { useMemo, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { listTests, listGoldenCases, getTestTestPrompt } from '../api/client.ts'
import type { TestFunc, TestCase, GoldenCase, TestPrompt } from '../api/types.ts'
import { useApi } from '../hooks/useApi.ts'
import { TreeView, type TreeNode } from '../components/TreeView.tsx'
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

function hasGoldenTestData(test: TestFunc): boolean {
  return test.hasGolden === true || (test.goldenCases?.length ?? 0) > 0
}

function toTreeNodes(tests: TestFunc[]): TreeNode[] {
  const byFile = groupByFile(tests)
  return Array.from(byFile.entries()).map(([file, funcs]) => ({
    id: `file:${file}`,
    label: <span>📁 {file}</span>,
    children: funcs.map((t) => ({
      id: t.id,
      label: <span>🧪 {t.name}</span>,
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
      })),
    })),
  }))
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
  return (
    <div style={{ marginTop: 12 }}>
      <h4 style={{ margin: '8px 0' }}>Golden Cases</h4>
      {cases.length === 0 ? (
        <EmptyState message="No golden cases." />
      ) : (
        <ul style={{ margin: 0, paddingLeft: 20 }}>
          {cases.map((c) => (
            <li key={c.id} style={{ marginBottom: 4 }}>
              <Link
                to={`/projects/${projectId}/tests/${encodeURIComponent(testId)}/golden/${encodeURIComponent(c.id)}/diff`}
                style={{ color: '#1976d2', textDecoration: 'none' }}
              >
                {c.name}
              </Link>
              <span style={{ fontSize: 12, color: '#888', marginLeft: 8 }}>
                {c.inExists ? '✅ in' : '❌ in'} / {c.outExists ? '✅ out' : '❌ out'}
              </span>
            </li>
          ))}
        </ul>
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
          onSelect={(id) => {
            if (!id.startsWith('file:')) {
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
