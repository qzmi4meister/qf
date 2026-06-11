import type { Tone } from './QFBadge'
import { TONE_VARS } from './QFBadge'

export type HostStatus = 'active' | 'enrolling' | 'stale' | 'needs_rebootstrap' | 'revoked' | string

const STATUS_MAP: Record<string, { tone: Tone; label: string }> = {
  active:            { tone: 'ok',      label: 'Active' },
  enrolling:         { tone: 'info',    label: 'Enrolling' },
  stale:             { tone: 'neutral', label: 'Stale' },
  needs_rebootstrap: { tone: 'bad',     label: 'Needs rebootstrap' },
  revoked:           { tone: 'term',    label: 'Revoked' },
}

interface StatusBadgeProps {
  status: string
}

export default function StatusBadge({ status }: StatusBadgeProps) {
  const mapped = STATUS_MAP[status] ?? { tone: 'neutral' as Tone, label: status }
  const t = TONE_VARS[mapped.tone]
  const isActive = mapped.tone === 'ok'

  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6,
      padding: '3px 9px',
      borderRadius: 'var(--qf-r-full)',
      fontSize: 'var(--qf-t-xs)', fontWeight: 600,
      background: t.bg, color: t.fg,
      whiteSpace: 'nowrap',
    }}>
      <span style={{
        width: 6, height: 6, borderRadius: '50%',
        background: t.solid, flexShrink: 0,
        boxShadow: isActive ? `0 0 0 2px ${t.bg}, 0 0 6px ${t.solid}` : 'none',
      }} />
      {mapped.label}
    </span>
  )
}
