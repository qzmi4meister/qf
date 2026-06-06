import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import {
  Stack, Title, Group, Select, Table, Text, TextInput, Loader, Center,
} from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { listHosts, listFlows } from '../api/hosts'

export default function Flows() {
  const [hostId, setHostId] = useState<string | null>(null)
  const [search, setSearch] = useState('')

  const { data: hosts = [] } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
  })

  const { data: flows = [], isLoading } = useQuery({
    queryKey: ['flows-all', hostId],
    queryFn: () => listFlows(hostId!, { limit: 200 }),
    enabled: !!hostId,
  })

  const filtered = flows.filter((f) => {
    return !search ||
      (f.src_ip ?? '').includes(search) ||
      (f.dst_ip ?? '').includes(search)
  })

  return (
    <Stack gap="md">
      <Title order={2}>Flow Explorer</Title>

      <Group>
        <Select
          placeholder="Select host"
          data={hosts.map((h) => ({ value: h.id, label: h.hostname }))}
          value={hostId}
          onChange={setHostId}
          searchable
          style={{ flex: 1 }}
        />
        <TextInput
          placeholder="Filter by IP…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={(e) => setSearch(e.currentTarget.value)}
          w={200}
        />
      </Group>

      {!hostId && <Text c="dimmed" ta="center" py="xl">Select a host</Text>}

      {hostId && isLoading && <Center h={200}><Loader /></Center>}

      {hostId && !isLoading && (
        <Table highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Proto</Table.Th>
              <Table.Th>Src</Table.Th>
              <Table.Th>Dst</Table.Th>
              <Table.Th>Bytes↑</Table.Th>
              <Table.Th>Bytes↓</Table.Th>
              <Table.Th>Pkts↑</Table.Th>
              <Table.Th>Pkts↓</Table.Th>
              <Table.Th>State</Table.Th>
              <Table.Th>Started</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {filtered.map((f) => (
              <Table.Tr key={f.id}>
                <Table.Td>{f.protocol}</Table.Td>
                <Table.Td style={{ fontSize: 12 }}>{f.src_ip ?? '—'}{f.src_port ? `:${f.src_port}` : ''}</Table.Td>
                <Table.Td style={{ fontSize: 12 }}>{f.dst_ip ?? '—'}{f.dst_port ? `:${f.dst_port}` : ''}</Table.Td>
                <Table.Td>{f.bytes_orig}</Table.Td>
                <Table.Td>{f.bytes_reply}</Table.Td>
                <Table.Td>{f.packets_orig}</Table.Td>
                <Table.Td>{f.packets_reply}</Table.Td>
                <Table.Td>{f.final_state ?? '—'}</Table.Td>
                <Table.Td style={{ fontSize: 12 }}>{fmtDateTime(f.started_at)}</Table.Td>
              </Table.Tr>
            ))}
            {filtered.length === 0 && (
              <Table.Tr>
                <Table.Td colSpan={9}>
                  <Text c="dimmed" ta="center" size="sm" py="md">No flows</Text>
                </Table.Td>
              </Table.Tr>
            )}
          </Table.Tbody>
        </Table>
      )}
    </Stack>
  )
}
