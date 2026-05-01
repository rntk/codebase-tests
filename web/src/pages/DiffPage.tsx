import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getGoldenCase, getGoldenDiff, getTest } from '../api/client.ts'
import { useApi } from '../hooks/useApi.ts'
import { DiffTree } from '../components/DiffTree.tsx'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import type { GoldenCaseContent, TestFunc } from '../api/types.ts'

function formatRawContent(value: unknown, emptyMessage: string): string {
  if (value === undefined) return emptyMessage
  return JSON.stringify(value, null, 2) ?? String(value)
}

function CodeBlock({ title, subtitle, code }: { title: string; subtitle?: string; code?: string }) {
  return (
    <div style={{ border: '1px solid #ddd', borderRadius: 4, overflow: 'hidden' }}>
      <div
        style={{
          padding: '8px 12px',
          background: '#f5f5f5',
          borderBottom: '1px solid #ddd',
          fontWeight: 600,
          fontSize: 13,
        }}
      >
        {title}
        {subtitle && (
          <span style={{ marginLeft: 8, color: '#777', fontWeight: 400 }}>
            {subtitle}
          </span>
        )}
      </div>
      <pre
        style={{
          padding: 12,
          margin: 0,
          fontSize: 12,
          overflow: 'auto',
          whiteSpace: 'pre-wrap',
        }}
      >
        {code || 'Source code is not available.'}
      </pre>
    </div>
  )
}

export function DiffPage() {
  const { projectId, testId, caseId } = useParams<{
    projectId: string
    testId: string
    caseId: string
  }>()
  const navigate = useNavigate()
  const [rawMode, setRawMode] = useState(false)
  const {
    data,
    loading,
    error,
  } = useApi(() => getGoldenDiff(projectId!, testId!, caseId!), [projectId, testId, caseId])
  const {
    data: content,
    loading: contentLoading,
    error: contentError,
  } = useApi(
    () => (rawMode ? getGoldenCase(projectId!, testId!, caseId!) : Promise.resolve(null as GoldenCaseContent | null)),
    [projectId, testId, caseId, rawMode]
  )
  const {
    data: test,
    loading: testLoading,
    error: testError,
  } = useApi(
    () => (rawMode ? getTest(projectId!, testId!) : Promise.resolve(null as TestFunc | null)),
    [projectId, testId, rawMode]
  )

  const isNoGit =
    (error?.message?.toLowerCase()?.includes('no git repo') ?? false) ||
    (error?.message?.toLowerCase()?.includes('untracked') ?? false)

  if (loading) return <LoadingState />
  if (error && !isNoGit) return <ErrorState error={error} />
  if (rawMode && contentError) return <ErrorState error={contentError} />
  if (rawMode && testError) return <ErrorState error={testError} />

  const inDiff = data?.inDiff
  const outDiff = data?.outDiff
  const inContent = formatRawContent(content?.in, 'No input content.')
  const outContent = formatRawContent(content?.out, 'No output content.')

  return (
    <div style={{ padding: 16 }}>
      <button
        onClick={() => navigate(`/projects/${projectId}/tests/${encodeURIComponent(testId!)}`)}
        style={{
          marginBottom: 12,
          padding: '6px 12px',
          cursor: 'pointer',
          background: '#fff',
          border: '1px solid #ccc',
          borderRadius: 4,
        }}
      >
        ← Back to test
      </button>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0 }}>
          Diff: {data?.name ?? caseId}
        </h2>
        <label style={{ display: 'flex', alignItems: 'center', gap: 6, cursor: 'pointer' }}>
          <input type="checkbox" checked={rawMode} onChange={(e) => setRawMode(e.target.checked)} />
          View raw
        </label>
      </div>

      {isNoGit && (
        <div
          style={{
            padding: 12,
            marginTop: 12,
            background: '#fff3cd',
            color: '#856404',
            borderRadius: 4,
          }}
        >
          {error?.message}
        </div>
      )}

      <div style={{ display: 'flex', gap: 16, marginTop: 16 }}>
        <div style={{ flex: 1, border: '1px solid #ddd', borderRadius: 4, overflow: 'hidden' }}>
          <div
            style={{
              padding: '8px 12px',
              background: '#f5f5f5',
              borderBottom: '1px solid #ddd',
              fontWeight: 600,
              fontSize: 13,
            }}
          >
            {rawMode ? 'Input Content' : 'Input Diff'}
          </div>
          <div style={{ maxHeight: 600, overflow: 'auto' }}>
            {rawMode ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12 }}>
                {contentLoading ? 'Loading input content...' : inContent}
              </pre>
            ) : (
              <DiffTree root={inDiff} />
            )}
          </div>
        </div>

        <div style={{ flex: 1, border: '1px solid #ddd', borderRadius: 4, overflow: 'hidden' }}>
          <div
            style={{
              padding: '8px 12px',
              background: '#f5f5f5',
              borderBottom: '1px solid #ddd',
              fontWeight: 600,
              fontSize: 13,
            }}
          >
            {rawMode ? 'Output Content' : 'Output Diff'}
          </div>
          <div style={{ maxHeight: 600, overflow: 'auto' }}>
            {rawMode ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12 }}>
                {contentLoading ? 'Loading output content...' : outContent}
              </pre>
            ) : (
              <DiffTree root={outDiff} />
            )}
          </div>
        </div>
      </div>

      {rawMode && (
        <div
          style={{
            marginTop: 16,
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))',
            gap: 16,
          }}
        >
          {testLoading ? (
            <CodeBlock title="Test Code" code="Loading test code..." />
          ) : (
            <>
              <CodeBlock
                title={`Test Code: ${test?.name ?? testId}`}
                subtitle={test ? `${test.file}:${test.line}` : undefined}
                code={test?.sourceCode}
              />
              {test?.coveredFuncSources && test.coveredFuncSources.length > 0 ? (
                test.coveredFuncSources.map((fn) => (
                  <CodeBlock
                    key={fn.id}
                    title={`Function Code: ${fn.qualifiedName || fn.name}`}
                    subtitle={`${fn.file}:${fn.line}`}
                    code={fn.sourceCode}
                  />
                ))
              ) : (
                <CodeBlock
                  title="Function Code"
                  code={
                    test?.coveredFuncs && test.coveredFuncs.length > 0
                      ? `Source code is not available for:\n${test.coveredFuncs.join('\n')}`
                      : 'No covered function was found for this test.'
                  }
                />
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
