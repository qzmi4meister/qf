import { useNavigate } from 'react-router-dom'
import { fmtDateTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import { listHosts } from '../api/hosts'
import { listPolicies } from '../api/policies'
import { listAuditLog } from '../api/misc'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge, { TONE_VARS } from '../components/QFBadge'
import { QFTable, TH, TD } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import EmptyState from '../components/EmptyState'
import ErrorState from '../components/ErrorState'
import { IconServer } from '@tabler/icons-react'

type Tone = 'ok' | 'bad' | 'warn' | 'info' | 'pol' | 'term' | 'neutral'

/* ---------- Kpi card ---------- */
interface BreakdownSegment { value: number; tone: Tone }
interface KpiProps {
  label: string
  value: string | number
  sub?: string
  tone?: Tone
  breakdown?: BreakdownSegment[]
}
function Kpi({ label, value, sub, tone = 'neutral', breakdown }: KpiProps) {
  const t = TONE_VARS[tone]
  return (
    <div style={{
      background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)',
      borderRadius: 'var(--qf-r-lg)', padding: 18,
      display: 'flex', flexDirection: 'column', gap: 8,
    }}>
      <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {label}
      </span>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{
          fontSize: 'var(--qf-t-3xl)', fontWeight: 700,
          color: tone === 'neutral' ? 'var(--qf-fg-1)' : t.fg,
          lineHeight: 1, letterSpacing: '-0.02em', fontFamily: 'var(--qf-mono)',
        }}>{value}</span>
        {sub && <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>{sub}</span>}
      </div>
      {breakdown && (
        <div style={{ display: 'flex', height: 5, borderRadius: 'var(--qf-r-full)', overflow: 'hidden', gap: 1.5, marginTop: 2 }}>
          {breakdown.map((b, i) => (
            <span key={i} style={{ flex: b.value, background: TONE_VARS[b.tone].solid }} />
          ))}
        </div>
      )}
    </div>
  )
}

/* ---------- SVG Donut ---------- */
interface DonutSegment { value: number; tone: Tone; label: string }
function Donut({ data, total }: { data: DonutSegment[]; total: number }) {
  const r = 54, sw = 15, C = 2 * Math.PI * r
  let acc = 0
  return (
    <svg width="148" height="148" viewBox="0 0 148 148">
      <circle cx="74" cy="74" r={r} fill="none" stroke="var(--qf-border-2)" strokeWidth={sw} />
      {data.filter(d => d.value > 0).map((d, i) => {
        const len = (d.value / total) * C
        const seg = (
          <circle key={i} cx="74" cy="74" r={r} fill="none"
            stroke={TONE_VARS[d.tone].solid}
            strokeWidth={sw}
            strokeDasharray={`${len} ${C - len}`}
            strokeDashoffset={-acc}
            transform="rotate(-90 74 74)"
          />
        )
        acc += len
        return seg
      })}
      <text x="74" y="70" textAnchor="middle" fontSize="26" fontWeight="700"
        fill="var(--qf-fg-1)" fontFamily="var(--qf-mono)">{total}</text>
      <text x="74" y="88" textAnchor="middle" fontSize="11" fontWeight="600"
        fill="var(--qf-fg-mute)" letterSpacing="0.08em">HOSTS</text>
    </svg>
  )
}

/* ---------- Sparkline ---------- */
function Spark({ data, color }: { data: number[]; color: string }) {
  const max = Math.max(...data, 1)
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 34 }}>
      {data.map((v, i) => (
        <span key={i} style={{
          flex: 1, height: `${(v / max) * 100}%`, minWidth: 2,
          background: color, borderRadius: 1,
          opacity: i > data.length - 4 ? 1 : 0.5,
        }} />
      ))}
    </div>
  )
}

/* ---------- action → tone ---------- */
function actionTone(action: string): Tone {
  if (action.includes('push') || action.includes('bundle')) return 'pol'
  if (action.includes('delete') || action.includes('revoke') || action.includes('deny')) return 'bad'
  if (action.includes('create') || action.includes('enroll') || action.includes('allow')) return 'ok'
  return 'info'
}

/* static spark data (no timeseries API yet) */
const PUSH_SPARK = [3, 5, 4, 8, 6, 9, 7, 11, 8, 6, 10, 12, 9, 7, 8, 11, 13, 10, 9, 8, 12, 10, 11, 9]

export default function Dashboard() {
  const navigate = useNavigate()

  const { data: hosts = [], isLoading: hostsLoading, isError: hostsError, refetch: refetchHosts } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const { data: policies = [], isLoading: policiesLoading } = useQuery({
    queryKey: ['policies'],
    queryFn: listPolicies,
    refetchInterval: 30_000,
  })
  const { data: audits = [], isLoading: auditsLoading } = useQuery({
    queryKey: ['audit-log', { limit: 8 }],
    queryFn: () => listAuditLog({ limit: 8 }),
    refetchInterval: 30_000,
  })

  const isLoading = hostsLoading || policiesLoading

  const statusCounts = hosts.reduce((acc, h) => {
    acc[h.status] = (acc[h.status] ?? 0) + 1
    return acc
  }, {} as Record<string, number>)

  const active = statusCounts['active'] ?? 0
  const offline = statusCounts['offline'] ?? 0
  const enrolling = statusCounts['enrolling'] ?? 0
  const total = hosts.length

  const donutData: DonutSegment[] = [
    { value: active, tone: 'ok', label: 'Active' },
    { value: enrolling, tone: 'warn', label: 'Enrolling' },
    { value: offline, tone: 'bad', label: 'Offline' },
    { value: statusCounts['error'] ?? 0, tone: 'term', label: 'Error' },
  ]

  const breakdownSegments: BreakdownSegment[] = [
    { value: active, tone: 'ok' },
    { value: enrolling, tone: 'warn' },
    { value: offline, tone: 'bad' },
    { value: statusCounts['error'] ?? 0, tone: 'term' },
  ].filter(s => s.value > 0)

  if (hostsError) {
    return (
      <div>
        <PageHead title="Fleet overview" sub="Real-time enforcement status across the fleet" />
        <QFCard>
          <ErrorState title="Couldn't load fleet data" onRetry={refetchHosts} />
        </QFCard>
      </div>
    )
  }

  return (
    <>
      <PageHead
        title="Fleet overview"
        sub={isLoading ? undefined : `${total} hosts · ${policies.length} policies`}
        actions={
          <button
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '8px 14px', fontSize: 'var(--qf-t-base)', fontWeight: 600,
              fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)',
              background: 'var(--qf-brand-solid)', color: '#fff', border: 'none',
            }}
          >
            Push bundle
          </button>
        }
      />

      {/* KPI row */}
      {isLoading ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 14, marginBottom: 14 }}>
          {Array(5).fill(0).map((_, i) => (
            <div key={i} style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', padding: 18, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div className="qf-skeleton" style={{ width: '55%', height: 10, borderRadius: 4 }} />
              <div className="qf-skeleton" style={{ width: '70%', height: 26, borderRadius: 4 }} />
            </div>
          ))}
        </div>
      ) : total === 0 ? (
        <QFCard style={{ minHeight: 420 }}>
          <EmptyState
            icon={<IconServer size={48} />}
            title="No hosts enrolled"
            body="Once you install the qf agent and it checks in, this overview will show fleet health, agent versions, and bundle push activity in real time."
            action={
              <button
                onClick={() => navigate('/tokens')}
                style={{ padding: '8px 16px', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600 }}
              >
                Create enrollment token
              </button>
            }
          />
        </QFCard>
      ) : (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 14, marginBottom: 14 }}>
            <Kpi label="Total Hosts" value={total} breakdown={breakdownSegments} />
            <Kpi label="Active" value={active} tone="ok" sub={total > 0 ? `${Math.round(active / total * 100)}%` : undefined} />
            <Kpi label="Offline" value={offline} tone="bad" sub={offline > 0 ? 'needs attn' : undefined} />
            <Kpi label="Enrolling" value={enrolling} tone="warn" />
            <Kpi label="Policies" value={policies.length} tone="pol" />
          </div>

          {/* Middle row */}
          <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 1.1fr 1fr', gap: 14, marginBottom: 14 }}>
            {/* Fleet status donut */}
            <QFCard>
              <div style={{ fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-2)', marginBottom: 14 }}>Fleet status</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
                <Donut data={donutData} total={total} />
                <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 10 }}>
                  {donutData.filter(d => d.value > 0).map(s => (
                    <div key={s.tone} style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
                      <span style={{ width: 9, height: 9, borderRadius: 3, background: TONE_VARS[s.tone].solid, flexShrink: 0 }} />
                      <span style={{ flex: 1, fontSize: 'var(--qf-t-md)', color: 'var(--qf-fg-2)' }}>{s.label}</span>
                      <span style={{ fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-md)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>{s.value}</span>
                    </div>
                  ))}
                </div>
              </div>
            </QFCard>

            {/* Agent versions (placeholder - same data structure) */}
            <QFCard>
              <div style={{ fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-2)', marginBottom: 14 }}>Host status breakdown</div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 13 }}>
                {donutData.filter(d => d.value > 0).map(s => {
                  const pct = total > 0 ? Math.round(s.value / total * 100) : 0
                  return (
                    <div key={s.tone} style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--qf-t-base)' }}>
                        <span style={{ color: TONE_VARS[s.tone].fg }}>{s.label}</span>
                        <span style={{ color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>{s.value} · {pct}%</span>
                      </div>
                      <div style={{ height: 7, background: 'var(--qf-bg-inset)', borderRadius: 'var(--qf-r-full)', overflow: 'hidden' }}>
                        <span style={{ display: 'block', height: '100%', width: `${pct}%`, background: TONE_VARS[s.tone].solid, borderRadius: 'var(--qf-r-full)' }} />
                      </div>
                    </div>
                  )
                })}
              </div>
            </QFCard>

            {/* Bundle pushes sparkline */}
            <QFCard>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 14 }}>
                <span style={{ fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-2)' }}>Bundle pushes</span>
                <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>24h</span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
                  <span style={{ fontSize: 'var(--qf-t-3xl)', fontWeight: 700, fontFamily: 'var(--qf-mono)', letterSpacing: '-0.02em', color: 'var(--qf-fg-1)' }}>
                    {audits.filter(a => a.action.includes('push') || a.action.includes('bundle')).length || '—'}
                  </span>
                  <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-ok-fg)' }}>pushes</span>
                </div>
                <Spark data={PUSH_SPARK} color="var(--qf-brand-solid)" />
                <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
                  <span>7d trend</span><span>p95 ~1s</span>
                </div>
              </div>
            </QFCard>
          </div>

          {/* Recent activity */}
          <QFCard pad={false}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '12px 16px', borderBottom: '1px solid var(--qf-border-2)' }}>
              <span style={{ fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-2)' }}>Recent activity</span>
              <button
                onClick={() => navigate('/audit')}
                style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', fontSize: 'var(--qf-t-sm)', cursor: 'pointer', fontFamily: 'inherit', padding: 0 }}
              >
                View audit log ›
              </button>
            </div>
            <QFTable>
              <thead>
                <tr style={{ height: 36 }}>
                  <TH w={120}>Time</TH>
                  <TH w={120}>Actor</TH>
                  <TH w={160}>Action</TH>
                  <TH>Object</TH>
                </tr>
              </thead>
              <tbody>
                {auditsLoading ? (
                  Array(5).fill(0).map((_, i) => <SkeletonRow key={i} cols={4} />)
                ) : audits.map((a) => (
                  <tr key={a.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD mono muted>{fmtDateTime(a.created_at)}</TD>
                    <TD mono>{a.actor_type}{a.actor_id ? ` · ${a.actor_id.slice(0, 8)}` : ''}</TD>
                    <TD><QFBadge tone={actionTone(a.action)}>{a.action}</QFBadge></TD>
                    <TD mono muted>{a.object_type}{a.object_id ? ` ${a.object_id.slice(0, 8)}` : ''}</TD>
                  </tr>
                ))}
                {!auditsLoading && audits.length === 0 && (
                  <tr><td colSpan={4}><div style={{ padding: '20px 16px', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)', textAlign: 'center' }}>No activity yet</div></td></tr>
                )}
              </tbody>
            </QFTable>
          </QFCard>
        </>
      )}
    </>
  )
}
