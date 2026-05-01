import { useState } from 'react'
import type { TestPrompt } from '../api/types.ts'

interface Props {
  prompt: TestPrompt | null
  loading?: boolean
  error?: Error | null
  onClose: () => void
}

const titleByKind: Record<TestPrompt['kind'], string> = {
  create: 'Generate tests',
  transform: 'Convert tests to table + golden format',
  supported: 'Already in supported format',
}

export function PromptModal({ prompt, loading, error, onClose }: Props) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    if (!prompt?.prompt) return
    try {
      await navigator.clipboard.writeText(prompt.prompt)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // ignore
    }
  }

  return (
    <div
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.4)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
      }}
      onClick={onClose}
    >
      <div
        style={{
          background: '#fff',
          width: 'min(820px, 92vw)',
          maxHeight: '88vh',
          display: 'flex',
          flexDirection: 'column',
          borderRadius: 6,
          boxShadow: '0 8px 32px rgba(0,0,0,0.2)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div
          style={{
            padding: '12px 16px',
            borderBottom: '1px solid #eee',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <h3 style={{ margin: 0, fontSize: 16 }}>
            {prompt ? titleByKind[prompt.kind] : 'Test prompt'}
          </h3>
          <button onClick={onClose} style={{ border: 'none', background: 'none', fontSize: 18, cursor: 'pointer' }}>
            ×
          </button>
        </div>

        <div style={{ padding: 16, overflow: 'auto', flex: 1 }}>
          {loading && <div>Loading prompt…</div>}
          {error && <div style={{ color: '#c62828' }}>{error.message}</div>}

          {prompt && (
            <>
              <div style={{ fontSize: 13, color: '#555', marginBottom: 12 }}>
                <div>
                  <strong>Function:</strong> {prompt.qualified}
                </div>
                <div>
                  <strong>Language:</strong> {prompt.language}
                </div>
                <div>
                  <strong>Golden directory:</strong> <code>{prompt.goldenDir}</code>
                </div>
                {prompt.testFile && (
                  <div>
                    <strong>Test file:</strong> <code>{prompt.testFile}</code>
                  </div>
                )}
              </div>

              {prompt.kind === 'supported' ? (
                <div style={{ padding: 12, background: '#e8f5e9', borderRadius: 4, color: '#2e7d32' }}>
                  {prompt.note ?? 'Tests are already in the supported format.'}
                </div>
              ) : (
                <>
                  <div style={{ marginBottom: 8, color: '#555', fontSize: 13 }}>
                    Copy this prompt and paste it into your coding agent.
                  </div>
                  <pre
                    style={{
                      background: '#f6f8fa',
                      border: '1px solid #e1e4e8',
                      padding: 12,
                      borderRadius: 4,
                      whiteSpace: 'pre-wrap',
                      fontSize: 12,
                      lineHeight: 1.45,
                      margin: 0,
                    }}
                  >
                    {prompt.prompt}
                  </pre>
                </>
              )}
            </>
          )}
        </div>

        {prompt && prompt.kind !== 'supported' && (
          <div style={{ padding: 12, borderTop: '1px solid #eee', display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
            <button onClick={copy} style={{ padding: '6px 12px', cursor: 'pointer' }}>
              {copied ? 'Copied ✓' : 'Copy prompt'}
            </button>
            <button onClick={onClose} style={{ padding: '6px 12px', cursor: 'pointer' }}>
              Close
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
