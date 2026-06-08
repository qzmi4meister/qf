import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import { Modal, Group, Button, Text, Code } from '@mantine/core'
import { IconDownload, IconSearch } from '@tabler/icons-react'
import { listAuditLog } from '../api/misc'
import { useSortState } from '../hooks/useSortState'
import { useInfiniteScroll } from '../hooks/useInfiniteScroll'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import type { Tone } from '../components/QFBadge'

function actionTone(action: string): Tone {
  if (action.includes('push') || action.includes('bundle')) return 'pol'
  if (action.includes('delete') || action.includes('revoke')) return 'bad'
  if (action.includes('create') || action.includes('enroll')) return 'ok'
  return 'info'
}

function ActorAvatar({ actor }: { actor: string }) {
  const isSystem = actor === 'system' || actor === 'ci-bot' || actor === 'api_token'
  const initials = actor.split(/[.\-_]/).map(s => s[0]).join('').toUpperCase().slice(0, 2)
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      <span style={{
        width: 22, height: 22, borderRadius: '50%',
        background: isSystem ? 'var(--qf-bg-muted)' : 'var(--qf-indigo-600)',
        color: isSystem ? 'var(--qf-fg-mute)' : '#fff',
        display: 'grid', placeItems: 'center',
        fontSize: 9, fontWeight: 700, flexShrink: 0,
      }}>{initials || '?'}</span>
      <span style={{ fontFamily: 'var(--qf-mono)', color: 'var(--qf-fg-2)', fontSize: 'var(--qf-t-base)' }}>{actor}</span>
    </span>
  )
}

export default function AuditLog() {
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<string | null>(null)
  const { sort, toggle, sorted } = useSortState({ key: 'created_at', dir: 'desc' })

  const { data: logs = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['audit-log'],
    queryFn: () => listAuditLog({ limit: 200 }),
  })

  const filtered = sorted(
    logs.filter(l =>
      !search ||
      (l.actor_id ?? '').toLowerCase().includes(search.toLowerCase()) ||
      (l.actor_type ?? '').toLowerCase().includes(search.toLowerCase()) ||
      l.action.toLowerCase().includes(search.toLowerCase()) ||
      (l.object_id ?? '').includes(search)
    ),
    (l, k) => k === 'created_at' ? l.created_at : k === 'action' ? l.action : undefined,
  )

  const { visible, sentinelRef } = useInfiniteScroll(filtered)

  function exportCSV() {
    const header = 'id,time,actor_type,actor_id,action,object_type,object_id\n'
    const rows = filtered.map(l =>
      [l.id, l.created_at, l.actor_type, l.actor_id ?? '', l.action, l.object_type, l.object_id ?? ''].join(',')
    ).join('\n')
    const blob = new Blob([header + rows], { type: 'text/csv' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'audit-log.csv'
    a.click()
    URL.revokeObjectURL(url)
  }

  const expandedEntry = expanded ? logs.find(l => l.id === expanded) : null

  return (
    <>
      <PageHead
        title="Audit Log"
        sub="Every change to policies, hosts, tokens, and access"
        actions={
          <button
            onClick={exportCSV}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '7px 12px', fontSize: 'var(--qf-t-base)', fontWeight: 600,
              fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)',
              background: 'transparent', color: 'var(--qf-fg-2)',
              border: '1px solid var(--qf-border-1)',
            }}
          >
            <IconDownload size={14} /> Export CSV
          </button>
        }
      />

      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14 }}>
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8, padding: '7px 11px',
          flex: '0 0 280px', background: 'var(--qf-bg-input)',
          border: '1px solid var(--qf-border-input)', borderRadius: 'var(--qf-r-md)',
          color: 'var(--qf-fg-mute)',
        }}>
          <IconSearch size={14} />
          <input
            placeholder="Filter by actor or action…"
            value={search}
            onChange={e => setSearch(e.currentTarget.value)}
            style={{ flex: 1, border: 'none', outline: 'none', background: 'transparent', color: 'var(--qf-fg-1)', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit' }}
          />
        </div>
        <span style={{ marginLeft: 'auto', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
          {filtered.length} entries
        </span>
      </div>

      <QFCard pad={false}>
        <QFTable minWidth={700}>
          <thead>
            <tr style={{ height: 36 }}>
              <SortTH sortKey="created_at" sort={sort} onSort={toggle} w={140}>Time</SortTH>
              <TH w={180}>Actor</TH>
              <SortTH sortKey="action" sort={sort} onSort={toggle} w={160}>Action</SortTH>
              <TH>Object</TH>
              <TH w={90}>Source</TH>
            </tr>
          </thead>
          <tbody>
            {isLoading
              ? Array(8).fill(0).map((_, i) => <SkeletonRow key={i} cols={5} />)
              : isError
              ? (
                <tr><td colSpan={5}>
                  <div style={{ padding: 20, textAlign: 'center', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)', fontFamily: 'var(--qf-mono)' }}>
                    Failed to load audit log.{' '}
                    <button onClick={() => refetch()} style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'inherit' }}>Retry</button>
                  </div>
                </td></tr>
              )
              : visible.length === 0
              ? (
                <tr><td colSpan={5}>
                  <EmptyState title="No audit entries" body="Actions will appear here once operators make changes." />
                </td></tr>
              )
              : visible.map(l => (
                <tr
                  key={l.id}
                  className="qf-row"
                  style={{ borderTop: '1px solid var(--qf-border-2)', cursor: 'pointer' }}
                  onClick={() => setExpanded(expanded === l.id ? null : l.id)}
                >
                  <TD mono muted>{fmtDateTime(l.created_at)}</TD>
                  <td style={{ padding: '0 12px', height: 40 }}>
                    <ActorAvatar actor={l.actor_id ?? l.actor_type} />
                  </td>
                  <TD><QFBadge tone={actionTone(l.action)}>{l.action}</QFBadge></TD>
                  <TD mono muted>{l.object_type}{l.object_id ? ` ${l.object_id.slice(0, 8)}` : ''}</TD>
                  <TD mono muted>{l.actor_type}</TD>
                </tr>
              ))
            }
          </tbody>
        </QFTable>
        <div ref={sentinelRef} />
      </QFCard>

      {expandedEntry && (
        <Modal
          opened={!!expanded}
          onClose={() => setExpanded(null)}
          title={<span style={{ fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-base)' }}>{expandedEntry.action}</span>}
          size="lg"
        >
          <Group align="flex-start" gap="md">
            <div style={{ flex: 1 }}>
              <Text size="xs" fw={600} mb={4}>Before</Text>
              <Code block style={{ fontSize: 11 }}>
                {expandedEntry.before ? JSON.stringify(expandedEntry.before, null, 2) : 'null'}
              </Code>
            </div>
            <div style={{ flex: 1 }}>
              <Text size="xs" fw={600} mb={4}>After</Text>
              <Code block style={{ fontSize: 11 }}>
                {expandedEntry.after ? JSON.stringify(expandedEntry.after, null, 2) : 'null'}
              </Code>
            </div>
          </Group>
          <Button mt="md" variant="subtle" onClick={() => setExpanded(null)} fullWidth>Close</Button>
        </Modal>
      )}
    </>
  )
}
