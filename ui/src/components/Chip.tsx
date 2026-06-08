interface ChipProps {
  k: string
  v: string
}

export default function Chip({ k, v }: ChipProps) {
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center',
      border: '1px solid var(--qf-border-1)',
      borderRadius: 'var(--qf-r-xs)',
      fontFamily: 'var(--qf-mono)',
      fontSize: 'var(--qf-t-xs)',
      overflow: 'hidden',
      whiteSpace: 'nowrap',
    }}>
      <span style={{
        padding: '2px 6px',
        background: 'var(--qf-bg-muted)',
        color: 'var(--qf-fg-mute)',
        borderRight: '1px solid var(--qf-border-1)',
      }}>
        {k}
      </span>
      <span style={{
        padding: '2px 6px',
        background: 'transparent',
        color: 'var(--qf-fg-2)',
      }}>
        {v}
      </span>
    </span>
  )
}
