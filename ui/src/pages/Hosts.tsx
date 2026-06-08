import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Modal, Button, Text, Group, ActionIcon } from '@mantine/core'
import { notifications } from '@mantine/notifications'
import { IconSearch, IconTrash, IconServer } from '@tabler/icons-react'
import { listHosts, deleteHost } from '../api/hosts'
import type { Host } from '../types'
import { useSortState } from '../hooks/useSortState'
import PageHead from '../components/PageHead'
import StatusBadge from '../components/StatusBadge'
import Chip from '../components/Chip'
import QFCard from '../components/QFCard'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import { TONE_VARS, type Tone } from '../components/QFBadge'

const STATUS_FILTERS: { status: string; label: string; tone: Tone }[] = [
  { status: 'active',    label: 'Active',    tone: 'ok' },
  { status: 'offline',   label: 'Offline',   tone: 'neutral' },
  { status: 'enrolling', label: 'Enrolling', tone: 'info' },
  { status: 'error',     label: 'Error',     tone: 'bad' },
]

export default function Hosts() {
  const queryClient = useQueryClient()
  const { data: hosts = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Host | null>(null)
  const { sort, toggle, sorted } = useSortState({ key: 'hostname', dir: 'asc' })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteHost(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['hosts'] })
      setDeleteTarget(null)
    },
    onError: (err: unknown) => {
      notifications.show({ message: err instanceof Error ? err.message : 'Delete failed', color: 'red' })
      setDeleteTarget(null)
    },
  })

  const filtered = hosts.filter((h) => {
    const matchSearch = !search || h.hostname.toLowerCase().includes(search.toLowerCase()) ||
      Object.entries(h.labels).some(([k, v]) => `${k}=${v}`.includes(search))
    const matchStatus = !statusFilter || h.status === statusFilter
    return matchSearch && matchStatus
  })

  const rows = sorted(filtered, (h, k) => {
    if (k === 'hostname') return h.hostname
    if (k === 'status') return h.status
    if (k === 'agent_version') return h.agent_version ?? ''
    if (k === 'current_generation') return h.current_generation
    if (k === 'last_heartbeat_at') return h.last_heartbeat_at ?? ''
    return undefined
  })

  return (
    <>
      <Modal opened={!!deleteTarget} onClose={() => setDeleteTarget(null)} title="Delete host" centered>
        <Text size="sm" mb="md">Delete <b>{deleteTarget?.hostname}</b>? This cannot be undone.</Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={() => setDeleteTarget(null)}>Cancel</Button>
          <Button color="red" loading={deleteMutation.isPending}
            onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}>
            Delete
          </Button>
        </Group>
      </Modal>

      <PageHead
        title="Hosts"
        sub={!isLoading ? `${hosts.length} enrolled · ${hosts.filter(h => h.status === 'active').length} enforcing` : undefined}
      />

      {/* Toolbar */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 14, flexWrap: 'wrap' }}>
        {/* Search */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8, padding: '7px 11px', flex: '0 0 280px',
          background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
          borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)',
        }}>
          <IconSearch size={14} />
          <input
            placeholder="Filter by hostname or label…"
            value={search}
            onChange={e => setSearch(e.currentTarget.value)}
            style={{
              flex: 1, border: 'none', outline: 'none',
              background: 'transparent', color: 'var(--qf-fg-1)',
              fontSize: 'var(--qf-t-base)', fontFamily: 'inherit',
            }}
          />
        </div>

        {/* Status filters */}
        <div style={{ display: 'flex', gap: 6 }}>
          {STATUS_FILTERS.map(({ status, label, tone }) => {
            const on = statusFilter === status
            const t = TONE_VARS[tone]
            return (
              <button
                key={status}
                onClick={() => setStatusFilter(on ? null : status)}
                style={{
                  display: 'inline-flex', alignItems: 'center', gap: 6,
                  padding: '6px 11px', fontSize: 'var(--qf-t-sm)', fontWeight: 600,
                  fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)',
                  border: `1px solid ${on ? t.solid : 'var(--qf-border-1)'}`,
                  background: on ? t.bg : 'transparent',
                  color: on ? t.fg : 'var(--qf-fg-3)',
                }}
              >
                <span style={{ width: 6, height: 6, borderRadius: '50%', background: t.solid }} />
                {label}
              </button>
            )
          })}
        </div>

        {/* Count */}
        <div style={{ marginLeft: 'auto', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>
          {!isLoading && <span><b style={{ color: 'var(--qf-fg-2)' }}>{rows.length}</b> of {hosts.length}</span>}
        </div>
      </div>

      <QFCard pad={false}>
        <QFTable minWidth={860}>
          <thead>
            <tr style={{ height: 38 }}>
              <SortTH sortKey="hostname" sort={sort} onSort={toggle}>Hostname</SortTH>
              <SortTH sortKey="status" sort={sort} onSort={toggle} w={120}>Status</SortTH>
              <TH w={130}>Labels</TH>
              <SortTH sortKey="agent_version" sort={sort} onSort={toggle} w={90}>Agent</SortTH>
              <SortTH sortKey="current_generation" sort={sort} onSort={toggle} w={70} right>Gen</SortTH>
              <SortTH sortKey="last_heartbeat_at" sort={sort} onSort={toggle} w={160} right>Last seen</SortTH>
              <TH w={40} />
            </tr>
          </thead>
          <tbody>
            {isLoading
              ? Array(8).fill(0).map((_, i) => <SkeletonRow key={i} cols={7} />)
              : rows.map(h => (
                <tr key={h.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                  <TD>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      <IconServer size={14} style={{ color: 'var(--qf-fg-mute)', flexShrink: 0 }} />
                      <Link
                        to={`/hosts/${h.id}`}
                        style={{ color: 'var(--qf-brand)', textDecoration: 'none', fontFamily: 'var(--qf-mono)', fontWeight: 500 }}
                      >
                        {h.hostname}
                      </Link>
                    </div>
                  </TD>
                  <TD><StatusBadge status={h.status} /></TD>
                  <TD>
                    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                      {Object.entries(h.labels).map(([k, v]) => <Chip key={k} k={k} v={v} />)}
                    </div>
                  </TD>
                  <TD mono muted>{h.agent_version ?? '—'}</TD>
                  <TD mono muted right>{h.current_generation}</TD>
                  <TD mono muted right>{fmtDateTime(h.last_heartbeat_at)}</TD>
                  <TD right>
                    <ActionIcon
                      variant="subtle" color="red" size="sm"
                      onClick={(e) => { e.stopPropagation(); setDeleteTarget(h) }}
                    >
                      <IconTrash size={13} />
                    </ActionIcon>
                  </TD>
                </tr>
              ))
            }
            {!isLoading && !isError && rows.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    icon={<IconServer size={48} />}
                    title={hosts.length === 0 ? 'No hosts enrolled yet' : 'No hosts match filter'}
                    body={hosts.length === 0
                      ? 'Install the qf agent on a host and issue it an enrollment token.'
                      : 'Try changing your search or status filter.'}
                  />
                </td>
              </tr>
            )}
            {isError && (
              <tr>
                <td colSpan={7}>
                  <EmptyState
                    title="Failed to load hosts"
                    body="Check your connection and try again."
                    action={<button onClick={() => refetch()} style={{ padding: '7px 14px', borderRadius: 'var(--qf-r-md)', border: '1px solid var(--qf-border-1)', background: 'var(--qf-bg-muted)', color: 'var(--qf-fg-2)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)' }}>Retry</button>}
                  />
                </td>
              </tr>
            )}
          </tbody>
        </QFTable>

        {!isLoading && rows.length > 0 && (
          <div style={{
            display: 'flex', alignItems: 'center', justifyContent: 'space-between',
            padding: '11px 16px', borderTop: '1px solid var(--qf-border-1)',
            fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)',
          }}>
            Showing {rows.length} of {hosts.length} hosts
          </div>
        )}
      </QFCard>
    </>
  )
}
