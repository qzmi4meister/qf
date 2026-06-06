import { useState } from 'react'
import { fmtDateTime, fmtTime } from '../utils/date'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Stack, Title, Tabs, Badge, Group, Text, Table, TextInput, Loader, Center, Anchor,
  Card, Grid, Select as MSelect, Button, Switch,
} from '@mantine/core'
import { IconSearch, IconDownload } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useMutation } from '@tanstack/react-query'
import { getHost, listEvents, listFlows, latestCounters, getHostRuleset, patchHost } from '../api/hosts'
import type { LogEvent, RulesetRuleItem } from '../types'

function statusColor(s: string) {
  return s === 'active' ? 'green' : s === 'offline' ? 'gray' : s === 'error' ? 'red' : 'yellow'
}

export default function HostDetail() {
  const { id } = useParams<{ id: string }>()
  const [search, setSearch] = useState('')
  const [actionFilter, setActionFilter] = useState<string | null>(null)

  const queryClient = useQueryClient()
  const { data: host, isLoading } = useQuery({
    queryKey: ['host', id],
    queryFn: () => getHost(id!),
    enabled: !!id,
    refetchInterval: 15_000,
  })
  const flowMut = useMutation({
    mutationFn: (enabled: boolean) => patchHost(id!, { flow_events_enabled: enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['host', id] }),
  })
  const { data: events = [] } = useQuery({
    queryKey: ['events', id],
    queryFn: () => listEvents(id!, { limit: 100 }),
    enabled: !!id,
    refetchInterval: 10_000,
  })
  const { data: flows = [] } = useQuery({
    queryKey: ['flows', id],
    queryFn: () => listFlows(id!, { limit: 100 }),
    enabled: !!id,
  })
  const { data: counters = [] } = useQuery({
    queryKey: ['counters', id],
    queryFn: () => latestCounters(id!),
    enabled: !!id,
    refetchInterval: 30_000,
  })
  const { data: ruleset, isLoading: rulesetLoading } = useQuery({
    queryKey: ['ruleset', id],
    queryFn: () => getHostRuleset(id!),
    enabled: !!id,
  })

  if (isLoading) return <Center h={200}><Loader /></Center>
  if (!host) return <Text c="red">Host not found</Text>

  const filteredEvents = events.filter((e) => {
    const matchAction = !actionFilter || e.action === actionFilter
    const matchSearch = !search ||
      (e.src_ip ?? '').includes(search) ||
      (e.dst_ip ?? '').includes(search) ||
      (e.rule_id ?? '').includes(search)
    return matchAction && matchSearch
  })

  return (
    <Stack gap="md">
      <Group>
        <Anchor component={Link} to="/hosts">Hosts</Anchor>
        <Text c="dimmed">/</Text>
        <Title order={2}>{host.hostname}</Title>
        <Badge color={statusColor(host.status)}>{host.status}</Badge>
      </Group>

      <Tabs defaultValue="overview">
        <Tabs.List>
          <Tabs.Tab value="overview">Overview</Tabs.Tab>
          <Tabs.Tab value="ruleset">Ruleset</Tabs.Tab>
          <Tabs.Tab value="events">Events</Tabs.Tab>
          <Tabs.Tab value="flows">Flows</Tabs.Tab>
          <Tabs.Tab value="counters">Counters</Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="overview" pt="md">
          <Grid>
            <Grid.Col span={{ base: 12, sm: 6 }}>
              <Card withBorder>
                <Stack gap="xs">
                  <Text fw={600}>Agent info</Text>
                  <Group justify="space-between">
                    <Text size="sm" c="dimmed">Version</Text>
                    <Text size="sm">{host.agent_version ?? '—'}</Text>
                  </Group>
                  <Group justify="space-between">
                    <Text size="sm" c="dimmed">Kernel</Text>
                    <Text size="sm">{host.kernel_version ?? '—'}</Text>
                  </Group>
                  <Group justify="space-between">
                    <Text size="sm" c="dimmed">Generation</Text>
                    <Text size="sm">{host.current_generation}</Text>
                  </Group>
                  <Group justify="space-between">
                    <Text size="sm" c="dimmed">Last heartbeat</Text>
                    <Text size="sm">{fmtDateTime(host.last_heartbeat_at)}</Text>
                  </Group>
                  <Group justify="space-between">
                    <Text size="sm" c="dimmed">Flow events</Text>
                    <Switch
                      size="sm"
                      checked={host.flow_events_enabled}
                      onChange={(e) => flowMut.mutate(e.currentTarget.checked)}
                      disabled={flowMut.isPending}
                    />
                  </Group>
                </Stack>
              </Card>
            </Grid.Col>
            <Grid.Col span={{ base: 12, sm: 6 }}>
              <Card withBorder>
                <Stack gap="xs">
                  <Text fw={600}>Labels</Text>
                  {Object.entries(host.labels).length === 0 ? (
                    <Text size="sm" c="dimmed">No labels</Text>
                  ) : (
                    Object.entries(host.labels).map(([k, v]) => (
                      <Group key={k} justify="space-between">
                        <Text size="sm" c="dimmed">{k}</Text>
                        <Text size="sm">{v}</Text>
                      </Group>
                    ))
                  )}
                </Stack>
              </Card>
            </Grid.Col>
          </Grid>
        </Tabs.Panel>

        <Tabs.Panel value="ruleset" pt="md">
          {rulesetLoading ? (
            <Center h={100}><Loader /></Center>
          ) : !ruleset ? (
            <Text c="dimmed">No ruleset data</Text>
          ) : (
            <Stack gap="sm">
              <Group justify="space-between">
                <Group gap="xs">
                  <Text size="sm" c="dimmed">Default ingress:</Text>
                  <Badge color={ruleset.default_ingress === 'allow' ? 'green' : 'red'} size="sm">{ruleset.default_ingress}</Badge>
                  <Text size="sm" c="dimmed" ml="md">Default egress:</Text>
                  <Badge color={ruleset.default_egress === 'allow' ? 'green' : 'red'} size="sm">{ruleset.default_egress}</Badge>
                </Group>
                <Button
                  size="xs"
                  variant="light"
                  leftSection={<IconDownload size={14} />}
                  onClick={() => downloadRuleset(host.hostname, ruleset.rules)}
                >
                  Download JSON
                </Button>
              </Group>
              <RulesetTable rules={ruleset.rules} />
            </Stack>
          )}
        </Tabs.Panel>

        <Tabs.Panel value="events" pt="md">
          <Stack gap="sm">
            <Group>
              <TextInput
                placeholder="Search IP or rule ID…"
                leftSection={<IconSearch size={14} />}
                value={search}
                onChange={(e) => setSearch(e.currentTarget.value)}
                style={{ flex: 1 }}
              />
              <MSelect
                placeholder="Action"
                clearable
                data={['allow', 'deny', 'log']}
                value={actionFilter}
                onChange={setActionFilter}
                w={120}
              />
            </Group>
            <EventsTable events={filteredEvents} />
          </Stack>
        </Tabs.Panel>

        <Tabs.Panel value="flows" pt="md">
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Protocol</Table.Th>
                <Table.Th>Src</Table.Th>
                <Table.Th>Dst</Table.Th>
                <Table.Th>Bytes↑</Table.Th>
                <Table.Th>Bytes↓</Table.Th>
                <Table.Th>State</Table.Th>
                <Table.Th>Time</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {flows.map((f) => (
                <Table.Tr key={f.id}>
                  <Table.Td>{f.protocol}</Table.Td>
                  <Table.Td>{f.src_ip ?? '—'}{f.src_port ? `:${f.src_port}` : ''}</Table.Td>
                  <Table.Td>{f.dst_ip ?? '—'}{f.dst_port ? `:${f.dst_port}` : ''}</Table.Td>
                  <Table.Td>{f.bytes_orig}</Table.Td>
                  <Table.Td>{f.bytes_reply}</Table.Td>
                  <Table.Td>{f.final_state ?? '—'}</Table.Td>
                  <Table.Td>{fmtTime(f.created_at)}</Table.Td>
                </Table.Tr>
              ))}
              {flows.length === 0 && (
                <Table.Tr><Table.Td colSpan={7}><Text c="dimmed" ta="center" size="sm" py="md">No flows</Text></Table.Td></Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Tabs.Panel>

        <Tabs.Panel value="counters" pt="md">
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Rule ID</Table.Th>
                <Table.Th>Packets</Table.Th>
                <Table.Th>Bytes</Table.Th>
                <Table.Th>Timestamp</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {counters.map((c) => (
                <Table.Tr key={c.id}>
                  <Table.Td style={{ fontFamily: 'monospace', fontSize: 12 }}>{c.rule_id.slice(0, 8)}</Table.Td>
                  <Table.Td>{c.packets}</Table.Td>
                  <Table.Td>{c.bytes}</Table.Td>
                  <Table.Td>{fmtDateTime(c.ts)}</Table.Td>
                </Table.Tr>
              ))}
              {counters.length === 0 && (
                <Table.Tr><Table.Td colSpan={4}><Text c="dimmed" ta="center" size="sm" py="md">No counters</Text></Table.Td></Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Tabs.Panel>
      </Tabs>
    </Stack>
  )
}

function downloadRuleset(hostname: string, rules: RulesetRuleItem[]) {
  const blob = new Blob([JSON.stringify(rules, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `${hostname}-ruleset.json`
  a.click()
  URL.revokeObjectURL(url)
}

function actionColor(a: string) {
  return a === 'deny' ? 'red' : a === 'allow' ? 'green' : 'blue'
}

function RulesetTable({ rules }: { rules: RulesetRuleItem[] }) {
  return (
    <Table highlightOnHover style={{ fontSize: 13 }}>
      <Table.Thead>
        <Table.Tr>
          <Table.Th>#</Table.Th>
          <Table.Th>Rule</Table.Th>
          <Table.Th>Policy</Table.Th>
          <Table.Th>Dir</Table.Th>
          <Table.Th>Action</Table.Th>
          <Table.Th>Proto</Table.Th>
          <Table.Th>Src CIDRs</Table.Th>
          <Table.Th>Dst CIDRs</Table.Th>
          <Table.Th>Src Ports</Table.Th>
          <Table.Th>Dst Ports</Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {rules.map((r, i) => (
          <Table.Tr key={r.rule_id}>
            <Table.Td style={{ color: 'var(--mantine-color-dimmed)', width: 30 }}>{i + 1}</Table.Td>
            <Table.Td style={{ fontFamily: 'monospace', fontSize: 12 }} title={r.rule_id}>{r.rule_name || r.rule_id.slice(0, 8)}</Table.Td>
            <Table.Td>{r.policy_name}</Table.Td>
            <Table.Td>{r.direction}</Table.Td>
            <Table.Td><Badge color={actionColor(r.action)} size="sm">{r.action}</Badge></Table.Td>
            <Table.Td>{r.protocol || 'any'}</Table.Td>
            <Table.Td style={{ fontFamily: 'monospace', fontSize: 11 }}>{r.src_cidrs?.join(', ') || '—'}</Table.Td>
            <Table.Td style={{ fontFamily: 'monospace', fontSize: 11 }}>{r.dst_cidrs?.join(', ') || '—'}</Table.Td>
            <Table.Td style={{ fontFamily: 'monospace', fontSize: 11 }}>{r.src_ports?.join(', ') || '—'}</Table.Td>
            <Table.Td style={{ fontFamily: 'monospace', fontSize: 11 }}>{r.dst_ports?.join(', ') || '—'}</Table.Td>
          </Table.Tr>
        ))}
        {rules.length === 0 && (
          <Table.Tr><Table.Td colSpan={10}><Text c="dimmed" ta="center" size="sm" py="md">No rules applied to this host</Text></Table.Td></Table.Tr>
        )}
      </Table.Tbody>
    </Table>
  )
}

function EventsTable({ events }: { events: LogEvent[] }) {
  return (
    <Table highlightOnHover>
      <Table.Thead>
        <Table.Tr>
          <Table.Th>Time</Table.Th>
          <Table.Th>Action</Table.Th>
          <Table.Th>Dir</Table.Th>
          <Table.Th>Proto</Table.Th>
          <Table.Th>Src</Table.Th>
          <Table.Th>Dst</Table.Th>
        </Table.Tr>
      </Table.Thead>
      <Table.Tbody>
        {events.map((e) => (
          <Table.Tr key={e.id}>
            <Table.Td style={{ whiteSpace: 'nowrap' }}>{fmtTime(e.created_at)}</Table.Td>
            <Table.Td><Badge color={actionColor(e.action)} size="sm">{e.action}</Badge></Table.Td>
            <Table.Td>{e.direction}</Table.Td>
            <Table.Td>{e.protocol}</Table.Td>
            <Table.Td>{e.src_ip ?? '—'}{e.src_port ? `:${e.src_port}` : ''}</Table.Td>
            <Table.Td>{e.dst_ip ?? '—'}{e.dst_port ? `:${e.dst_port}` : ''}</Table.Td>
          </Table.Tr>
        ))}
        {events.length === 0 && (
          <Table.Tr><Table.Td colSpan={6}><Text c="dimmed" ta="center" size="sm" py="md">No events</Text></Table.Td></Table.Tr>
        )}
      </Table.Tbody>
    </Table>
  )
}
