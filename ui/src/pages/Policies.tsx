import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Modal, Button, Text, Group, Stack, ActionIcon } from '@mantine/core'
import { IconSearch, IconPlus, IconTrash, IconShield } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listPolicies, deletePolicy } from '../api/policies'
import { useSortState } from '../hooks/useSortState'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'

export default function Policies() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const { sort, toggle, sorted } = useSortState({ key: 'name', dir: 'asc' })

  const { data: policies = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['policies'],
    queryFn: listPolicies,
  })

  const deleteMut = useMutation({
    mutationFn: deletePolicy,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['policies'] })
      setDeleteId(null)
      notifications.show({ message: 'Policy deleted', color: 'green' })
    },
  })

  const filtered = policies.filter(p =>
    !search || p.name.toLowerCase().includes(search.toLowerCase())
  )

  const rows = sorted(filtered, (p, k) => {
    if (k === 'name') return p.name
    if (k === 'priority') return p.priority
    if (k === 'updated_at') return p.updated_at
    return undefined
  })

  return (
    <>
      <Modal opened={!!deleteId} onClose={() => setDeleteId(null)} title="Delete policy" size="sm">
        <Stack gap="md">
          <Text>Delete this policy? This cannot be undone.</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setDeleteId(null)}>Cancel</Button>
            <Button color="red" loading={deleteMut.isPending}
              onClick={() => deleteId && deleteMut.mutate(deleteId)}>
              Delete
            </Button>
          </Group>
        </Stack>
      </Modal>

      <PageHead
        title="Policies"
        sub={!isLoading ? `${policies.length} total` : undefined}
        actions={
          <button
            onClick={() => navigate('/policies/new')}
            style={{
              display: 'inline-flex', alignItems: 'center', gap: 6,
              padding: '8px 14px', fontSize: 'var(--qf-t-base)', fontWeight: 600,
              fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)',
              background: 'var(--qf-brand-solid)', color: '#fff', border: 'none',
            }}
          >
            <IconPlus size={14} /> New policy
          </button>
        }
      />

      {/* Search */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8, padding: '7px 11px',
        width: 280, background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
        borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)', marginBottom: 14,
      }}>
        <IconSearch size={14} />
        <input
          placeholder="Search policies…"
          value={search}
          onChange={e => setSearch(e.currentTarget.value)}
          style={{
            flex: 1, border: 'none', outline: 'none',
            background: 'transparent', color: 'var(--qf-fg-1)',
            fontSize: 'var(--qf-t-base)', fontFamily: 'inherit',
          }}
        />
      </div>

      <QFCard pad={false}>
        <QFTable minWidth={600}>
          <thead>
            <tr style={{ height: 38 }}>
              <SortTH sortKey="name" sort={sort} onSort={toggle}>Name</SortTH>
              <SortTH sortKey="priority" sort={sort} onSort={toggle} w={90}>Priority</SortTH>
              <TH w={80}>Version</TH>
              <SortTH sortKey="updated_at" sort={sort} onSort={toggle} w={160} right>Updated</SortTH>
              <TH w={40} />
            </tr>
          </thead>
          <tbody>
            {isLoading
              ? Array(5).fill(0).map((_, i) => <SkeletonRow key={i} cols={5} />)
              : rows.map(p => (
                <tr key={p.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                  <TD>
                    <Link
                      to={`/policies/${p.id}`}
                      style={{ color: 'var(--qf-brand)', textDecoration: 'none', fontWeight: 500 }}
                    >
                      {p.name}
                    </Link>
                  </TD>
                  <TD mono muted>{p.priority}</TD>
                  <TD mono muted>v{p.current_version}</TD>
                  <TD mono muted right>{fmtDateTime(p.updated_at)}</TD>
                  <TD right>
                    <ActionIcon variant="subtle" color="red" size="sm" onClick={() => setDeleteId(p.id)}>
                      <IconTrash size={13} />
                    </ActionIcon>
                  </TD>
                </tr>
              ))
            }
            {!isLoading && !isError && rows.length === 0 && (
              <tr>
                <td colSpan={5}>
                  <EmptyState
                    icon={<IconShield size={48} />}
                    title={policies.length === 0 ? 'No policies yet' : 'No policies match search'}
                    body={policies.length === 0
                      ? 'Create a policy to define firewall rules for your hosts.'
                      : 'Try a different search term.'}
                    action={policies.length === 0
                      ? <button onClick={() => navigate('/policies/new')} style={{ padding: '8px 16px', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600 }}>New policy</button>
                      : undefined
                    }
                  />
                </td>
              </tr>
            )}
            {isError && (
              <tr>
                <td colSpan={5}>
                  <EmptyState
                    title="Failed to load policies"
                    action={<button onClick={() => refetch()} style={{ padding: '7px 14px', borderRadius: 'var(--qf-r-md)', border: '1px solid var(--qf-border-1)', background: 'var(--qf-bg-muted)', color: 'var(--qf-fg-2)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)' }}>Retry</button>}
                  />
                </td>
              </tr>
            )}
          </tbody>
        </QFTable>
      </QFCard>
    </>
  )
}
