import { useState, useEffect, useRef } from 'react'
import { fmtTime } from '../utils/date'
import { useQuery } from '@tanstack/react-query'
import {
  Stack, Title, Group, Select, Badge, Table, Text, Button, Switch, ScrollArea,
  TextInput,
} from '@mantine/core'
import { IconSearch } from '@tabler/icons-react'
import { listHosts } from '../api/hosts'
import { listEvents } from '../api/hosts'
import type { LogEvent } from '../types'

function actionColor(a: string) {
  return a === 'deny' ? 'red' : a === 'allow' ? 'green' : 'blue'
}

function protoName(p: number): string {
  const names: Record<number, string> = { 1: 'any', 2: 'TCP', 3: 'UDP', 4: 'ICMP', 5: 'ICMPv6' }
  return names[p] ?? String(p)
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

  const { data: hosts = [] } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
  })

  const { data: historyEvents = [] } = useQuery({
    queryKey: ['events-history', hostId, actionFilter],
    queryFn: () => listEvents(hostId!, { limit: 200, action: actionFilter ?? undefined }),
    enabled: !live && !!hostId,
  })

  // SSE subscription
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
        setLiveEvents((prev) => {
          const next = [...prev, event].slice(-500) // keep last 500
          return next
        })
      } catch { /* noop */ }
    })

    es.onerror = () => {
      // SSE auto-reconnects; no action needed
    }

    return () => {
      es.close()
    }
  }, [live, hostId])

  // Auto-scroll
  useEffect(() => {
    if (!paused && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [liveEvents, paused])

  const displayEvents = live ? liveEvents : historyEvents

  const filtered = displayEvents.filter((e) => {
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
    <Stack gap="md">
      <Title order={2}>Events</Title>

      <Group>
        <Select
          placeholder="Select host"
          data={hosts.map((h) => ({ value: h.id, label: h.hostname }))}
          value={hostId}
          onChange={setHostId}
          searchable
          style={{ flex: 1 }}
        />
        <Select
          placeholder="Action"
          clearable
          data={['allow', 'deny', 'log']}
          value={actionFilter}
          onChange={setActionFilter}
          w={120}
        />
        <TextInput
          placeholder="IP, port or rule ID…"
          leftSection={<IconSearch size={14} />}
          value={search}
          onChange={(e) => setSearch(e.currentTarget.value)}
          w={200}
        />
        <Switch
          label="Live"
          checked={live}
          onChange={(e) => {
            setLive(e.currentTarget.checked)
            if (e.currentTarget.checked) setLiveEvents([])
          }}
        />
        {live && (
          <Button size="xs" variant="outline" onClick={() => setPaused((p) => !p)}>
            {paused ? 'Resume' : 'Pause'}
          </Button>
        )}
        {live && (
          <Button size="xs" variant="subtle" color="red" onClick={() => setLiveEvents([])}>
            Clear
          </Button>
        )}
      </Group>

      {!hostId && (
        <Text c="dimmed" ta="center" py="xl">Select a host to view events</Text>
      )}

      {hostId && (
        <ScrollArea
          h={560}
          viewportRef={scrollRef}
          onMouseEnter={() => setPaused(true)}
          onMouseLeave={() => setPaused(false)}
        >
          <Table highlightOnHover>
            <Table.Thead style={{ position: 'sticky', top: 0, background: 'white', zIndex: 1 }}>
              <Table.Tr>
                <Table.Th>Time</Table.Th>
                <Table.Th>Action</Table.Th>
                <Table.Th>Dir</Table.Th>
                <Table.Th>Proto</Table.Th>
                <Table.Th>Src</Table.Th>
                <Table.Th>Dst</Table.Th>
                <Table.Th>Rule</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {filtered.map((e) => (
                <Table.Tr key={e.id}>
                  <Table.Td style={{ whiteSpace: 'nowrap', fontSize: 12 }}>
                    {fmtTime(e.created_at)}
                  </Table.Td>
                  <Table.Td>
                    <Badge size="xs" color={actionColor(e.action)}>{e.action}</Badge>
                  </Table.Td>
                  <Table.Td style={{ fontSize: 12 }}>{e.direction}</Table.Td>
                  <Table.Td style={{ fontSize: 12 }}>{protoName(e.protocol)}</Table.Td>
                  <Table.Td style={{ fontSize: 12 }}>
                    {e.src_ip ?? '—'}{e.src_port ? `:${e.src_port}` : ''}
                  </Table.Td>
                  <Table.Td style={{ fontSize: 12 }}>
                    {e.dst_ip ?? '—'}{e.dst_port ? `:${e.dst_port}` : ''}
                  </Table.Td>
                  <Table.Td style={{ fontSize: 12, fontFamily: 'monospace' }}>
                    {e.rule_id ? e.rule_id.slice(0, 8) : '—'}
                  </Table.Td>
                </Table.Tr>
              ))}
              {filtered.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={7}>
                    <Text c="dimmed" ta="center" size="sm" py="md">
                      {live ? 'Waiting for events…' : 'No events'}
                    </Text>
                  </Table.Td>
                </Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </ScrollArea>
      )}
    </Stack>
  )
}
