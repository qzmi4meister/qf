import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { Link, useNavigate } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Button, Table, Text, Anchor, Group, TextInput, Badge, ActionIcon,
  Loader, Center, Modal,
} from '@mantine/core'
import { IconSearch, IconPlus, IconTrash } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listPolicies, deletePolicy } from '../api/policies'
import { useSortState } from '../hooks/useSortState'
import { SortTh } from '../components/SortTh'

export default function Policies() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const { sort, toggle, sorted } = useSortState({ key: 'name', dir: 'asc' })

  const { data: policies = [], isLoading } = useQuery({
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

  const filtered = policies.filter((p) =>
    !search || p.name.toLowerCase().includes(search.toLowerCase())
  )

  const rows = sorted(filtered, (p, k) => {
    if (k === 'name') return p.name
    if (k === 'priority') return p.priority
    if (k === 'updated_at') return p.updated_at
    return undefined
  })

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={2}>Policies</Title>
        <Button leftSection={<IconPlus size={14} />} onClick={() => navigate('/policies/new')}>
          New policy
        </Button>
      </Group>

      <TextInput
        placeholder="Search…"
        leftSection={<IconSearch size={14} />}
        value={search}
        onChange={(e) => setSearch(e.currentTarget.value)}
      />

      <Table highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <SortTh sortKey="name" sort={sort} onSort={toggle}>Name</SortTh>
            <SortTh sortKey="priority" sort={sort} onSort={toggle}>Priority</SortTh>
            <Table.Th>Version</Table.Th>
            <SortTh sortKey="updated_at" sort={sort} onSort={toggle}>Updated</SortTh>
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {rows.map((p) => (
            <Table.Tr key={p.id}>
              <Table.Td>
                <Anchor component={Link} to={`/policies/${p.id}`}>{p.name}</Anchor>
              </Table.Td>
              <Table.Td>
                <Badge size="sm" variant="outline">{p.priority}</Badge>
              </Table.Td>
              <Table.Td>v{p.current_version}</Table.Td>
              <Table.Td>{fmtDateTime(p.updated_at)}</Table.Td>
              <Table.Td>
                <ActionIcon
                  color="red"
                  variant="subtle"
                  onClick={() => setDeleteId(p.id)}
                >
                  <IconTrash size={14} />
                </ActionIcon>
              </Table.Td>
            </Table.Tr>
          ))}
          {rows.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={5}>
                <Text c="dimmed" ta="center" size="sm" py="md">No policies</Text>
              </Table.Td>
            </Table.Tr>
          )}
        </Table.Tbody>
      </Table>

      <Modal
        opened={!!deleteId}
        onClose={() => setDeleteId(null)}
        title="Delete policy"
        size="sm"
      >
        <Stack gap="md">
          <Text>Delete this policy? This cannot be undone.</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setDeleteId(null)}>Cancel</Button>
            <Button
              color="red"
              loading={deleteMut.isPending}
              onClick={() => deleteId && deleteMut.mutate(deleteId)}
            >
              Delete
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
