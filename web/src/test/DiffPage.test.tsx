import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'
import { getGoldenCase, getGoldenDiff, getTest, listSymbols } from '../api/client.ts'
import { DiffPage } from '../pages/DiffPage.tsx'

vi.mock('../api/client.ts', () => ({
  getGoldenDiff: vi.fn(),
  getGoldenCase: vi.fn(),
  getTest: vi.fn(),
  listSymbols: vi.fn(),
}))

describe('DiffPage', () => {
  beforeEach(() => {
    vi.mocked(getGoldenDiff).mockReset()
    vi.mocked(getGoldenCase).mockReset()
    vi.mocked(getTest).mockReset()
    vi.mocked(listSymbols).mockReset()
  })

  it('loads current golden content for raw mode', async () => {
    vi.mocked(getGoldenDiff).mockResolvedValue({
      id: 'go:calc/add_test.go:calc.TestAdd:positive',
      name: 'positive',
      inDiff: {
        kind: 'changed',
        children: [{ kind: 'changed', key: 'b', oldValue: 2, newValue: 3 }],
      },
      outDiff: {
        kind: 'changed',
        children: [{ kind: 'changed', key: 'result', oldValue: 3, newValue: 4 }],
      },
    })
    vi.mocked(getGoldenCase).mockResolvedValue({
      id: 'go:calc/add_test.go:calc.TestAdd:positive',
      name: 'positive',
      in: { a: 1, b: 2 },
      out: { result: 3 },
    })
    vi.mocked(getTest).mockResolvedValue({
      id: 'go:calc/add_test.go:calc.TestAdd',
      name: 'TestAdd',
      file: 'calc/add_test.go',
      line: 10,
      column: 1,
      package: 'calc',
      coveredFuncs: ['go:calc/add.go:calc.Add'],
      sourceCode: 'func TestAdd(t *testing.T) {\n\tgot := Add(1, 2)\n}',
      coveredFuncSources: [
        {
          id: 'go:calc/add.go:calc.Add',
          name: 'Add',
          qualifiedName: 'calc.Add',
          kind: 'function',
          file: 'calc/add.go',
          line: 1,
          column: 1,
          package: 'calc',
          sourceCode: 'func Add(a, b int) int {\n\treturn a + b\n}',
        },
      ],
    })
    vi.mocked(listSymbols).mockResolvedValue([])

    render(
      <MemoryRouter
        initialEntries={[
          `/projects/default/tests/${encodeURIComponent('go:calc/add_test.go:calc.TestAdd')}/golden/${encodeURIComponent(
            'go:calc/add_test.go:calc.TestAdd:positive'
          )}/diff`,
        ]}
      >
        <Routes>
          <Route path="/projects/:projectId/tests/:testId/golden/:caseId/diff" element={<DiffPage />} />
        </Routes>
      </MemoryRouter>
    )

    await screen.findByText('Diff: positive')
    expect(screen.getByRole('tab', { name: 'View raw' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Diff' })).toBeInTheDocument()

    await waitFor(() => {
      expect(getGoldenCase).toHaveBeenCalledWith(
        'default',
        'go:calc/add_test.go:calc.TestAdd',
        'go:calc/add_test.go:calc.TestAdd:positive'
      )
      expect(getTest).toHaveBeenCalledWith('default', 'go:calc/add_test.go:calc.TestAdd')
      expect(listSymbols).toHaveBeenCalledWith('default', true)
      expect(screen.getByText((text) => text.includes('"a": 1'))).toBeInTheDocument()
      expect(screen.getByText((text) => text.includes('"result": 3'))).toBeInTheDocument()
      expect(screen.getByText('Test Code: TestAdd')).toBeInTheDocument()
      expect(screen.getByText((text) => text.includes('func TestAdd'))).toBeInTheDocument()
    })
    fireEvent.click(screen.getByTitle('View code: calc.Add'))
    expect(screen.getByText('Function Code: calc.Add')).toBeInTheDocument()
    expect(screen.getByText((text) => text.includes('func Add'))).toBeInTheDocument()
    expect(screen.queryByText((text) => text.includes('"oldValue"'))).not.toBeInTheDocument()
  })
})
