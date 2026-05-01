import type {
  Project,
  FileNode,
  Symbol,
  TestFunc,
  GoldenCase,
  GoldenCaseContent,
  GoldenCaseDiff,
  CoverageReport,
  TestPrompt,
} from './types.ts'
import { getMockResponse } from './mockData.ts'

const API_BASE = (import.meta.env.VITE_API_BASE_URL as string) || ''
const USE_MOCK = (import.meta.env.VITE_MOCK_API as string) === 'true'

function pathSegment(value: string): string {
  return encodeURIComponent(encodeURIComponent(value))
}

export class ApiError extends Error {
  constructor(
    message: string,
    public status: number
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

async function apiFetch<T>(path: string): Promise<T> {
  if (USE_MOCK) {
    const mock = getMockResponse<T>(path)
    if (mock !== undefined) {
      return Promise.resolve(mock)
    }
    throw new ApiError('Not found in mock data', 404)
  }

  const res = await fetch(`${API_BASE}${path}`)
  if (!res.ok) {
    if (res.status === 501) {
      throw new ApiError('This feature is not implemented yet.', 501)
    }
    const text = await res.text().catch(() => res.statusText)
    throw new ApiError(`HTTP ${res.status}: ${text}`, res.status)
  }
  return res.json() as Promise<T>
}

export function listProjects(): Promise<Project[]> {
  return apiFetch('/api/projects')
}

export function getProject(id: string): Promise<Project> {
  return apiFetch(`/api/projects/${pathSegment(id)}`)
}

export function listFiles(projectId: string): Promise<FileNode[]> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/files`)
}

export function listSymbols(projectId: string): Promise<Symbol[]> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/symbols`)
}

export function listTests(projectId: string): Promise<TestFunc[]> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/tests`)
}

export function getTest(projectId: string, testId: string): Promise<TestFunc> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/tests/${pathSegment(testId)}`)
}

export function listGoldenCases(projectId: string, testId: string): Promise<GoldenCase[]> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/tests/${pathSegment(testId)}/golden`)
}

export function getGoldenCase(projectId: string, testId: string, caseId: string): Promise<GoldenCaseContent> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/tests/${pathSegment(testId)}/golden/${pathSegment(caseId)}`)
}

export function getGoldenDiff(projectId: string, testId: string, caseId: string): Promise<GoldenCaseDiff> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/tests/${pathSegment(testId)}/golden/${pathSegment(caseId)}/diff`)
}

export function getCoverage(projectId: string): Promise<CoverageReport> {
  return apiFetch(`/api/projects/${pathSegment(projectId)}/coverage`)
}

export function getSymbolTestPrompt(projectId: string, symbolId: string): Promise<TestPrompt> {
  return apiFetch(
    `/api/projects/${pathSegment(projectId)}/symbols/${pathSegment(symbolId)}/test-prompt`
  )
}

export function getTestTestPrompt(projectId: string, testId: string): Promise<TestPrompt> {
  return apiFetch(
    `/api/projects/${pathSegment(projectId)}/tests/${pathSegment(testId)}/test-prompt`
  )
}
