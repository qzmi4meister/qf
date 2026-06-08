import type { CSSProperties, ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'

interface THProps extends ThHTMLAttributes<HTMLTableCellElement> {
  children?: ReactNode
  w?: number | string
  right?: boolean
}

export function TH({ children, w, right, style, ...rest }: THProps) {
  return (
    <th
      style={{
        padding: '0 16px 10px',
        textAlign: right ? 'right' : 'left',
        fontSize: 'var(--qf-t-xs)',
        fontWeight: 600,
        color: 'var(--qf-fg-mute)',
        textTransform: 'uppercase',
        letterSpacing: '0.05em',
        whiteSpace: 'nowrap',
        width: w,
        ...style,
      }}
      {...rest}
    >
      {children}
    </th>
  )
}

interface TDProps extends TdHTMLAttributes<HTMLTableCellElement> {
  children?: ReactNode
  mono?: boolean
  muted?: boolean
  right?: boolean
}

export function TD({ children, mono, muted, right, style, ...rest }: TDProps) {
  return (
    <td
      style={{
        padding: '0 16px',
        height: 'var(--qf-row-h)',
        fontSize: 'var(--qf-t-base)',
        color: muted ? 'var(--qf-fg-3)' : 'var(--qf-fg-2)',
        fontFamily: mono ? 'var(--qf-mono)' : 'inherit',
        textAlign: right ? 'right' : 'left',
        whiteSpace: 'nowrap',
        ...style,
      }}
      {...rest}
    >
      {children}
    </td>
  )
}

interface QFTableProps {
  children: ReactNode
  style?: CSSProperties
  minWidth?: number
}

export function QFTable({ children, style, minWidth = 700 }: QFTableProps) {
  return (
    <div style={{ overflow: 'auto' }}>
      <table style={{ width: '100%', borderCollapse: 'collapse', minWidth, ...style }}>
        {children}
      </table>
    </div>
  )
}
