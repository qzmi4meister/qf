import type { ReactNode } from 'react'

interface EmptyStateProps {
  icon?: ReactNode
  title: string
  body?: string
  action?: ReactNode
}

export default function EmptyState({ icon, title, body, action }: EmptyStateProps) {
  return (
    <div style={{
      display: 'flex', flexDirection: 'column', alignItems: 'center',
      justifyContent: 'center', padding: '60px 24px', gap: 12, textAlign: 'center',
    }}>
      {icon && (
        <div style={{ color: 'var(--qf-fg-faint)', marginBottom: 4 }}>
          {icon}
        </div>
      )}
      <div style={{
        fontSize: 'var(--qf-t-lg)', fontWeight: 600,
        color: 'var(--qf-fg-1)',
      }}>
        {title}
      </div>
      {body && (
        <div style={{
          fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-mute)',
          maxWidth: 420, lineHeight: 1.55,
        }}>
          {body}
        </div>
      )}
      {action && <div style={{ marginTop: 8 }}>{action}</div>}
    </div>
  )
}
