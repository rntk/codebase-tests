import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'
import { getGoldenCase, getGoldenDiff } from '../api/client.ts'
import { DiffPage } from '../pages/DiffPage.tsx'

vi.mock('../api/client.ts', () => ({
  getGoldenDiff: vi.fn(),
  getGoldenCase: vi.fn(),
}))

describe('DiffPage', () => {
  beforeEach(() => {
    vi.mocked(getGoldenDiff).mockReset()
    vi.mocked(getGoldenCase).mockReset()
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
    fireEvent.click(screen.getByLabelText('View raw'))

    await waitFor(() => {
      expect(getGoldenCase).toHaveBeenCalledWith(
        'default',
        'go:calc/add_test.go:calc.TestAdd',
        'go:calc/add_test.go:calc.TestAdd:positive'
      )
      expect(screen.getByText((text) => text.includes('"a": 1'))).toBeInTheDocument()
      expect(screen.getByText((text) => text.includes('"result": 3'))).toBeInTheDocument()
    })
    expect(screen.queryByText((text) => text.includes('"oldValue"'))).not.toBeInTheDocument()
  })
})
