import { useNavigate } from 'react-router-dom'
import { fmtDateTime } from '../utils/date'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { IconRefresh, IconArrowRight, IconServer } from '@tabler/icons-react'
import { listHosts } from '../api/hosts'
import { listPolicies } from '../api/policies'
import { listAuditLog } from '../api/misc'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import { TONE_VARS } from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import type { Tone } from '../components/QFBadge'

/* ── helpers ─────────────────────────────────────────────────────── */

function fmt(n: number) {
  return n >= 1000 ? (n / 1000).toFixed(1).replace(/\.0$/, '') + 'k' : String(n)
}

function actionTone(action: string): Tone {
  if (action.includes('push') || action.includes('bundle')) return 'pol'
  if (action.includes('delete') || action.includes('revoke')) return 'bad'
  if (action.includes('create') || action.includes('enroll')) return 'ok'
  return 'info'
}

/* ── Card with title header ──────────────────────────────────────── */
function CardHead({ title, right }: { title: string; right?: React.ReactNode }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'space-between',
      padding: '12px 16px', borderBottom: '1px solid var(--qf-border-2)',
    }}>
      <span style={{ fontSize: 'var(--qf-t-md)', fontWeight: 600, color: 'var(--qf-fg-2)' }}>{title}</span>
      {right}
    </div>
  )
}

/* ── KPI card ────────────────────────────────────────────────────── */
interface BreakdownSeg { value: number; tone: Tone }
interface KpiProps {
  label: string; value: string | number; sub?: string
  tone?: Tone; breakdown?: BreakdownSeg[]
}
function Kpi({ label, value, sub, tone = 'neutral', breakdown }: KpiProps) {
  const t = TONE_VARS[tone]
  return (
    <div style={{
      background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)',
      borderRadius: 'var(--qf-r-lg)', padding: 18,
      display: 'flex', flexDirection: 'column', gap: 8,
    }}>
      <span style={{
        fontSize: 'var(--qf-t-xs)', color: 'var(--qf-fg-mute)', fontWeight: 600,
        textTransform: 'uppercase', letterSpacing: '0.06em',
      }}>{label}</span>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
        <span style={{
          fontSize: 'var(--qf-t-3xl)', fontWeight: 700, lineHeight: 1,
          letterSpacing: '-0.02em', fontFamily: 'var(--qf-mono)',
          color: tone === 'neutral' ? 'var(--qf-fg-1)' : t.fg,
        }}>{value}</span>
        {sub && <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>{sub}</span>}
      </div>
      {breakdown && (
        <div style={{ display: 'flex', height: 4, borderRadius: 'var(--qf-r-full)', overflow: 'hidden', gap: 1.5 }}>
          {breakdown.filter(b => b.value > 0).map((b, i) => (
            <span key={i} style={{ flex: b.value, background: TONE_VARS[b.tone].solid }} />
          ))}
        </div>
      )}
    </div>
  )
}

/* ── SVG Donut ────────────────────────────────────────────────────── */
interface DonutSeg { value: number; tone: Tone; label: string }
function Donut({ data, total }: { data: DonutSeg[]; total: number }) {
  const r = 54, sw = 15, C = 2 * Math.PI * r
  let acc = 0
  const segs = data.filter(d => d.value > 0)
  return (
    <svg width="148" height="148" viewBox="0 0 148 148" style={{ flexShrink: 0 }}>
      <circle cx="74" cy="74" r={r} fill="none" stroke="var(--qf-border-2)" strokeWidth={sw} />
      {segs.map((d, i) => {
        const len = (d.value / total) * C
        const seg = (
          <circle key={i} cx="74" cy="74" r={r} fill="none"
            stroke={TONE_VARS[d.tone].solid} strokeWidth={sw}
            strokeDasharray={`${len} ${C - len}`}
            strokeDashoffset={-acc}
            transform="rotate(-90 74 74)"
          />
        )
        acc += len; return seg
      })}
      <text x="74" y="69" textAnchor="middle" fontSize="26" fontWeight="700"
        fill="var(--qf-fg-1)" fontFamily="var(--qf-mono)">{fmt(total)}</text>
      <text x="74" y="87" textAnchor="middle" fontSize="11" fontWeight="600"
        fill="var(--qf-fg-mute)" letterSpacing="0.08em">HOSTS</text>
    </svg>
  )
}

/* ── Sparkline ────────────────────────────────────────────────────── */
function Spark({ data, color }: { data: number[]; color: string }) {
  const max = Math.max(...data, 1)
  return (
    <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 34 }}>
      {data.map((v, i) => (
        <span key={i} style={{
          flex: 1, height: `${(v / max) * 100}%`, minWidth: 2,
          background: color, borderRadius: 1,
          opacity: i > data.length - 4 ? 1 : 0.45,
        }} />
      ))}
    </div>
  )
}

/* ── Actor resolution ─────────────────────────────────────────────── */
function resolveActor(a: { actor_username?: string; actor_type: string; after: unknown }): string {
  if (a.actor_username) return a.actor_username
  if (a.after && typeof a.after === 'object') {
    const u = (a.after as Record<string, unknown>).attempted_username
    if (typeof u === 'string' && u) return u
  }
  return a.actor_type
}

/* ── Actor avatar ─────────────────────────────────────────────────── */
function ActorAvatar({ actor }: { actor: string }) {
  const isSystem = actor === 'system' || actor === 'api_token' || actor === 'ci-bot'
  const initials = actor.split(/[.\-_@]/).map(s => s[0]).join('').toUpperCase().slice(0, 2) || '?'
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      <span style={{
        width: 22, height: 22, borderRadius: '50%', flexShrink: 0,
        background: isSystem ? 'var(--qf-bg-muted)' : 'var(--qf-indigo-600)',
        color: isSystem ? 'var(--qf-fg-mute)' : '#fff',
        display: 'grid', placeItems: 'center',
        fontSize: 9, fontWeight: 700,
      }}>{initials}</span>
      <span style={{ fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-2)' }}>
        {actor}
      </span>
    </span>
  )
}

/* ── Static spark (no timeseries API yet) ─────────────────────────── */
const PUSH_SPARK = [3,5,4,8,6,9,7,11,8,6,10,12,9,7,8,11,13,10,9,8,12,10,11,9]
const AGENT_RANK_TONE: Tone[] = ['ok', 'info', 'warn', 'bad']

/* ══════════════════════════════════════════════════════════════════ */
export default function Dashboard() {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const {
    data: hosts = [], isLoading: hostsLoading, isError: hostsError,
  } = useQuery({ queryKey: ['hosts'], queryFn: listHosts })

  const { data: policies = [], isLoading: policiesLoading } = useQuery({
    queryKey: ['policies'], queryFn: listPolicies,
  })

  const { data: audits = [], isLoading: auditsLoading } = useQuery({
    queryKey: ['audit-log', { limit: 8 }],
    queryFn: () => listAuditLog({ limit: 8 }),
  })

  function handleRefresh() {
    qc.invalidateQueries({ queryKey: ['hosts'] })
    qc.invalidateQueries({ queryKey: ['policies'] })
    qc.invalidateQueries({ queryKey: ['audit-log'] })
  }

  const isLoading = hostsLoading || policiesLoading
  const total = hosts.length

  /* status breakdown */
  const sc = hosts.reduce((a, h) => { a[h.status] = (a[h.status] ?? 0) + 1; return a }, {} as Record<string,number>)
  const active = sc['active'] ?? 0
  const offline = sc['offline'] ?? 0
  const enrolling = sc['enrolling'] ?? 0
  const errored = sc['error'] ?? 0

  const donutData: DonutSeg[] = [
    { value: active,   tone: 'ok',   label: 'Active' },
    { value: enrolling, tone: 'warn', label: 'Enrolling' },
    { value: offline,  tone: 'bad',  label: 'Offline' },
    { value: errored,  tone: 'term', label: 'Error' },
  ]

  const breakdownSegs: BreakdownSeg[] = donutData.filter(d => d.value > 0)
    .map(d => ({ value: d.value, tone: d.tone }))

  /* agent versions — derived from real host data */
  const agentVersionMap = hosts.reduce((a, h) => {
    const v = h.agent_version ?? 'unknown'
    a[v] = (a[v] ?? 0) + 1; return a
  }, {} as Record<string,number>)
  const agentVersions = Object.entries(agentVersionMap)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)

  /* ── Error state ── */
  if (hostsError) {
    return (
      <>
        <PageHead title="Fleet overview" sub="Real-time enforcement status across the fleet" />
        <QFCard>
          <div style={{ padding: '48px 24px', textAlign: 'center' }}>
            <p style={{ fontFamily: 'var(--qf-mono)', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)', margin: '0 0 16px' }}>
              GET /api/v1/hosts → failed
            </p>
            <button onClick={handleRefresh} style={{ padding: '7px 14px', borderRadius: 'var(--qf-r-md)', border: '1px solid var(--qf-border-1)', background: 'var(--qf-bg-muted)', color: 'var(--qf-fg-2)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)' }}>
              Retry
            </button>
          </div>
        </QFCard>
      </>
    )
  }

  /* ── Empty state ── */
  const isEmpty = !isLoading && total === 0

  return (
    <>
      <PageHead
        title="Fleet overview"
        sub={isLoading ? undefined : `${fmt(total)} hosts · ${policies.length} policies · enforcing now`}
        actions={
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            {!isLoading && (
              <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
                Live · WS
              </span>
            )}
            <button
              onClick={handleRefresh}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: 'transparent', color: 'var(--qf-fg-2)', border: '1px solid var(--qf-border-1)' }}
            >
              <IconRefresh size={13} /> Refresh
            </button>
          </div>
        }
      />

      {/* ── KPI row ── */}
      {isLoading ? (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 14, marginBottom: 14 }}>
          {Array(5).fill(0).map((_, i) => (
            <div key={i} style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', padding: 18, display: 'flex', flexDirection: 'column', gap: 12 }}>
              <div className="qf-skeleton" style={{ width: '55%', height: 9, borderRadius: 4 }} />
              <div className="qf-skeleton" style={{ width: '65%', height: 28, borderRadius: 4 }} />
            </div>
          ))}
        </div>
      ) : isEmpty ? null : (
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 14, marginBottom: 14 }}>
          <Kpi label="Total Hosts" value={fmt(total)} breakdown={breakdownSegs} />
          <Kpi label="Active" value={fmt(active)} tone="ok" sub={total > 0 ? `${Math.round(active / total * 100)}%` : undefined} />
          <Kpi label="Offline" value={offline} tone="bad" sub={offline > 0 ? 'needs attn' : undefined} />
          <Kpi label="Enrolling" value={enrolling} tone="warn" />
          <Kpi label="Policies" value={policies.length} tone="pol" sub={`${policies.length} active`} />
        </div>
      )}

      {isEmpty ? (
        /* ── Empty fleet ── */
        <QFCard style={{ minHeight: 420 }}>
          <EmptyState
            icon={<IconServer size={48} />}
            title="No hosts enrolled"
            body="Once you install the qf agent and it checks in, this overview will show fleet health, agent versions, and bundle push activity in real time."
            action={
              <div style={{ display: 'flex', gap: 10 }}>
                <button onClick={() => navigate('/tokens')} style={{ padding: '8px 16px', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600 }}>
                  Create enrollment token
                </button>
              </div>
            }
          />
        </QFCard>
      ) : (
        <>
          {/* ── Middle row ── */}
          {isLoading ? (
            <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 1.1fr 1fr', gap: 14, marginBottom: 14 }}>
              {Array(3).fill(0).map((_, i) => (
                <div key={i} style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', overflow: 'hidden' }}>
                  <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--qf-border-2)' }}>
                    <div className="qf-skeleton" style={{ width: 110, height: 12, borderRadius: 4 }} />
                  </div>
                  <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {Array(4).fill(0).map((_, j) => <div key={j} className="qf-skeleton" style={{ width: `${90 - j * 12}%`, height: 12, borderRadius: 4 }} />)}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div style={{ display: 'grid', gridTemplateColumns: '1.1fr 1.1fr 1fr', gap: 14, marginBottom: 14 }}>

              {/* Fleet status — Donut */}
              <div style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', overflow: 'hidden' }}>
                <CardHead title="Fleet status" />
                <div style={{ padding: 16, display: 'flex', alignItems: 'center', gap: 18 }}>
                  <Donut data={donutData} total={total} />
                  <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 11 }}>
                    {donutData.filter(d => d.value > 0).map(s => (
                      <div key={s.tone} style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
                        <span style={{ width: 9, height: 9, borderRadius: 2, background: TONE_VARS[s.tone].solid, flexShrink: 0 }} />
                        <span style={{ flex: 1, fontSize: 'var(--qf-t-md)', color: 'var(--qf-fg-2)' }}>{s.label}</span>
                        <span style={{ fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-md)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>{fmt(s.value)}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </div>

              {/* Agent versions */}
              <div style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', overflow: 'hidden' }}>
                <CardHead title="Agent versions" />
                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 14 }}>
                  {agentVersions.length === 0 ? (
                    <span style={{ color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>No agent data</span>
                  ) : agentVersions.map(([v, count], i) => {
                    const pct = total > 0 ? Math.round(count / total * 100) : 0
                    const tone = AGENT_RANK_TONE[i] ?? 'bad'
                    return (
                      <div key={v} style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--qf-t-base)' }}>
                          <span style={{ fontFamily: 'var(--qf-mono)', color: 'var(--qf-fg-2)' }}>{v}</span>
                          <span style={{ fontFamily: 'var(--qf-mono)', color: 'var(--qf-fg-mute)' }}>{fmt(count)} · {pct}%</span>
                        </div>
                        <div style={{ height: 6, background: 'var(--qf-bg-inset)', borderRadius: 'var(--qf-r-full)', overflow: 'hidden' }}>
                          <span style={{ display: 'block', height: '100%', width: `${pct}%`, background: TONE_VARS[tone].solid, borderRadius: 'var(--qf-r-full)' }} />
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* Bundle pushes */}
              <div style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', overflow: 'hidden' }}>
                <CardHead
                  title="Bundle pushes"
                  right={<span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>24h</span>}
                />
                <div style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
                  <div style={{ display: 'flex', alignItems: 'baseline', gap: 8 }}>
                    <span style={{ fontSize: 'var(--qf-t-3xl)', fontWeight: 700, fontFamily: 'var(--qf-mono)', letterSpacing: '-0.02em', color: 'var(--qf-fg-1)' }}>
                      {audits.filter(a => a.action.toLowerCase().includes('push')).length || '—'}
                    </span>
                    <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-ok-fg)' }}>all converged</span>
                  </div>
                  <Spark data={PUSH_SPARK} color="var(--qf-brand-solid)" />
                  <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
                    <span>7d trend</span>
                    <span>p95 ~1.2s</span>
                  </div>
                </div>
              </div>

            </div>
          )}

          {/* ── Recent activity ── */}
          <div style={{ background: 'var(--qf-bg-surface)', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-lg)', overflow: 'hidden' }}>
            <CardHead
              title="Recent activity"
              right={
                <button onClick={() => navigate('/audit')} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, background: 'none', border: 'none', color: 'var(--qf-brand)', fontSize: 'var(--qf-t-sm)', cursor: 'pointer', fontFamily: 'inherit', fontWeight: 500, padding: 0 }}>
                  View audit log <IconArrowRight size={12} />
                </button>
              }
            />
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--qf-t-base)' }}>
              <thead>
                <tr style={{ height: 36 }}>
                  {(['Time', 'Actor', 'Action', 'Object'] as const).map((h, i) => (
                    <th key={h} style={{
                      padding: '0 16px', textAlign: 'left',
                      fontSize: 'var(--qf-t-xs)', fontWeight: 600,
                      color: 'var(--qf-fg-mute)', textTransform: 'uppercase', letterSpacing: '0.05em',
                      borderBottom: '1px solid var(--qf-border-1)',
                      width: i === 0 ? 140 : i === 1 ? 180 : i === 2 ? 180 : 'auto',
                    }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {auditsLoading
                  ? Array(6).fill(0).map((_, i) => (
                    <tr key={i} style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                      <td colSpan={4} style={{ padding: '0 16px', height: 40 }}>
                        <div style={{ display: 'flex', gap: 24, alignItems: 'center' }}>
                          {[90, 140, 100, 160].map((w, j) => <div key={j} className="qf-skeleton" style={{ width: w, height: 12, borderRadius: 4 }} />)}
                        </div>
                      </td>
                    </tr>
                  ))
                  : audits.length === 0
                  ? (
                    <tr><td colSpan={4} style={{ padding: '24px 16px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>
                      No activity yet
                    </td></tr>
                  )
                  : audits.map((a) => (
                    <tr key={a.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                      <td style={{ padding: '0 16px', height: 40, fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-mute)', whiteSpace: 'nowrap' }}>
                        {fmtDateTime(a.created_at)}
                      </td>
                      <td style={{ padding: '0 16px', height: 40 }}>
                        <ActorAvatar actor={resolveActor(a)} />
                      </td>
                      <td style={{ padding: '0 16px', height: 40 }}>
                        <QFBadge tone={actionTone(a.action)}>{a.action}</QFBadge>
                      </td>
                      <td style={{ padding: '0 16px', height: 40, fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-3)', whiteSpace: 'nowrap' }}>
                        {a.object_type}{a.object_id ? ` · ${a.object_id.slice(0, 8)}` : ''}
                      </td>
                    </tr>
                  ))
                }
              </tbody>
            </table>
          </div>
        </>
      )}
    </>
  )
}
