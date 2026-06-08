import type { ReactNode } from 'react'

interface PageHeadProps {
  title: string
  sub?: string
  actions?: ReactNode
}

export default function PageHead({ title, sub, actions }: PageHeadProps) {
  return (
    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 20 }}>
      <div>
        <h1 style={{
          margin: 0,
          fontSize: 'var(--qf-t-xl)', fontWeight: 700,
          color: 'var(--qf-fg-1)', letterSpacing: '-0.02em',
          lineHeight: 1.2,
        }}>
          {title}
        </h1>
        {sub && (
          <div style={{
            fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)',
            fontFamily: 'var(--qf-mono)', marginTop: 4,
          }}>
            {sub}
          </div>
        )}
      </div>
      {actions && (
        <div style={{ display: 'flex', gap: 8, flexShrink: 0 }}>
          {actions}
        </div>
      )}
    </div>
  )
}
