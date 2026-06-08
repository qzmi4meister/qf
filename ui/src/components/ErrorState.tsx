import { IconAlertTriangle } from '@tabler/icons-react'

interface ErrorStateProps {
  title: string
  body?: string
  onRetry?: () => void
}

export default function ErrorState({ title, body, onRetry }: ErrorStateProps) {
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', alignItems: 'center',
      justifyContent: 'center', padding: '60px 24px', gap: 12, textAlign: 'center',
    }}>
      <IconAlertTriangle size={48} style={{ color: 'var(--qf-bad-fg)' }} />
      <div style={{ fontSize: 'var(--qf-t-lg)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>
        {title}
      </div>
      {body && (
        <div style={{
          fontSize: 'var(--qf-t-sm)', color: 'var(--qf-bad-fg)',
          fontFamily: 'var(--qf-mono)', maxWidth: 480, lineHeight: 1.5,
        }}>
          {body}
        </div>
      )}
      {onRetry && (
        <button
          onClick={onRetry}
          style={{
            marginTop: 8, padding: '8px 16px',
            background: 'var(--qf-bg-muted)', border: '1px solid var(--qf-border-1)',
            borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-2)',
            fontSize: 'var(--qf-t-base)', fontFamily: 'inherit', cursor: 'pointer',
          }}
        >
          Try again
        </button>
      )}
    </div>
  )
}
