import { useState, useMemo } from 'react'
import { fmtDateTime } from '../utils/date'
import { Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, TextInput, Select, Table, Badge, Anchor, Text, Group, Loader, Center,
  ActionIcon, Modal, Button,
} from '@mantine/core'
import { notifications } from '@mantine/notifications'
import {
  createColumnHelper, getCoreRowModel, getFilteredRowModel, getSortedRowModel,
  useReactTable, flexRender, type SortingState,
} from '@tanstack/react-table'
import { useState as useSortingState } from 'react'
import { IconSearch, IconTrash } from '@tabler/icons-react'
import { listHosts, deleteHost } from '../api/hosts'
import type { Host } from '../types'

function statusColor(s: string) {
  return s === 'active' ? 'green' : s === 'offline' ? 'gray' : s === 'error' ? 'red' : 'yellow'
}

const col = createColumnHelper<Host>()

export default function Hosts() {
  const queryClient = useQueryClient()
  const { data: hosts = [], isLoading } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Host | null>(null)
  const [sorting, setSorting] = useSortingState<SortingState>([{ id: 'hostname', desc: false }])

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteHost(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['hosts'] })
      setDeleteTarget(null)
    },
    onError: (err: unknown) => {
      const msg = err instanceof Error ? err.message : 'Delete failed'
      notifications.show({ message: msg, color: 'red' })
      setDeleteTarget(null)
    },
  })

  const columns = useMemo(() => [
    col.accessor('hostname', {
      header: 'Hostname',
      cell: (i) => <Anchor component={Link} to={`/hosts/${i.row.original.id}`}>{i.getValue()}</Anchor>,
    }),
    col.accessor('status', {
      header: 'Status',
      cell: (i) => <Badge color={statusColor(i.getValue())} size="sm">{i.getValue()}</Badge>,
    }),
    col.accessor('agent_version', {
      header: 'Agent',
      cell: (i) => <Text size="sm">{i.getValue() ?? '—'}</Text>,
    }),
    col.accessor('current_generation', {
      header: 'Generation',
      cell: (i) => <Text size="sm">{i.getValue()}</Text>,
    }),
    col.accessor('last_heartbeat_at', {
      header: 'Last seen',
      cell: (i) => {
        const v = i.getValue()
        return <Text size="sm">{fmtDateTime(v)}</Text>
      },
    }),
    col.accessor('labels', {
      header: 'Labels',
      cell: (i) => (
        <Group gap={4}>
          {Object.entries(i.getValue()).map(([k, v]) => (
            <Badge key={k} size="xs" variant="outline">{k}={v}</Badge>
          ))}
        </Group>
      ),
      enableColumnFilter: false,
    }),
    col.display({
      id: 'actions',
      header: '',
      cell: (i) => (
        <ActionIcon
          variant="subtle"
          color="red"
          size="sm"
          onClick={(e) => { e.stopPropagation(); e.preventDefault(); setDeleteTarget(i.row.original) }}
        >
          <IconTrash size={14} />
        </ActionIcon>
      ),
    }),
  // eslint-disable-next-line react-hooks/exhaustive-deps
  ], [])

  const filtered = useMemo(
    () => hosts.filter((h) => {
      const matchSearch = !search || h.hostname.includes(search) ||
        Object.entries(h.labels).some(([k, v]) => `${k}=${v}`.includes(search))
      const matchStatus = !statusFilter || h.status === statusFilter
      return matchSearch && matchStatus
    }),
    [hosts, search, statusFilter],
  )

  const table = useReactTable({
    data: filtered,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
  })

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Modal
        opened={!!deleteTarget}
        onClose={() => setDeleteTarget(null)}
        title="Delete host"
        centered
      >
        <Text size="sm" mb="md">
          Delete <b>{deleteTarget?.hostname}</b>? This cannot be undone.
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={() => setDeleteTarget(null)}>Cancel</Button>
          <Button
            color="red"
            loading={deleteMutation.isPending}
            onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
          >
            Delete
          </Button>
        </Group>
      </Modal>

      <Title order={2}>Hosts</Title>

      <Group>
        <TextInput
          placeholder="Search hostname or label…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={(e) => setSearch(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <Select
          placeholder="Status"
          clearable
          data={['active', 'offline', 'enrolling', 'error']}
          value={statusFilter}
          onChange={setStatusFilter}
          w={140}
        />
      </Group>

      <Table highlightOnHover>
        <Table.Thead>
          {table.getHeaderGroups().map((hg) => (
            <Table.Tr key={hg.id}>
              {hg.headers.map((h) => {
                const canSort = h.column.getCanSort()
                const sorted = h.column.getIsSorted()
                return (
                  <Table.Th
                    key={h.id}
                    onClick={canSort ? h.column.getToggleSortingHandler() : undefined}
                    style={{ cursor: canSort ? 'pointer' : undefined, userSelect: 'none' }}
                  >
                    <Group gap={4} wrap="nowrap">
                      {flexRender(h.column.columnDef.header, h.getContext())}
                      {canSort && (
                        <Text size="xs" c={sorted ? 'blue' : 'dimmed'} style={{ lineHeight: 1 }}>
                          {sorted === 'asc' ? '↑' : sorted === 'desc' ? '↓' : '↕'}
                        </Text>
                      )}
                    </Group>
                  </Table.Th>
                )
              })}
            </Table.Tr>
          ))}
        </Table.Thead>
        <Table.Tbody>
          {table.getRowModel().rows.map((row) => (
            <Table.Tr key={row.id}>
              {row.getVisibleCells().map((cell) => (
                <Table.Td key={cell.id}>{flexRender(cell.column.columnDef.cell, cell.getContext())}</Table.Td>
              ))}
            </Table.Tr>
          ))}
          {table.getRowModel().rows.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={columns.length}>
                <Text c="dimmed" ta="center" size="sm" py="md">No hosts found</Text>
              </Table.Td>
            </Table.Tr>
          )}
        </Table.Tbody>
      </Table>
    </Stack>
  )
}
