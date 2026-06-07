import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import {
  Stack, Title, Table, Text, Group, Badge, Button, Select, TextInput,
  Code, Loader, Center,
} from '@mantine/core'
import { IconDownload, IconSearch, IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { listAuditLog } from '../api/misc'
import { useSortState } from '../hooks/useSortState'
import { SortTh } from '../components/SortTh'
import { useInfiniteScroll } from '../hooks/useInfiniteScroll'

export default function AuditLog() {
  const [objectTypeFilter, setObjectTypeFilter] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const { sort, toggle, sorted } = useSortState({ key: 'created_at', dir: 'desc' })

  const { data: logs = [], isLoading } = useQuery({
    queryKey: ['audit-log', { objectTypeFilter }],
    queryFn: () => listAuditLog({ limit: 200, object_type: objectTypeFilter ?? undefined }),
  })

  const filtered = sorted(
    logs.filter((l) => {
      return !search ||
        l.actor_id?.includes(search) ||
        l.object_id?.includes(search) ||
        l.action.includes(search)
    }),
    (l, k) => k === 'created_at' ? l.created_at : k === 'action' ? l.action : undefined,
  )

  function toggleExpand(id: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function exportCSV() {
    const header = 'id,time,actor_type,actor_id,action,object_type,object_id\n'
    const rows = filtered.map((l) =>
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

  const objectTypes = [...new Set(logs.map((l) => l.object_type))]
  const { visible, sentinelRef } = useInfiniteScroll(filtered)

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={2}>Audit Log</Title>
        <Button
          size="sm"
          variant="outline"
          leftSection={<IconDownload size={14} />}
          onClick={exportCSV}
        >
          Export CSV
        </Button>
      </Group>

      <Group>
        <Select
          placeholder="Object type"
          clearable
          data={objectTypes}
          value={objectTypeFilter}
          onChange={setObjectTypeFilter}
          w={180}
        />
        <TextInput
          placeholder="Search…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={(e) => setSearch(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
      </Group>

      <Table highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th w={24} />
            <SortTh sortKey="created_at" sort={sort} onSort={toggle}>Time</SortTh>
            <Table.Th>Actor</Table.Th>
            <SortTh sortKey="action" sort={sort} onSort={toggle}>Action</SortTh>
            <Table.Th>Object</Table.Th>
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {visible.map((l) => (
            <>
              <Table.Tr
                key={l.id}
                onClick={() => toggleExpand(l.id)}
                style={{ cursor: 'pointer' }}
              >
                <Table.Td>
                  {expanded.has(l.id) ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />}
                </Table.Td>
                <Table.Td style={{ whiteSpace: 'nowrap', fontSize: 12 }}>
                  {fmtDateTime(l.created_at)}
                </Table.Td>
                <Table.Td>
                  <Text size="sm">{l.actor_type}</Text>
                  {l.actor_id && <Text size="xs" c="dimmed">{l.actor_id.slice(0, 8)}</Text>}
                </Table.Td>
                <Table.Td>
                  <Badge size="sm" variant="light">{l.action}</Badge>
                </Table.Td>
                <Table.Td>
                  <Text size="sm">{l.object_type}</Text>
                  {l.object_id && <Text size="xs" c="dimmed">{l.object_id.slice(0, 8)}</Text>}
                </Table.Td>
              </Table.Tr>
              {expanded.has(l.id) && (
                <Table.Tr key={`${l.id}-detail`}>
                  <Table.Td colSpan={5} style={{ background: 'var(--mantine-color-gray-0)' }}>
                    <Group align="flex-start" gap="md" p="xs">
                      <div style={{ flex: 1 }}>
                        <Text size="xs" fw={600} mb={4}>Before</Text>
                        <Code block style={{ fontSize: 11 }}>
                          {l.before ? JSON.stringify(l.before, null, 2) : 'null'}
                        </Code>
                      </div>
                      <div style={{ flex: 1 }}>
                        <Text size="xs" fw={600} mb={4}>After</Text>
                        <Code block style={{ fontSize: 11 }}>
                          {l.after ? JSON.stringify(l.after, null, 2) : 'null'}
                        </Code>
                      </div>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              )}
            </>
          ))}
          {filtered.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={5}>
                <Text c="dimmed" ta="center" size="sm" py="md">No audit entries</Text>
              </Table.Td>
            </Table.Tr>
          )}
        </Table.Tbody>
      </Table>
      <div ref={sentinelRef} />
    </Stack>
  )
}
