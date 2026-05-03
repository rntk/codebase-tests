import { useMemo, useState, useRef, useCallback } from 'react'
import type { ReactNode } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getGoldenCase, getGoldenDiff, getTest, listSymbols } from '../api/client.ts'
import { useApi } from '../hooks/useApi.ts'
import { DiffTree } from '../components/DiffTree.tsx'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import type { GoldenCaseContent, TestFunc, SourceSnippet, Symbol } from '../api/types.ts'

function formatRawContent(value: unknown, emptyMessage: string): string {
  if (value === undefined) return emptyMessage
  return JSON.stringify(value, null, 2) ?? String(value)
}

function occursAsIdentifier(name: string, code: string): boolean {
  if (!name) return false
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`(?<![\\w.])${escaped}(?![\\w])`).test(code)
}

function findOccurrences(
  code: string,
  funcs: SourceSnippet[]
): Array<{ start: number; end: number; func: SourceSnippet }> {
  const matches: Array<{ start: number; end: number; func: SourceSnippet }> = []

  for (const fn of funcs) {
    const names = new Set([fn.name])
    if (fn.qualifiedName && fn.qualifiedName !== fn.name) names.add(fn.qualifiedName)

    for (const name of names) {
      // escape regex special chars, then match as identifier (word boundaries)
      const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      const regex = new RegExp(`(?<![\\w.])${escaped}(?![\\w])`, 'g')
      let m
      while ((m = regex.exec(code)) !== null) {
        matches.push({ start: m.index, end: m.index + m[0].length, func: fn })
      }
    }
  }

  matches.sort((a, b) => a.start - b.start)

  // remove overlaps, keeping earlier match
  const result: typeof matches = []
  let lastEnd = 0
  for (const m of matches) {
    if (m.start >= lastEnd) {
      result.push(m)
      lastEnd = m.end
    }
  }
  return result
}

function InteractiveCode({
  code,
  funcs,
  selectedFuncId,
  onFunctionClick,
}: {
  code: string
  funcs: SourceSnippet[]
  selectedFuncId: string | null
  onFunctionClick: (func: SourceSnippet) => void
}) {
  const occurrences = findOccurrences(code, funcs)
  if (!occurrences.length) return <>{code}</>

  const segments: ReactNode[] = []
  let pos = 0

  for (const occ of occurrences) {
    if (occ.start > pos) segments.push(code.slice(pos, occ.start))
    const isSelected = selectedFuncId === occ.func.id
    segments.push(
      <span
        key={`${occ.func.id}-${occ.start}`}
        onClick={() => onFunctionClick(occ.func)}
        title={`View code: ${occ.func.qualifiedName || occ.func.name}`}
        style={{
          cursor: 'pointer',
          backgroundColor: isSelected ? '#fef08a' : '#dbeafe',
          color: isSelected ? '#713f12' : '#1d4ed8',
          borderRadius: 2,
          padding: '0 2px',
          textDecoration: 'underline',
          textDecorationStyle: 'dotted',
        }}
      >
        {code.slice(occ.start, occ.end)}
      </span>
    )
    pos = occ.end
  }
  if (pos < code.length) segments.push(code.slice(pos))
  return <>{segments}</>
}

function CodeBlock({
  title,
  subtitle,
  code,
  highlighted,
  blockRef,
  children,
}: {
  title: string
  subtitle?: string
  code?: string
  highlighted?: boolean
  blockRef?: (el: HTMLDivElement | null) => void
  children?: ReactNode
}) {
  return (
    <div
      ref={blockRef}
      style={{
        border: highlighted ? '2px solid #2563eb' : '1px solid #ddd',
        borderRadius: 4,
        overflow: 'hidden',
        transition: 'border-color 0.2s',
      }}
    >
      <div
        style={{
          padding: '8px 12px',
          background: highlighted ? '#eff6ff' : '#f5f5f5',
          borderBottom: highlighted ? '1px solid #bfdbfe' : '1px solid #ddd',
          fontWeight: 600,
          fontSize: 13,
          transition: 'background 0.2s',
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
        {children ?? (code || 'Source code is not available.')}
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
  const [selectedFuncId, setSelectedFuncId] = useState<string | null>(null)
  const funcEls = useRef<Map<string, HTMLDivElement | null>>(new Map())

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
  const {
    data: symbols,
    loading: symbolsLoading,
    error: symbolsError,
  } = useApi(
    () => (rawMode ? listSymbols(projectId!, true) : Promise.resolve(null as Symbol[] | null)),
    [projectId, rawMode]
  )

  const getFuncRef = useCallback((id: string) => (el: HTMLDivElement | null) => {
    funcEls.current.set(id, el)
  }, [])

  const handleFunctionClick = useCallback((func: SourceSnippet) => {
    setSelectedFuncId(func.id)
    funcEls.current.get(func.id)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }, [])

  const referencedFuncs = useMemo<SourceSnippet[]>(() => {
    const code = test?.sourceCode ?? ''
    if (!code) return []
    const merged = new Map<string, SourceSnippet>()
    for (const fn of test?.coveredFuncSources ?? []) merged.set(fn.id, fn)
    for (const s of symbols ?? []) {
      if (s.kind !== 'function' && s.kind !== 'method') continue
      if (s.id === test?.id) continue
      if (merged.has(s.id)) continue
      merged.set(s.id, {
        id: s.id,
        name: s.name,
        qualifiedName: s.qualifiedName,
        kind: s.kind,
        file: s.file,
        line: s.line,
        column: s.column,
        package: s.package,
        sourceCode: s.sourceCode,
      })
    }
    return Array.from(merged.values()).filter(
      (fn) => occursAsIdentifier(fn.name, code) || occursAsIdentifier(fn.qualifiedName, code)
    )
  }, [test?.sourceCode, test?.coveredFuncSources, test?.id, symbols])

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

  const selectedFunc = (() => {
    const fromReferenced = referencedFuncs.find((fn) => fn.id === selectedFuncId)
    if (fromReferenced) return fromReferenced
    const s = symbols?.find((sym) => sym.id === selectedFuncId)
    if (!s) return null
    return {
      id: s.id,
      name: s.name,
      qualifiedName: s.qualifiedName,
      kind: s.kind,
      file: s.file,
      line: s.line,
      column: s.column,
      package: s.package,
      sourceCode: s.sourceCode,
    }
  })()

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
                subtitle={
                  test
                    ? `${test.file}:${test.line}${
                        referencedFuncs.length
                          ? ` · ${referencedFuncs.length} function${
                              referencedFuncs.length === 1 ? '' : 's'
                            } — click to view`
                          : symbolsLoading
                          ? ' · loading symbols…'
                          : ' · no symbols matched'
                      }`
                    : undefined
                }
              >
                {test?.sourceCode ? (
                  <InteractiveCode
                    code={test.sourceCode}
                    funcs={referencedFuncs}
                    selectedFuncId={selectedFuncId}
                    onFunctionClick={handleFunctionClick}
                  />
                ) : (
                  'Source code is not available.'
                )}
              </CodeBlock>
              <CodeBlock
                blockRef={selectedFunc ? getFuncRef(selectedFunc.id) : undefined}
                highlighted={!!selectedFunc}
                title={
                  selectedFunc
                    ? `Function Code: ${selectedFunc.qualifiedName || selectedFunc.name}`
                    : 'Function Code'
                }
                subtitle={selectedFunc ? `${selectedFunc.file}:${selectedFunc.line}` : undefined}
                code={
                  selectedFunc
                    ? selectedFunc.sourceCode || 'Source code is not available for this symbol.'
                    : referencedFuncs.length
                    ? 'Click a highlighted function in the test code to view its source.'
                    : symbolsLoading
                    ? 'Loading symbols…'
                    : 'No referenced functions detected in this test.'
                }
              />
            </>
          )}
        </div>
      )}

      {rawMode && (
        <div
          style={{
            marginTop: 16,
            border: '1px solid #ddd',
            borderRadius: 4,
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              padding: '8px 12px',
              background: '#f5f5f5',
              borderBottom: '1px solid #ddd',
              fontWeight: 600,
              fontSize: 13,
              display: 'flex',
              alignItems: 'center',
              gap: 8,
            }}
          >
            Symbols
            {symbols && symbols.length > 0 && (
              <span style={{ color: '#777', fontWeight: 400, fontSize: 12 }}>
                {symbols.length} total
              </span>
            )}
          </div>
          <div style={{ maxHeight: 400, overflow: 'auto' }}>
            {symbolsLoading ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12 }}>Loading symbols...</pre>
            ) : symbolsError ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12, color: '#dc2626' }}>
                Error loading symbols: {symbolsError.message}
              </pre>
            ) : symbols && symbols.length > 0 ? (
              symbols.map((sym) => {
                const isSelected = selectedFuncId === sym.id
                return (
                  <div
                    key={sym.id}
                    onClick={() => {
                      setSelectedFuncId(sym.id)
                      funcEls.current.get(sym.id)?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
                    }}
                    style={{
                      padding: '6px 12px',
                      cursor: 'pointer',
                      borderBottom: '1px solid #f0f0f0',
                      background: isSelected ? '#eff6ff' : undefined,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 8,
                      fontSize: 12,
                    }}
                  >
                    <span
                      style={{
                        background:
                          sym.kind === 'function' ? '#2563eb' :
                          sym.kind === 'method' ? '#7c3aed' :
                          sym.kind === 'class' ? '#dc2626' :
                          sym.kind === 'interface' ? '#0891b2' :
                          sym.kind === 'struct' ? '#ca8a04' :
                          '#6b7280',
                        color: '#fff',
                        borderRadius: 3,
                        padding: '1px 6px',
                        fontSize: 10,
                        fontWeight: 600,
                        textTransform: 'uppercase',
                        flexShrink: 0,
                      }}
                    >
                      {sym.kind}
                    </span>
                    <span style={{ fontWeight: 500, color: '#1d4ed8' }}>
                      {sym.name}
                    </span>
                    {sym.qualifiedName !== sym.name && (
                      <span style={{ color: '#666' }}>
                        {sym.qualifiedName}
                      </span>
                    )}
                    <span style={{ color: '#999', marginLeft: 'auto', flexShrink: 0, fontSize: 11 }}>
                      {sym.file}:{sym.line}
                    </span>
                    {sym.covered && (
                      <span title="Covered" style={{ color: '#16a34a', flexShrink: 0 }}>✓</span>
                    )}
                  </div>
                )
              })
            ) : (
              <pre style={{ padding: 12, margin: 0, fontSize: 12, color: '#777' }}>
                No symbols found.
              </pre>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
