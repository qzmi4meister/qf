import { useQuery } from '@tanstack/react-query'
import { fmtDateTime } from '../utils/date'
import { Grid, Card, Title, Text, Group, Badge, Stack, Table, Loader, Center } from '@mantine/core'
import { BarChart } from '@mantine/charts'
import { IconServer, IconShield } from '@tabler/icons-react'
import { listHosts } from '../api/hosts'
import { listPolicies } from '../api/policies'
import { listAuditLog } from '../api/misc'

function statusColor(s: string) {
  return s === 'active' ? 'green' : s === 'offline' ? 'gray' : s === 'error' ? 'red' : 'yellow'
}

export default function Dashboard() {
  const { data: hosts = [], isLoading: hostsLoading } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const { data: policies = [] } = useQuery({
    queryKey: ['policies'],
    queryFn: listPolicies,
    refetchInterval: 30_000,
  })
  const { data: audits = [] } = useQuery({
    queryKey: ['audit-log', { limit: 10 }],
    queryFn: () => listAuditLog({ limit: 10 }),
    refetchInterval: 30_000,
  })

  const statusCounts = hosts.reduce(
    (acc, h) => {
      acc[h.status] = (acc[h.status] ?? 0) + 1
      return acc
    },
    {} as Record<string, number>,
  )

  const chartData = [
    { status: 'Active', count: statusCounts['active'] ?? 0 },
    { status: 'Offline', count: statusCounts['offline'] ?? 0 },
    { status: 'Enrolling', count: statusCounts['enrolling'] ?? 0 },
    { status: 'Error', count: statusCounts['error'] ?? 0 },
  ].filter((d) => d.count > 0)

  if (hostsLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Title order={2}>Dashboard</Title>

      <Grid>
        <Grid.Col span={{ base: 12, sm: 6, md: 3 }}>
          <Card withBorder>
            <Group justify="space-between">
              <div>
                <Text size="xs" c="dimmed">Total hosts</Text>
                <Text size="xl" fw={700}>{hosts.length}</Text>
              </div>
              <IconServer size={32} color="var(--mantine-color-blue-5)" />
            </Group>
            <Group gap={4} mt="xs">
              {Object.entries(statusCounts).map(([s, n]) => (
                <Badge key={s} color={statusColor(s)} size="sm">{n} {s}</Badge>
              ))}
            </Group>
          </Card>
        </Grid.Col>

        <Grid.Col span={{ base: 12, sm: 6, md: 3 }}>
          <Card withBorder>
            <Group justify="space-between">
              <div>
                <Text size="xs" c="dimmed">Policies</Text>
                <Text size="xl" fw={700}>{policies.length}</Text>
              </div>
              <IconShield size={32} color="var(--mantine-color-violet-5)" />
            </Group>
          </Card>
        </Grid.Col>
      </Grid>

      {chartData.length > 0 && (
        <Card withBorder>
          <Title order={4} mb="sm">Host status</Title>
          <BarChart
            h={180}
            data={chartData}
            dataKey="status"
            series={[{ name: 'count', color: 'blue.6' }]}
          />
        </Card>
      )}

      <Card withBorder>
        <Title order={4} mb="sm">Recent audit events</Title>
        <Table highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Time</Table.Th>
              <Table.Th>Actor</Table.Th>
              <Table.Th>Action</Table.Th>
              <Table.Th>Object</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {audits.map((a) => (
              <Table.Tr key={a.id}>
                <Table.Td style={{ whiteSpace: 'nowrap' }}>
                  {fmtDateTime(a.created_at)}
                </Table.Td>
                <Table.Td>{a.actor_type}</Table.Td>
                <Table.Td>
                  <Badge size="sm" variant="light">{a.action}</Badge>
                </Table.Td>
                <Table.Td>{a.object_type}{a.object_id ? ` ${a.object_id.slice(0, 8)}` : ''}</Table.Td>
              </Table.Tr>
            ))}
            {audits.length === 0 && (
              <Table.Tr>
                <Table.Td colSpan={4}>
                  <Text c="dimmed" ta="center" size="sm">No events</Text>
                </Table.Td>
              </Table.Tr>
            )}
          </Table.Tbody>
        </Table>
      </Card>
    </Stack>
  )
}
