import { getMockResponse } from '../api/mockData.ts'
import type { GoldenCaseContent, GoldenCaseDiff } from '../api/types.ts'

describe('mockData', () => {
  it('resolves golden content and diff with canonical full case IDs', () => {
    const testId = encodeURIComponent(encodeURIComponent('go:calc/add_test.go:calc.TestAdd'))
    const caseId = encodeURIComponent(encodeURIComponent('go:calc/add_test.go:calc.TestAdd:positive'))

    const content = getMockResponse<GoldenCaseContent>(`/api/projects/default/tests/${testId}/golden/${caseId}`)
    const diff = getMockResponse<GoldenCaseDiff>(`/api/projects/default/tests/${testId}/golden/${caseId}/diff`)

    expect(content?.in).toEqual({ a: 1, b: 2 })
    expect(diff?.inDiff?.kind).toBe('changed')
  })
})
