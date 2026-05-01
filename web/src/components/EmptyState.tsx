export function EmptyState({ message = 'Nothing to show.' }: { message?: string }) {
  return (
    <div style={{ padding: 16, color: '#888' }} data-testid="empty-state">
      {message}
    </div>
  )
}
