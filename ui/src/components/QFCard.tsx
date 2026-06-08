import type { CSSProperties, ReactNode } from 'react'

interface QFCardProps {
  children: ReactNode
  pad?: false | number
  style?: CSSProperties
}

export default function QFCard({ children, pad, style }: QFCardProps) {
  const padding = pad === false ? 0 : (pad ?? 16)
  return (
    <div style={{
      background: 'var(--qf-bg-surface)',
      border: '1px solid var(--qf-border-1)',
      borderRadius: 'var(--qf-r-lg)',
      padding,
      overflow: pad === false ? 'hidden' : undefined,
      ...style,
    }}>
      {children}
    </div>
  )
}
