import { getTestTestPrompt, listSymbols } from '../api/client.ts'

describe('api client', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('double-encodes path IDs so slash-bearing test IDs stay in one server route segment', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => ({
        kind: 'transform',
        language: 'go',
        qualified: 'calc.TestAdd',
        file: 'calc/add_test.go',
        line: 1,
        goldenDir: 'tests/golden/calc/TestAdd',
        prompt: 'prompt',
      }),
    } as Response)

    await getTestTestPrompt('default', 'go:calc/add_test.go:calc.TestAdd')

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/projects/default/tests/go%253Acalc%252Fadd_test.go%253Acalc.TestAdd/test-prompt',
      undefined
    )
  })

  it('passes symbol language and source flags as query parameters', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue({
      ok: true,
      json: async () => [],
    } as unknown as Response)

    await listSymbols('default', true, 'go')

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/projects/default/symbols?withSource=true&language=go',
      undefined
    )
  })
})
