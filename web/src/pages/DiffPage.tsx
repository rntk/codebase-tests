import { useEffect, useMemo, useState, useRef, useCallback } from 'react'
import type { ReactNode } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { getGoldenCase, getGoldenDiff, getTest, listSymbols, mutateGoldenCase } from '../api/client.ts'
import { useApi } from '../hooks/useApi.ts'
import { DiffTree } from '../components/DiffTree.tsx'
import { LoadingState } from '../components/LoadingState.tsx'
import { ErrorState } from '../components/ErrorState.tsx'
import type { GoldenCaseContent, TestFunc, SourceSnippet, Symbol, DiffNode, MutationResult } from '../api/types.ts'
import { languageFromId, languageLabel } from '../api/ids.ts'

function formatRawContent(value: unknown, emptyMessage: string): string {
  if (value === undefined) return emptyMessage
  return JSON.stringify(value, null, 2) ?? String(value)
}

function truncateForPrompt(value: string, max = 6000): string {
  if (value.length <= max) return value
  return `${value.slice(0, max)}\n\n[truncated ${value.length - max} characters]`
}

function formatFunctionList(funcs: SourceSnippet[]): string {
  if (!funcs.length) return '- No referenced functions were detected for this test.'
  return funcs
    .map((fn) => {
      const name = fn.qualifiedName || fn.name
      const location = fn.file ? `${fn.file}:${fn.line}` : 'unknown file'
      return `- ${name} (${fn.kind || 'symbol'}) at ${location}`
    })
    .join('\n')
}

function formatFunctionSources(funcs: SourceSnippet[]): string {
  const withSource = funcs.filter((fn) => fn.sourceCode)
  if (!withSource.length) return 'No referenced function source is available.'
  return withSource
    .map((fn) => {
      const name = fn.qualifiedName || fn.name
      return `### ${name} (${fn.file}:${fn.line})\n\`\`\`\n${truncateForPrompt(fn.sourceCode ?? '')}\n\`\`\``
    })
    .join('\n\n')
}

function mutationReason(result: MutationResult): string {
  if (result.survived) {
    return 'The mutation survived: the selected test still passed after the golden input was changed. This means the test or golden data is too weak and should fail for this mutation.'
  }
  return 'The mutation was killed: the selected test failed after the golden input was changed. This result is expected, but the runner output is included in case it reveals an unexpected failure mode.'
}

function buildMutationFixPrompt({
  projectId,
  testId,
  caseId,
  caseName,
  language,
  test,
  functions,
  inputContent,
  outputContent,
  result,
}: {
  projectId?: string
  testId?: string
  caseId?: string
  caseName?: string
  language: string
  test?: TestFunc | null
  functions: SourceSnippet[]
  inputContent: string
  outputContent: string
  result: MutationResult
}): string {
  const testName = test?.name ?? testId ?? 'unknown test'
  const testLocation = test?.file ? `${test.file}:${test.line}:${test.column}` : 'unknown file'
  const testSource = test?.sourceCode
    ? `\`\`\`\n${truncateForPrompt(test.sourceCode)}\n\`\`\``
    : 'Test source is not available.'
  const runnerOutput = result.output?.trim()
    ? truncateForPrompt(result.output.trim(), 8000)
    : 'No runner output was captured.'

  return `You are fixing a weak or failing golden test after mutation testing.

Goal:
Update the test and/or golden fixture so the test catches the mutation below. Preserve correct production behavior unless the test reveals a real implementation bug.

Mutation result:
- Mutation: ${result.mutation}
- Result: ${result.survived ? 'SURVIVED (bad: test should have failed)' : 'KILLED (expected)'}
- Reason: ${mutationReason(result)}

Test context:
- Project: ${projectId ?? 'unknown project'}
- Language: ${languageLabel(language)}
- Test name: ${testName}
- Test id: ${testId ?? 'unknown test id'}
- Test file: ${testLocation}
- Golden case: ${caseName ?? caseId ?? 'unknown case'}
- Golden case id: ${caseId ?? 'unknown case id'}

Referenced functions:
${formatFunctionList(functions)}

Runner output:
\`\`\`
${runnerOutput}
\`\`\`

Current test source:
${testSource}

Current golden input:
\`\`\`json
${truncateForPrompt(inputContent)}
\`\`\`

Current expected golden output:
\`\`\`json
${truncateForPrompt(outputContent)}
\`\`\`

Referenced function source:
${formatFunctionSources(functions)}

Please explain the failing reason briefly, then make the smallest code or fixture change needed so this mutation is caught by the test.`
}

function hasDiffNode(node?: DiffNode | null): boolean {
  if (!node) return false
  if (node.kind !== 'unchanged') return true
  return node.children?.some(hasDiffNode) ?? false
}

// maskRanges returns inclusive-exclusive [start, end) ranges within `code`
// covering Go-style line comments, block comments, and string literals.
// Matches inside these ranges should be ignored when highlighting identifiers.
function maskRanges(code: string): Array<[number, number]> {
  const ranges: Array<[number, number]> = []
  const re = /\/\/[^\n]*|\/\*[\s\S]*?\*\/|"(?:[^"\\\n]|\\.)*"|`[^`]*`/g
  let m: RegExpExecArray | null
  while ((m = re.exec(code)) !== null) {
    ranges.push([m.index, m.index + m[0].length])
  }
  return ranges
}

function isMasked(pos: number, ranges: Array<[number, number]>): boolean {
  for (const [s, e] of ranges) {
    if (pos >= s && pos < e) return true
    if (s > pos) break
  }
  return false
}

function findOccurrences(
  code: string,
  funcs: SourceSnippet[]
): Array<{ start: number; end: number; func: SourceSnippet }> {
  const masked = maskRanges(code)
  const matches: Array<{ start: number; end: number; func: SourceSnippet }> = []

  for (const fn of funcs) {
    const names = new Set([fn.name])
    if (fn.qualifiedName && fn.qualifiedName !== fn.name) names.add(fn.qualifiedName)

    for (const name of names) {
      if (!name) continue
      const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
      // Disallow word chars or `.` before (so `pkg.Foo` matches qualifiedName
      // but plain `Foo` won't match the `.Foo` in `pkg.Foo`); disallow word
      // chars after. Mirrors backend boundary rules.
      const regex = new RegExp(`(?<![\\w.])${escaped}(?![\\w])`, 'g')
      let m: RegExpExecArray | null
      while ((m = regex.exec(code)) !== null) {
        if (isMasked(m.index, masked)) continue
        matches.push({ start: m.index, end: m.index + m[0].length, func: fn })
      }
    }
  }

  matches.sort((a, b) => a.start - b.start)

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
  const occurrences = useMemo(() => findOccurrences(code, funcs), [code, funcs])
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
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab') === 'diff' ? 'diff' : 'raw'
  const shouldLoadRaw = requestedTab === 'raw'
  const currentLanguage = testId ? languageFromId(testId) : 'unknown'
  const symbolLanguage = currentLanguage === 'unknown' ? undefined : currentLanguage
  const [selectedFuncId, setSelectedFuncId] = useState<string | null>(null)
  const [symbolFilter, setSymbolFilter] = useState('')
  const funcEls = useRef<Map<string, HTMLDivElement>>(new Map())

  const [mutationResults, setMutationResults] = useState<MutationResult[] | null>(null)
  const [mutationLoading, setMutationLoading] = useState(false)
  const [mutationError, setMutationError] = useState<Error | null>(null)
  const [copiedMutationIndex, setCopiedMutationIndex] = useState<number | null>(null)

  const {
    data,
    loading,
    error,
    refetch: refetchDiff,
  } = useApi(() => getGoldenDiff(projectId!, testId!, caseId!), [projectId, testId, caseId])
  const {
    data: content,
    loading: contentLoading,
    error: contentError,
  } = useApi(
    () => (shouldLoadRaw ? getGoldenCase(projectId!, testId!, caseId!) : Promise.resolve(null as GoldenCaseContent | null)),
    [projectId, testId, caseId, shouldLoadRaw]
  )
  const {
    data: test,
    loading: testLoading,
    error: testError,
  } = useApi(
    () => (shouldLoadRaw ? getTest(projectId!, testId!) : Promise.resolve(null as TestFunc | null)),
    [projectId, testId, shouldLoadRaw]
  )
  const {
    data: symbols,
    loading: symbolsLoading,
    error: symbolsError,
  } = useApi(
    () => (shouldLoadRaw ? listSymbols(projectId!, true, symbolLanguage) : Promise.resolve(null as Symbol[] | null)),
    [projectId, shouldLoadRaw, symbolLanguage]
  )

  const setFuncRef = useCallback((id: string, el: HTMLDivElement | null) => {
    if (el) funcEls.current.set(id, el)
    else funcEls.current.delete(id)
  }, [])

  // Highlightable functions in the test source come from the server's
  // coveredFuncSources, which is already filtered using token-aware matching.
  // We don't re-derive matches client-side from the full symbol list — that
  // duplicates server logic and risks divergence.
  const referencedFuncs: SourceSnippet[] = useMemo(
    () =>
      (test?.coveredFuncSources ?? []).filter(
        (fn) => !symbolLanguage || languageFromId(fn.id) === symbolLanguage
      ),
    [test?.coveredFuncSources, symbolLanguage]
  )

  const languageSymbols = useMemo(
    () => (symbols ?? []).filter((sym) => !symbolLanguage || languageFromId(sym.id) === symbolLanguage),
    [symbols, symbolLanguage]
  )

  const filteredLanguageSymbols = useMemo(() => {
    if (!symbolFilter) return languageSymbols
    const q = symbolFilter.toLowerCase()
    return languageSymbols.filter(
      (sym) =>
        sym.name.toLowerCase().includes(q) ||
        (sym.qualifiedName?.toLowerCase().includes(q) ?? false) ||
        sym.file.toLowerCase().includes(q)
    )
  }, [languageSymbols, symbolFilter])

  // Symbols not in coveredFuncSources are still selectable from the symbols
  // panel; this map lets the function-code panel resolve any selected id.
  const symbolById = useMemo(() => {
    const m = new Map<string, SourceSnippet>()
    for (const fn of referencedFuncs) m.set(fn.id, fn)
    for (const s of languageSymbols) {
      if (m.has(s.id)) continue
      m.set(s.id, {
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
    return m
  }, [referencedFuncs, languageSymbols])

  const selectedFunc = selectedFuncId ? symbolById.get(selectedFuncId) ?? null : null

  useEffect(() => {
    if (selectedFuncId && !symbolById.has(selectedFuncId)) {
      setSelectedFuncId(null)
    }
  }, [selectedFuncId, symbolById])

  // Scroll the function-code panel into view when the selection changes.
  // This fires AFTER the ref attaches (the panel re-renders with a new id).
  useEffect(() => {
    if (!selectedFuncId) return
    funcEls.current
      .get(selectedFuncId)
      ?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
  }, [selectedFuncId])

  const handleFunctionClick = useCallback((func: SourceSnippet) => {
    setSelectedFuncId(func.id)
  }, [])

  const isNoGit =
    (error?.message?.toLowerCase()?.includes('no git repo') ?? false) ||
    (error?.message?.toLowerCase()?.includes('untracked') ?? false)

  const inDiff = data?.inDiff
  const outDiff = data?.outDiff
  const hasDiff = hasDiffNode(inDiff) || hasDiffNode(outDiff)
  const rawMode = requestedTab !== 'diff' || !hasDiff

  useEffect(() => {
    if (!data || requestedTab !== 'diff' || hasDiff) return
    setSearchParams({}, { replace: true })
  }, [data, requestedTab, hasDiff, setSearchParams])

  if (loading) return <LoadingState />
  if (error && !isNoGit) return <ErrorState error={error} />
  if (rawMode && contentError) return <ErrorState error={contentError} />
  if (rawMode && testError) return <ErrorState error={testError} />

  const inContent = formatRawContent(content?.in, 'No input content.')
  const outContent = formatRawContent(content?.out, 'No output content.')

  async function handleMutate() {
    setMutationLoading(true)
    setMutationError(null)
    setMutationResults(null)
    setCopiedMutationIndex(null)
    try {
      const res = await mutateGoldenCase(projectId!, testId!, caseId!)
      setMutationResults(res)
    } catch (e) {
      setMutationError(e as Error)
    } finally {
      setMutationLoading(false)
    }
  }

  async function copyMutationPrompt(result: MutationResult, index: number) {
    const prompt = buildMutationFixPrompt({
      projectId,
      testId,
      caseId,
      caseName: data?.name,
      language: currentLanguage,
      test,
      functions: referencedFuncs,
      inputContent: inContent,
      outputContent: outContent,
      result,
    })

    try {
      await navigator.clipboard.writeText(prompt)
      setCopiedMutationIndex(index)
      setTimeout(() => setCopiedMutationIndex(null), 1500)
    } catch {
      // ignore clipboard failures; the button simply stays unchanged
    }
  }

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
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <div role="tablist" aria-label="Golden case view" style={{ display: 'flex', gap: 4 }}>
            <button
              role="tab"
              aria-selected={rawMode}
              onClick={() => setSearchParams({})}
              style={{
                padding: '6px 12px',
                cursor: 'pointer',
                background: rawMode ? '#e3f2fd' : '#fff',
                border: '1px solid #ccc',
                borderRadius: 4,
                fontWeight: rawMode ? 600 : 400,
              }}
            >
              View raw
            </button>
            {hasDiff && (
              <button
                role="tab"
                aria-selected={!rawMode}
                onClick={() => setSearchParams({ tab: 'diff' })}
                style={{
                  padding: '6px 12px',
                  cursor: 'pointer',
                  background: !rawMode ? '#e3f2fd' : '#fff',
                  border: '1px solid #ccc',
                  borderRadius: 4,
                  fontWeight: !rawMode ? 600 : 400,
                }}
              >
                Diff
              </button>
            )}
          </div>
          <button
            onClick={handleMutate}
            disabled={mutationLoading}
            style={{
              padding: '6px 12px',
              cursor: 'pointer',
              background: '#fff',
              border: '1px solid #ccc',
              borderRadius: 4,
            }}
          >
            Mutate
          </button>
          <button
            onClick={() => refetchDiff()}
            style={{
              padding: '6px 12px',
              cursor: 'pointer',
              background: '#fff',
              border: '1px solid #ccc',
              borderRadius: 4,
            }}
          >
            Refresh
          </button>
        </div>
      </div>

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
            <h5 style={{ margin: 0 }}>Mutation Results</h5>
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
                    <th style={{ padding: 4 }}>Prompt</th>
                  </tr>
                </thead>
                <tbody>
                  {mutationResults.map((r, i) => (
                    <tr key={i} style={{ borderBottom: '1px solid #eee' }}>
                      <td style={{ padding: 4 }}>{r.mutation}</td>
                      <td style={{ padding: 4, color: r.survived ? '#d32f2f' : '#2e7d32' }}>
                        {r.survived ? '❌ Survived (test should have failed)' : '✅ Killed (expected)'}
                      </td>
                      <td style={{ padding: 4 }}>
                        <button
                          onClick={() => copyMutationPrompt(r, i)}
                          title="Copy an LLM prompt with the test, mutation, failure reason, files, and referenced functions"
                          style={{
                            padding: '2px 8px',
                            cursor: 'pointer',
                            background: r.survived ? '#fff' : '#f8f8f8',
                            border: '1px solid #ccc',
                            borderRadius: 4,
                            fontSize: 11,
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {copiedMutationIndex === i ? 'Copied ✓' : 'Copy fix prompt'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

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
                          : ' · no referenced functions'
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
                blockRef={selectedFunc ? (el) => setFuncRef(selectedFunc.id, el) : undefined}
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
            {symbolLanguage && (
              <span style={{ color: '#777', fontWeight: 400, fontSize: 12 }}>
                {languageLabel(symbolLanguage)}
              </span>
            )}
            {languageSymbols.length > 0 && (
              <span style={{ color: '#777', fontWeight: 400, fontSize: 12 }}>
                {languageSymbols.length} total
              </span>
            )}
          </div>
          <div style={{ maxHeight: 400, overflow: 'auto' }}>
            <input
              type="text"
              placeholder="Filter symbols..."
              value={symbolFilter}
              onChange={(e) => setSymbolFilter(e.target.value)}
              style={{
                width: '100%',
                boxSizing: 'border-box',
                padding: '6px 12px',
                border: 'none',
                borderBottom: '1px solid #eee',
                outline: 'none',
                fontSize: 13,
                position: 'sticky',
                top: 0,
                background: '#fff',
                zIndex: 1,
              }}
            />
            {symbolsLoading ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12 }}>Loading symbols...</pre>
            ) : symbolsError ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12, color: '#dc2626' }}>
                Error loading symbols: {symbolsError.message}
              </pre>
            ) : filteredLanguageSymbols.length > 0 ? (
              filteredLanguageSymbols.map((sym) => {
                const isSelected = selectedFuncId === sym.id
                return (
                  <div
                    key={sym.id}
                    onClick={() => setSelectedFuncId(sym.id)}
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
            ) : languageSymbols.length > 0 ? (
              <pre style={{ padding: 12, margin: 0, fontSize: 12, color: '#777' }}>
                No symbols match the filter.
              </pre>
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
