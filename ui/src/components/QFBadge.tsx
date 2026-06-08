import type { CSSProperties, ReactNode } from 'react'

export type Tone = 'ok' | 'bad' | 'warn' | 'info' | 'pol' | 'term' | 'neutral'

export const TONE_VARS: Record<Tone, { bg: string; fg: string; solid: string }> = {
  ok:      { bg: 'var(--qf-ok-bg)',      fg: 'var(--qf-ok-fg)',      solid: 'var(--qf-ok-solid)' },
  bad:     { bg: 'var(--qf-bad-bg)',     fg: 'var(--qf-bad-fg)',     solid: 'var(--qf-bad-solid)' },
  warn:    { bg: 'var(--qf-warn-bg)',    fg: 'var(--qf-warn-fg)',    solid: 'var(--qf-warn-solid)' },
  info:    { bg: 'var(--qf-info-bg)',    fg: 'var(--qf-info-fg)',    solid: 'var(--qf-info-solid)' },
  pol:     { bg: 'var(--qf-pol-bg)',     fg: 'var(--qf-pol-fg)',     solid: 'var(--qf-pol-solid)' },
  term:    { bg: 'var(--qf-term-bg)',    fg: 'var(--qf-term-fg)',    solid: 'var(--qf-term-solid)' },
  neutral: { bg: 'var(--qf-neutral-bg)', fg: 'var(--qf-neutral-fg)', solid: 'var(--qf-neutral-fg)' },
}

interface QFBadgeProps {
  tone?: Tone
  children: ReactNode
  style?: CSSProperties
}

export default function QFBadge({ tone = 'neutral', children, style }: QFBadgeProps) {
  const t = TONE_VARS[tone]
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center',
      padding: '2px 8px',
      borderRadius: 'var(--qf-r-full)',
      fontSize: 'var(--qf-t-xs)', fontWeight: 600,
      background: t.bg, color: t.fg,
      letterSpacing: '0.02em',
      whiteSpace: 'nowrap',
      ...style,
    }}>
      {children}
    </span>
  )
}
