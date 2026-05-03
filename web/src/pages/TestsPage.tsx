import { useMemo, useState } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { listTests, listGoldenCases, getTestTestPrompt, getGoldenDiff } from '../api/client.ts'
import type { TestFunc, TestCase, GoldenCase, TestPrompt, DiffNode, GoldenCaseDiff } from '../api/types.ts'
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

function hasDiffNode(node?: DiffNode | null): boolean {
  if (!node) return false
  if (node.kind !== 'unchanged') return true
  return node.children?.some(hasDiffNode) ?? false
}

function hasGoldenDiff(diff?: GoldenCaseDiff | null): boolean {
  return hasDiffNode(diff?.inDiff) || hasDiffNode(diff?.outDiff)
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

function GoldenCaseListItem({
  projectId,
  testId,
  goldenCase,
  refreshToken,
}: {
  projectId: string
  testId: string
  goldenCase: GoldenCase
  refreshToken: number
}) {
  const { data: diff, loading: diffLoading, error: diffError } = useApi(
    () => getGoldenDiff(projectId, testId, goldenCase.id),
    [projectId, testId, goldenCase.id, refreshToken]
  )
  const caseUrl = `/projects/${projectId}/tests/${encodeURIComponent(testId)}/golden/${encodeURIComponent(goldenCase.id)}/diff`
  const changed = hasGoldenDiff(diff)

  return (
    <li style={{ marginBottom: 4, display: 'flex', alignItems: 'center', gap: 8 }}>
      <Link to={caseUrl} style={{ color: '#1976d2', textDecoration: 'none' }}>
        {goldenCase.name}
      </Link>
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
            flex: '0 0 auto',
          }}
        >
          Δ
        </Link>
      )}
      {!changed && diffLoading && (
        <span
          aria-label={`Checking ${goldenCase.name} for changes`}
          title="Checking for changes"
          style={{ fontSize: 12, color: '#888' }}
        >
          …
        </span>
      )}
      {diffError && (
        <span
          aria-label={`Could not check ${goldenCase.name} for changes`}
          title={diffError.message}
          style={{
            color: '#b45309',
            background: '#fffbeb',
            border: '1px solid #fcd34d',
            width: 18,
            height: 18,
            borderRadius: 999,
            fontSize: 12,
            fontWeight: 700,
            lineHeight: 1,
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            flex: '0 0 auto',
          }}
        >
          !
        </span>
      )}
      <span style={{ fontSize: 12, color: '#888' }}>
        {goldenCase.inExists ? '✅ in' : '❌ in'} / {goldenCase.outExists ? '✅ out' : '❌ out'}
      </span>
    </li>
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
        <ul style={{ margin: 0, paddingLeft: 20 }}>
          {cases.map((c) => (
            <GoldenCaseListItem
              key={c.id}
              projectId={projectId}
              testId={testId}
              goldenCase={c}
              refreshToken={refreshToken}
            />
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
