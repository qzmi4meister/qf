import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import { Select } from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { listHosts, listFlows } from '../api/hosts'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'

function protoName(p: number): string {
  const names: Record<number, string> = { 1: 'any', 2: 'TCP', 3: 'UDP', 4: 'ICMP', 5: 'ICMPv6' }
  return names[p] ?? String(p)
}

function FilterPills({ opts, value, onChange }: {
  opts: Array<[string, string, string]>
  value: string | null
  onChange: (v: string | null) => void
}) {
  return (
    <div style={{ display: 'flex', gap: 6 }}>
      {opts.map(([k, label, tone]) => {
        const on = value === k
        return (
          <button key={k} onClick={() => onChange(on ? null : k)} style={{
            display: 'inline-flex', alignItems: 'center', gap: 6, padding: '6px 11px',
            fontSize: 'var(--qf-t-sm)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer',
            borderRadius: 'var(--qf-r-md)',
            border: `1px solid ${on ? `var(--qf-${tone}-solid)` : 'var(--qf-border-1)'}`,
            background: on ? `var(--qf-${tone}-bg)` : 'transparent',
            color: on ? `var(--qf-${tone}-fg)` : 'var(--qf-fg-3)',
          }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: `var(--qf-${tone}-solid)` }} />
            {label}
          </button>
        )
      })}
    </div>
  )
}

export default function Flows() {
  const [hostId, setHostId] = useState<string | null>(null)
  const [verdictFilter, setVerdictFilter] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const { data: hosts = [] } = useQuery({ queryKey: ['hosts'], queryFn: listHosts })

  const { data: flows = [], isLoading } = useQuery({
    queryKey: ['flows-all', hostId],
    queryFn: () => listFlows(hostId!, { limit: 200 }),
    enabled: !!hostId,
  })

  const filtered = flows.filter(f =>
    (!verdictFilter || f.final_state === verdictFilter) &&
    (!search || (f.src_ip ?? '').includes(search) || (f.dst_ip ?? '').includes(search))
  )

  return (
    <>
      <PageHead
        title="Flows"
        sub="Sampled packet verdicts"
        actions={undefined}
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
          value={verdictFilter}
          onChange={setVerdictFilter}
        />
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 10px', background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)', borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)' }}>
          <IconSearch size={13} />
          <input
            placeholder="Filter by IP…"
            value={search}
            onChange={e => setSearch(e.currentTarget.value)}
            style={{ width: 140, border: 'none', outline: 'none', background: 'transparent', color: 'var(--qf-fg-1)', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit' }}
          />
        </div>
        <span style={{ marginLeft: 'auto', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
          {hostId ? `${filtered.length} flows` : ''}
        </span>
      </div>

      {!hostId ? (
        <QFCard style={{ minHeight: 300 }}>
          <EmptyState title="Select a host" body="Choose a host above to explore its flows." />
        </QFCard>
      ) : (
        <QFCard pad={false}>
          <QFTable minWidth={800}>
            <thead>
              <tr style={{ height: 36 }}>
                <TH w={60}>Proto</TH>
                <TH>Src</TH>
                <TH>Dst</TH>
                <TH w={80} right>Bytes↑</TH>
                <TH w={80} right>Bytes↓</TH>
                <TH w={80} right>Pkts↑</TH>
                <TH w={80} right>Pkts↓</TH>
                <TH w={80}>State</TH>
                <TH w={140}>Started</TH>
              </tr>
            </thead>
            <tbody>
              {isLoading
                ? Array(8).fill(0).map((_, i) => <SkeletonRow key={i} cols={9} />)
                : filtered.length === 0
                ? (
                  <tr><td colSpan={9}>
                    <EmptyState title="No flows" body="No sampled flows match current filters." />
                  </td></tr>
                )
                : filtered.map(f => (
                  <tr key={f.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD mono muted>{protoName(f.protocol)}</TD>
                    <TD mono>{f.src_ip ?? '—'}{f.src_port ? `:${f.src_port}` : ''}</TD>
                    <TD mono>{f.dst_ip ?? '—'}{f.dst_port ? `:${f.dst_port}` : ''}</TD>
                    <TD mono muted right>{f.bytes_orig}</TD>
                    <TD mono muted right>{f.bytes_reply}</TD>
                    <TD mono muted right>{f.packets_orig}</TD>
                    <TD mono muted right>{f.packets_reply}</TD>
                    <TD mono muted>{f.final_state ?? '—'}</TD>
                    <TD mono muted>{fmtDateTime(f.started_at)}</TD>
                  </tr>
                ))
              }
            </tbody>
          </QFTable>
        </QFCard>
      )}
    </>
  )
}
