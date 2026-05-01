import { ApiError } from '../api/client.ts'

export function ErrorState({ error }: { error: Error | null }) {
  const is501 = error instanceof ApiError && error.status === 501
  return (
    <div
      style={{
        padding: 16,
        color: is501 ? '#856404' : '#a94442',
        background: is501 ? '#fff3cd' : '#f2dede',
        borderRadius: 4,
      }}
      data-testid="error-state"
    >
      <strong>{is501 ? 'Not implemented yet' : 'Error'}</strong>
      <p>{error?.message ?? 'Something went wrong.'}</p>
    </div>
  )
}
