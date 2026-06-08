import { useState, useEffect, useRef, useMemo } from 'react'
import { fmtTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import { Select } from '@mantine/core'
import { IconSearch, IconDownload } from '@tabler/icons-react'
import { listHosts, listEvents, getHostRuleset } from '../api/hosts'
import type { LogEvent } from '../types'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import type { Tone } from '../components/QFBadge'

type RuleMap = Map<string, { rule_name: string; policy_name: string }>

function ruleLabel(ruleId: string | undefined, ruleMap: RuleMap): string {
  if (!ruleId) return '—'
  const r = ruleMap.get(ruleId)
  if (r?.rule_name) return r.policy_name ? `${r.rule_name} (${r.policy_name})` : r.rule_name
  return ruleId.slice(0, 8)
}

function actionTone(a: string): Tone {
  return a === 'deny' ? 'bad' : a === 'allow' ? 'ok' : 'info'
}

function protoName(p: number): string {
  const names: Record<number, string> = { 1: 'any', 2: 'TCP', 3: 'UDP', 4: 'ICMP', 5: 'ICMPv6' }
  return names[p] ?? String(p)
}

const VERDICT_TONE: Record<string, Tone> = { allow: 'ok', deny: 'bad', log: 'info' }

function FilterPills({ opts, value, onChange }: {
  opts: Array<[string, string, Tone]>
  value: string | null
  onChange: (v: string | null) => void
}) {
  return (
    <div style={{ display: 'flex', gap: 6 }}>
      {opts.map(([k, label, tone]) => {
        const on = value === k
        const t = { bg: `var(--qf-${tone}-bg)`, fg: `var(--qf-${tone}-fg)`, solid: `var(--qf-${tone}-solid)` }
        return (
          <button key={k} onClick={() => onChange(on ? null : k)} style={{
            display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 11px',
            fontSize: 'var(--qf-t-sm)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer',
            borderRadius: 'var(--qf-r-md)',
            border: `1px solid ${on ? t.solid : 'var(--qf-border-1)'}`,
            background: on ? t.bg : 'transparent', color: on ? t.fg : 'var(--qf-fg-3)',
          }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: t.solid }} />
            {label}
          </button>
        )
      })}
    </div>
  )
}

export default function Events() {
  const [hostId, setHostId] = useState<string | null>(null)
  const [actionFilter, setActionFilter] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [live, setLive] = useState(false)
  const [liveEvents, setLiveEvents] = useState<LogEvent[]>([])
  const [paused, setPaused] = useState(false)
  const esRef = useRef<EventSource | null>(null)
  const scrollRef = useRef<HTMLDivElement>(null)

  const { data: hosts = [] } = useQuery({ queryKey: ['hosts'], queryFn: listHosts })

  const { data: historyEvents = [], isLoading } = useQuery({
    queryKey: ['events-history', hostId, actionFilter],
    queryFn: () => listEvents(hostId!, { limit: 200, action: actionFilter ?? undefined }),
    enabled: !live && !!hostId,
  })

  const { data: ruleset } = useQuery({
    queryKey: ['ruleset', hostId],
    queryFn: () => getHostRuleset(hostId!),
    enabled: !!hostId,
  })
  const ruleMap: RuleMap = useMemo(() => {
    const m: RuleMap = new Map()
    for (const r of ruleset?.rules ?? []) {
      m.set(r.rule_id, { rule_name: r.rule_name, policy_name: r.policy_name })
    }
    return m
  }, [ruleset])

  useEffect(() => {
    if (!live || !hostId) {
      esRef.current?.close()
      esRef.current = null
      return
    }
    const url = `/hosts/${hostId}/events/stream`
    const es = new EventSource(url, { withCredentials: true })
    esRef.current = es
    es.addEventListener('log_event', (e) => {
      if (paused) return
      try {
        const event: LogEvent = JSON.parse(e.data)
        setLiveEvents(prev => [...prev, event].slice(-500))
      } catch { /* noop */ }
    })
    return () => { es.close() }
  }, [live, hostId])

  useEffect(() => {
    if (!paused && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [liveEvents, paused])

  const displayEvents = live ? liveEvents : historyEvents

  const filtered = displayEvents.filter(e => {
    const matchAction = !actionFilter || e.action === actionFilter
    const matchSearch = !search ||
      (e.src_ip ?? '').includes(search) ||
      (e.dst_ip ?? '').includes(search) ||
      (e.rule_id ?? '').includes(search) ||
      (e.src_port != null && String(e.src_port).includes(search)) ||
      (e.dst_port != null && String(e.dst_port).includes(search))
    return matchAction && matchSearch
  })

  return (
    <>
      <PageHead
        title="Events"
        sub="Agent and enforcement events"
        actions={
          <div style={{ display: 'flex', gap: 8 }}>
            <button style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: 'transparent', color: 'var(--qf-fg-2)', border: '1px solid var(--qf-border-1)', fontWeight: 600 }}>
              <IconDownload size={14} /> Export
            </button>
            <button
              onClick={() => { setLive(l => !l); if (!live) setLiveEvents([]) }}
              style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '7px 12px', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: live ? 'var(--qf-ok-bg)' : 'transparent', color: live ? 'var(--qf-ok-fg)' : 'var(--qf-fg-2)', border: `1px solid ${live ? 'var(--qf-ok-solid)' : 'var(--qf-border-1)'}`, fontWeight: 600 }}
            >
              {live ? 'Live ●' : 'Live'}
            </button>
          </div>
        }
      />

      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
        <Select
          placeholder="Select host"
          data={hosts.map(h => ({ value: h.id, label: h.hostname }))}
          value={hostId}
          onChange={setHostId}
          searchable
          style={{ width: 220 }}
          styles={{ input: { background: 'var(--qf-bg-input)', borderColor: 'var(--qf-border-input)', fontSize: 'var(--qf-t-base)', height: 34 } }}
        />
        <FilterPills
          opts={[['allow', 'Allow', 'ok'], ['deny', 'Deny', 'bad'], ['log', 'Log', 'info']]}
          value={actionFilter}
          onChange={setActionFilter}
        />
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 10px', background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)', borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)' }}>
          <IconSearch size={13} />
          <input
            placeholder="IP, port or rule ID…"
            value={search}
            onChange={e => setSearch(e.currentTarget.value)}
            style={{ width: 160, border: 'none', outline: 'none', background: 'transparent', color: 'var(--qf-fg-1)', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit' }}
          />
        </div>
        {live && (
          <button
            onClick={() => setPaused(p => !p)}
            style={{ padding: '6px 11px', border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-md)', background: 'transparent', color: 'var(--qf-fg-2)', fontSize: 'var(--qf-t-sm)', fontFamily: 'inherit', cursor: 'pointer', fontWeight: 600 }}
          >
            {paused ? 'Resume' : 'Pause'}
          </button>
        )}
        <span style={{ marginLeft: 'auto', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
          {hostId ? `${filtered.length} events` : ''}
        </span>
      </div>

      {!hostId ? (
        <QFCard style={{ minHeight: 300 }}>
          <EmptyState title="Select a host" body="Choose a host above to view its events." />
        </QFCard>
      ) : (
        <QFCard pad={false}>
          <div ref={scrollRef} style={{ maxHeight: 560, overflowY: 'auto' }}
            onMouseEnter={() => setPaused(true)}
            onMouseLeave={() => setPaused(false)}
          >
            <QFTable minWidth={700}>
              <thead style={{ position: 'sticky', top: 0, background: 'var(--qf-bg-surface)', zIndex: 1 }}>
                <tr style={{ height: 36 }}>
                  <TH w={90}>Time</TH>
                  <TH w={90}>Action</TH>
                  <TH w={60}>Dir</TH>
                  <TH w={60}>Proto</TH>
                  <TH>Src</TH>
                  <TH>Dst</TH>
                  <TH w={160}>Rule</TH>
                </tr>
              </thead>
              <tbody>
                {isLoading
                  ? Array(8).fill(0).map((_, i) => <SkeletonRow key={i} cols={7} />)
                  : filtered.length === 0
                  ? (
                    <tr><td colSpan={7}>
                      <EmptyState title={live ? 'Waiting for events…' : 'No events'} body="No events match the current filters." />
                    </td></tr>
                  )
                  : filtered.map(e => (
                    <tr key={e.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                      <TD mono muted>{fmtTime(e.created_at)}</TD>
                      <TD><QFBadge tone={actionTone(e.action)}>{e.action}</QFBadge></TD>
                      <TD mono muted>{e.direction}</TD>
                      <TD mono muted>{protoName(e.protocol)}</TD>
                      <TD mono>{e.src_ip ?? '—'}{e.src_port ? `:${e.src_port}` : ''}</TD>
                      <TD mono>{e.dst_ip ?? '—'}{e.dst_port ? `:${e.dst_port}` : ''}</TD>
                      <TD mono muted>{ruleLabel(e.rule_id, ruleMap)}</TD>
                    </tr>
                  ))
                }
              </tbody>
            </QFTable>
          </div>
        </QFCard>
      )}
    </>
  )
}
