import type {
  Project,
  FileNode,
  Symbol,
  TestFunc,
  GoldenCase,
  GoldenCaseContent,
  GoldenCaseDiff,
  CoverageReport,
} from './types.ts'

const project: Project = {
  id: 'default',
  name: 'Sample Project',
  path: '/tmp/sample',
  plugins: [{ name: 'go', language: 'go', lspServerAddress: 'gopls' }],
  goldenRoot: 'testdata',
  toolTimeoutSeconds: 30,
}

const files: FileNode[] = [
  {
    name: 'calc',
    path: 'calc',
    isDir: true,
    children: [
      { name: 'add.go', path: 'calc/add.go', isDir: false },
      { name: 'sub.go', path: 'calc/sub.go', isDir: false },
      { name: 'mul.go', path: 'calc/mul.go', isDir: false },
      { name: 'add_test.go', path: 'calc/add_test.go', isDir: false },
      { name: 'sub_test.go', path: 'calc/sub_test.go', isDir: false },
    ],
  },
]

const symbols: Symbol[] = [
  {
    id: 'go:calc/add.go:calc.Add:1:1',
    name: 'Add',
    qualifiedName: 'calc.Add',
    kind: 'function',
    file: 'calc/add.go',
    line: 1,
    column: 1,
    package: 'calc',
    covered: true,
    coveredBy: ['go:calc/add_test.go:calc.TestAdd'],
  },
  {
    id: 'go:calc/sub.go:calc.Sub:1:1',
    name: 'Sub',
    qualifiedName: 'calc.Sub',
    kind: 'function',
    file: 'calc/sub.go',
    line: 1,
    column: 1,
    package: 'calc',
    covered: true,
    coveredBy: ['go:calc/sub_test.go:calc.TestSub'],
  },
  {
    id: 'go:calc/mul.go:calc.Mul:1:1',
    name: 'Mul',
    qualifiedName: 'calc.Mul',
    kind: 'function',
    file: 'calc/mul.go',
    line: 1,
    column: 1,
    package: 'calc',
    covered: false,
  },
]

const tests: TestFunc[] = [
  {
    id: 'go:calc/add_test.go:calc.TestAdd',
    name: 'TestAdd',
    file: 'calc/add_test.go',
    line: 1,
    column: 1,
    package: 'calc',
    subCases: [
      { id: 'go:calc/add_test.go:calc.TestAdd:positive', name: 'positive', casePath: 'testdata/calc/TestAdd/positive' },
      { id: 'go:calc/add_test.go:calc.TestAdd:negative', name: 'negative', casePath: 'testdata/calc/TestAdd/negative' },
    ],
    coveredFuncs: ['go:calc/add.go:calc.Add:1:1'],
  },
  {
    id: 'go:calc/sub_test.go:calc.TestSub',
    name: 'TestSub',
    file: 'calc/sub_test.go',
    line: 1,
    column: 1,
    package: 'calc',
    subCases: [],
    coveredFuncs: ['go:calc/sub.go:calc.Sub:1:1'],
  },
]

const coverage: CoverageReport = {
  scope: 'run',
  percentage: 66.7,
  files: [
    {
      path: 'calc/add.go',
      percentage: 100,
      lines: [{ start: 1, end: 3, hit: true }],
    },
    {
      path: 'calc/mul.go',
      percentage: 0,
      lines: [{ start: 1, end: 3, hit: false }],
    },
  ],
  uncovered: ['go:calc/mul.go:calc.Mul:1:1'],
}

const goldenCases: Record<string, GoldenCase[]> = {
  'go:calc/add_test.go:calc.TestAdd': [
    {
      id: 'go:calc/add_test.go:calc.TestAdd:positive',
      name: 'positive',
      inPath: 'testdata/calc/TestAdd/positive.in.json',
      outPath: 'testdata/calc/TestAdd/positive.out.json',
      metaPath: 'testdata/calc/TestAdd/positive.meta.json',
      inExists: true,
      outExists: true,
      meta: { duration_ms: 12 },
    },
    {
      id: 'go:calc/add_test.go:calc.TestAdd:negative',
      name: 'negative',
      inPath: 'testdata/calc/TestAdd/negative.in.json',
      outPath: 'testdata/calc/TestAdd/negative.out.json',
      metaPath: 'testdata/calc/TestAdd/negative.meta.json',
      inExists: true,
      outExists: true,
    },
  ],
}

const goldenContents: Record<string, GoldenCaseContent> = {
  'go:calc/add_test.go:calc.TestAdd:positive': {
    id: 'go:calc/add_test.go:calc.TestAdd:positive',
    name: 'positive',
    in: { a: 1, b: 2 },
    out: { result: 3 },
    meta: { duration_ms: 12 },
  },
  'go:calc/add_test.go:calc.TestAdd:negative': {
    id: 'go:calc/add_test.go:calc.TestAdd:negative',
    name: 'negative',
    in: { a: -1, b: -2 },
    out: { result: -3 },
  },
}

const goldenDiffs: Record<string, GoldenCaseDiff> = {
  'go:calc/add_test.go:calc.TestAdd:positive': {
    id: 'go:calc/add_test.go:calc.TestAdd:positive',
    name: 'positive',
    inDiff: {
      kind: 'changed',
      children: [
        { kind: 'unchanged', key: 'a', oldValue: 1, newValue: 1 },
        { kind: 'changed', key: 'b', oldValue: 2, newValue: 3 },
      ],
    },
    outDiff: {
      kind: 'changed',
      children: [
        { kind: 'changed', key: 'result', oldValue: 3, newValue: 4 },
      ],
    },
  },
  'go:calc/add_test.go:calc.TestAdd:negative': {
    id: 'go:calc/add_test.go:calc.TestAdd:negative',
    name: 'negative',
    inDiff: {
      kind: 'added',
      oldValue: undefined,
      newValue: { a: -1, b: -2 },
    },
    outDiff: {
      kind: 'added',
      oldValue: undefined,
      newValue: { result: -3 },
    },
  },
}

function goldenLookupKey(testId: string, caseId: string): string {
  if (caseId.startsWith(`${testId}:`)) return caseId
  return `${testId}:${caseId}`
}

export function getMockResponse<T>(path: string): T | undefined {
  if (path === '/api/projects') return [project] as T
  if (path === `/api/projects/${project.id}`) return project as T
  if (path === `/api/projects/${project.id}/files`) return files as T
  if (path === `/api/projects/${project.id}/symbols`) return symbols as T
  if (path === `/api/projects/${project.id}/tests`) return tests as T
  if (path === `/api/projects/${project.id}/coverage`) return coverage as T

  // Check more specific golden routes first
  const goldenDiffMatch = path.match(
    new RegExp(`/api/projects/${project.id}/tests/(.+)/golden/([^/]+)/diff$`)
  )
  if (goldenDiffMatch) {
    const key = goldenLookupKey(decodeURIComponent(goldenDiffMatch[1]), decodeURIComponent(goldenDiffMatch[2]))
    const diff = goldenDiffs[key]
    if (diff) return diff as T
  }

  const goldenContentMatch = path.match(
    new RegExp(`/api/projects/${project.id}/tests/(.+)/golden/([^/]+)$`)
  )
  if (goldenContentMatch) {
    const key = goldenLookupKey(decodeURIComponent(goldenContentMatch[1]), decodeURIComponent(goldenContentMatch[2]))
    const content = goldenContents[key]
    if (content) return content as T
  }

  const goldenListMatch = path.match(
    new RegExp(`/api/projects/${project.id}/tests/(.+)/golden$`)
  )
  if (goldenListMatch) {
    const testId = decodeURIComponent(goldenListMatch[1])
    const cases = goldenCases[testId]
    if (cases) return cases as T
  }

  // Test detail (must come after golden routes)
  const testMatch = path.match(
    new RegExp(`/api/projects/${project.id}/tests/(.+)$`)
  )
  if (testMatch) {
    const testId = decodeURIComponent(testMatch[1])
    const t = tests.find((x) => x.id === testId)
    if (t) return t as T
  }

  return undefined
}
