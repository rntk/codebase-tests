import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { vi } from 'vitest'
import { getGoldenCase, getGoldenDiff, getTest, listSymbols } from '../api/client.ts'
import { DiffPage } from '../pages/DiffPage.tsx'
import * as fs from 'fs'
import * as path from 'path'

vi.mock('../api/client.ts', () => ({
  getGoldenDiff: vi.fn(),
  getGoldenCase: vi.fn(),
  getTest: vi.fn(),
  listSymbols: vi.fn(),
}))

const goldenDir = path.resolve(__dirname, '../../../tests/golden/web/src/test/DiffPage/DiffPage > loads current golden content for raw mode')

const caseFiles = fs
  .readdirSync(goldenDir)
  .filter((f) => f.endsWith('.in.json'))
  .map((f) => f.replace('.in.json', ''))

describe('DiffPage', () => {
  beforeEach(() => {
    vi.mocked(getGoldenDiff).mockReset()
    vi.mocked(getGoldenCase).mockReset()
    vi.mocked(getTest).mockReset()
    vi.mocked(listSymbols).mockReset()
  })

  describe('loads current golden content for raw mode', () => {
    for (const caseName of caseFiles) {
      it(caseName, async () => {
        const inp = JSON.parse(
          fs.readFileSync(path.join(goldenDir, `${caseName}.in.json`), 'utf-8')
        )
        const exp = JSON.parse(
          fs.readFileSync(path.join(goldenDir, `${caseName}.out.json`), 'utf-8')
        )

        vi.mocked(getGoldenDiff).mockResolvedValue(inp.goldenDiff)
        vi.mocked(getGoldenCase).mockResolvedValue(inp.goldenCase)
        vi.mocked(getTest).mockResolvedValue(inp.test)
        vi.mocked(listSymbols).mockResolvedValue(inp.symbols)

        const route = `/projects/${encodeURIComponent(inp.projectId)}/tests/${encodeURIComponent(inp.testId)}/golden/${encodeURIComponent(inp.caseId)}/diff`

        render(
          <MemoryRouter initialEntries={[route]}>
            <Routes>
              <Route path="/projects/:projectId/tests/:testId/golden/:caseId/diff" element={<DiffPage />} />
            </Routes>
          </MemoryRouter>
        )

        await screen.findByText('Diff: positive')
        expect(screen.getByRole('tab', { name: 'View raw' })).toHaveAttribute('aria-selected', exp.tabs['View raw']['aria-selected'])
        expect(screen.getByRole('tab', { name: 'Diff' })).toBeInTheDocument()

        await waitFor(() => {
          expect(getGoldenCase).toHaveBeenCalledWith(
            'default',
            'go:calc/add_test.go:calc.TestAdd',
            'go:calc/add_test.go:calc.TestAdd:positive'
          )
          expect(getTest).toHaveBeenCalledWith('default', 'go:calc/add_test.go:calc.TestAdd')
          expect(listSymbols).toHaveBeenCalledWith('default', true)
        })

        for (const text of exp.domContains) {
          await screen.findByText((content: string) => content.includes(text))
        }

        for (const interaction of inp.interactions) {
          if (interaction.type === 'click') {
            fireEvent.click(screen.getByTitle(interaction.value))
          }
        }

        await waitFor(() => {
          for (const text of exp.domNotContains) {
            expect(screen.queryByText((content: string) => content.includes(text))).not.toBeInTheDocument()
          }
        })

        for (const text of (exp.domPresentAfterInteraction || [])) {
          await screen.findByText((content: string) => content.includes(text))
        }
      })
    }
  })
})
