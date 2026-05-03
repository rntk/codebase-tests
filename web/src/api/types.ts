// Generated from OpenAPI spec. Do not edit manually.

export interface Project {
  id: string;
  path: string;
  name: string;
  plugins: PluginSettings[];
  goldenRoot: string;
  toolTimeoutSeconds: number;
}

export interface PluginSettings {
  name: string;
  language: string;
  lspServerAddress: string;
}

export interface FileNode {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileNode[];
}

export interface Symbol {
  id: string;
  name: string;
  qualifiedName: string;
  kind: string;
  file: string;
  line: number;
  column: number;
  package: string;
  covered: boolean;
  coveredBy?: string[];
  sourceCode?: string;
}

export interface TestFunc {
  id: string;
  name: string;
  file: string;
  line: number;
  column: number;
  package: string;
  subCases?: TestCase[];
  coveredFuncs?: string[];
  sourceCode?: string;
  coveredFuncSources?: SourceSnippet[];
  goldenCases?: GoldenCase[];
}

export interface SourceSnippet {
  id: string;
  name: string;
  qualifiedName: string;
  kind: string;
  file: string;
  line: number;
  column: number;
  package: string;
  sourceCode?: string;
}

export interface TestCase {
  id: string;
  name: string;
  casePath: string;
}

export interface GoldenCase {
  id: string;
  name: string;
  inPath: string;
  outPath: string;
  metaPath: string;
  inExists: boolean;
  outExists: boolean;
  meta?: Record<string, unknown> | null;
}

export interface GoldenCaseContent {
  id: string;
  name: string;
  in: unknown;
  out: unknown;
  meta?: Record<string, unknown> | null;
  inExists?: boolean;
  outExists?: boolean;
}

export interface GoldenCaseDiff {
  id: string;
  name: string;
  inDiff?: DiffNode;
  outDiff?: DiffNode;
}

export interface DiffNode {
  kind: 'added' | 'removed' | 'changed' | 'unchanged';
  key?: string | null;
  oldValue?: unknown;
  newValue?: unknown;
  children?: DiffNode[];
}

export interface CoverageReport {
  scope: 'run' | 'test';
  percentage: number;
  files?: FileCoverage[];
  uncovered?: string[];
}

export interface FileCoverage {
  path: string;
  percentage: number;
  lines?: LineRange[];
}

export interface LineRange {
  start: number;
  end: number;
  hit: boolean;
}

export interface TestPrompt {
  kind: 'create' | 'transform' | 'supported';
  language: string;
  qualified: string;
  file: string;
  line: number;
  testFile?: string;
  goldenDir: string;
  prompt: string;
  note?: string;
}
