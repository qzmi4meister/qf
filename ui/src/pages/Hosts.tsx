import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Stack, Title, TextInput, Select, Table, Badge, Anchor, Text, Group, Loader, Center,
} from '@mantine/core'
import {
  createColumnHelper, getCoreRowModel, getFilteredRowModel, useReactTable, flexRender,
} from '@tanstack/react-table'
import { IconSearch } from '@tabler/icons-react'
import { listHosts } from '../api/hosts'
import type { Host } from '../types'

function statusColor(s: string) {
  return s === 'active' ? 'green' : s === 'offline' ? 'gray' : s === 'error' ? 'red' : 'yellow'
}

const col = createColumnHelper<Host>()
const columns = [
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
      return <Text size="sm">{v ? new Date(v).toLocaleString() : '—'}</Text>
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
]

export default function Hosts() {
  const { data: hosts = [], isLoading } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<string | null>(null)

  const filtered = hosts.filter((h) => {
    const matchSearch = !search || h.hostname.includes(search) ||
      Object.entries(h.labels).some(([k, v]) => `${k}=${v}`.includes(search))
    const matchStatus = !statusFilter || h.status === statusFilter
    return matchSearch && matchStatus
  })

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
  })

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
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
              {hg.headers.map((h) => (
                <Table.Th key={h.id}>{flexRender(h.column.columnDef.header, h.getContext())}</Table.Th>
              ))}
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
