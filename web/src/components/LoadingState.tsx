export function LoadingState({ message = 'Loading…' }: { message?: string }) {
  return (
    <div style={{ padding: 16, color: '#666' }} data-testid="loading-state">
      {message}
    </div>
  )
}
