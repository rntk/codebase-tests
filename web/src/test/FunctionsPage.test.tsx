import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'
import { getSymbolTestPrompt, listMutators, listSymbols, runMutationTesting } from '../api/client.ts'
import { FunctionsPage } from '../pages/FunctionsPage.tsx'

vi.mock('../api/client.ts', () => ({
  getSymbolTestPrompt: vi.fn(),
  listMutators: vi.fn(),
  listSymbols: vi.fn(),
  runMutationTesting: vi.fn(),
}))

describe('FunctionsPage', () => {
  beforeEach(() => {
    vi.mocked(getSymbolTestPrompt).mockReset()
    vi.mocked(listMutators).mockReset()
    vi.mocked(listSymbols).mockReset()
    vi.mocked(runMutationTesting).mockReset()
  })

  it('copies an LLM fix prompt for a failed mutation run mutant', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.assign(navigator, {
      clipboard: { writeText },
    })

    vi.mocked(listSymbols).mockResolvedValue([
      {
        id: 'go:calc/add.go:calc.Add',
        name: 'Add',
        qualifiedName: 'calc.Add',
        kind: 'function',
        file: 'calc/add.go',
        line: 1,
        column: 1,
        package: 'calc',
        covered: true,
        coveredBy: ['go:calc/add_test.go:calc.TestAdd'],
        sourceCode: 'func Add(a, b int) int {\n  return a + b\n}',
      },
    ])
    vi.mocked(listMutators).mockResolvedValue([
      {
        id: 'go-mutesting',
        name: 'Go mutesting',
        description: 'Run Go mutations',
        language: 'go',
      },
    ])
    vi.mocked(runMutationTesting).mockResolvedValue({
      tool: 'go-mutesting',
      score: 0.5,
      total: 1,
      killed: 0,
      survived: 1,
      noCoverage: 0,
      timedOut: 0,
      errored: 0,
      duration: 1.2,
      output: 'PASS calc.TestAdd',
      mutants: [
        {
          file: 'calc/add.go',
          line: 2,
          column: 10,
          operator: 'ArithmeticOperatorReplacement',
          status: 'survived',
          original: '+',
          replacement: '-',
        },
      ],
    })

    render(
      <MemoryRouter initialEntries={['/projects/default/functions/go%3Acalc%2Fadd.go%3Acalc.Add']}>
        <Routes>
          <Route path="/projects/:projectId/functions/:symbolId" element={<FunctionsPage />} />
        </Routes>
      </MemoryRouter>
    )

    await screen.findByText('calc.Add')
    fireEvent.click(screen.getByRole('button', { name: 'Load' }))
    await screen.findByRole('button', { name: 'Go mutesting' })
    fireEvent.click(screen.getByRole('button', { name: 'Go mutesting' }))
    await screen.findByText('survived')
    fireEvent.click(screen.getByRole('button', { name: 'Copy fix prompt' }))

    await waitFor(() => expect(writeText).toHaveBeenCalled())
    const prompt = writeText.mock.calls[0][0]
    expect(prompt).toContain('Function: calc.Add')
    expect(prompt).toContain('Mutant status: SURVIVED')
    expect(prompt).toContain('Operator: ArithmeticOperatorReplacement')
    expect(prompt).toContain('go:calc/add_test.go:calc.TestAdd')
    expect(prompt).toContain('PASS calc.TestAdd')
    expect(prompt).toContain('func Add(a, b int) int')
  })
})
