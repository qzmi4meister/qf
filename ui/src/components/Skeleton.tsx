import type { CSSProperties } from 'react'

interface SkeletonProps {
  w?: number | string
  h?: number | string
  r?: number | string
  style?: CSSProperties
}

export default function Skeleton({ w = '100%', h = 12, r = 4, style }: SkeletonProps) {
  return (
    <span className="qf-skeleton" style={{
      width: w, height: h,
      borderRadius: r,
      ...style,
    }} />
  )
}

export function SkeletonRow({ cols = 5 }: { cols?: number }) {
  return (
    <tr style={{ borderTop: '1px solid var(--qf-border-2)' }}>
      <td colSpan={cols} style={{ padding: '0 16px', height: 'var(--qf-row-h)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 20 }}>
          <Skeleton w={16} h={16} r={4} />
          <Skeleton w={160} />
          <Skeleton w={80} />
          <Skeleton w={110} />
          <Skeleton w={120} />
          <Skeleton w={80} style={{ marginLeft: 'auto' }} />
        </div>
      </td>
    </tr>
  )
}
