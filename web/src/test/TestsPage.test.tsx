import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'
import { getGoldenDiff, getTestTestPrompt, listGoldenCases, listTests } from '../api/client.ts'
import { TestsPage } from '../pages/TestsPage.tsx'

vi.mock('../api/client.ts', () => ({
  listTests: vi.fn(),
  listGoldenCases: vi.fn(),
  getGoldenDiff: vi.fn(),
  getTestTestPrompt: vi.fn(),
}))

describe('TestsPage', () => {
  beforeEach(() => {
    vi.mocked(listTests).mockReset()
    vi.mocked(listGoldenCases).mockReset()
    vi.mocked(getGoldenDiff).mockReset()
    vi.mocked(getTestTestPrompt).mockReset()
  })

  it('links changed golden case markers to the diff tab', async () => {
    vi.mocked(listTests).mockResolvedValue([
      {
        id: 'test-1',
        name: 'TestOne',
        file: 'calc/add_test.go',
        line: 10,
        column: 1,
        package: 'calc',
        hasGolden: true,
      },
    ])
    vi.mocked(listGoldenCases).mockResolvedValue([
      {
        id: 'case-changed',
        name: 'changed',
        inPath: 'testdata/changed.in.json',
        outPath: 'testdata/changed.out.json',
        metaPath: 'testdata/changed.meta.json',
        inExists: true,
        outExists: true,
      },
      {
        id: 'case-stable',
        name: 'stable',
        inPath: 'testdata/stable.in.json',
        outPath: 'testdata/stable.out.json',
        metaPath: 'testdata/stable.meta.json',
        inExists: true,
        outExists: true,
      },
    ])
    vi.mocked(getGoldenDiff).mockImplementation((_projectId, _testId, caseId) =>
      Promise.resolve(
        caseId === 'case-changed'
          ? {
              id: caseId,
              name: 'changed',
              inDiff: { kind: 'changed', key: 'value', oldValue: 1, newValue: 2 },
            }
          : {
              id: caseId,
              name: 'stable',
              inDiff: { kind: 'unchanged', key: 'value', oldValue: 1, newValue: 1 },
            }
      )
    )

    render(
      <MemoryRouter initialEntries={['/projects/default/tests/test-1']}>
        <Routes>
          <Route path="/projects/:projectId/tests/:testId" element={<TestsPage />} />
        </Routes>
      </MemoryRouter>
    )

    const marker = await screen.findByRole('link', { name: 'changed has changes' })
    expect(marker).toHaveAttribute(
      'href',
      '/projects/default/tests/test-1/golden/case-changed/diff?tab=diff'
    )
    expect(screen.queryByRole('link', { name: 'stable has changes' })).not.toBeInTheDocument()
  })
})
