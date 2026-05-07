// @ts-nocheck
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import { getCoverage, getProject, listFiles } from '../api/client.ts'
import App from '../App.tsx'

vi.mock('../api/client.ts', () => ({
  getCoverage: vi.fn(),
  getProject: vi.fn(),
  listFiles: vi.fn(),
}))

describe('App', () => {
  beforeEach(() => {
    vi.mocked(getCoverage).mockReset()
    vi.mocked(getProject).mockReset()
    vi.mocked(listFiles).mockReset()
  })

  it('loads coverage asynchronously and shows total coverage per language in the header', async () => {
    vi.mocked(getProject).mockResolvedValue({
      id: 'default',
      path: '/workspace',
      name: 'Workspace',
      plugins: [
        { name: 'go', language: 'go', lspServerAddress: 'gopls' },
        { name: 'python', language: 'python', lspServerAddress: 'pylsp' },
      ],
      goldenRoot: 'tests/golden',
      toolTimeoutSeconds: 30,
    })
    vi.mocked(getCoverage).mockResolvedValue({
      scope: 'run',
      percentage: 80,
      files: [
        {
          path: 'calc/add.go',
          percentage: 50,
          lines: [
            { start: 1, end: 1, hit: true },
            { start: 2, end: 2, hit: false },
          ],
        },
        { path: 'calc/add.py', percentage: 72 },
      ],
    })
    vi.mocked(listFiles).mockResolvedValue([])

    render(
      <MemoryRouter initialEntries={['/projects/default/files']}>
        <App />
      </MemoryRouter>
    )

    expect(getCoverage).toHaveBeenCalledWith('default')
    await screen.findByText('Workspace')
    expect(await screen.findByText(/50% coverage/)).toBeInTheDocument()
    expect(screen.getByText(/72% coverage/)).toBeInTheDocument()
  })
})
