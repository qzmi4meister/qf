import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Tabs, Button, Table, Text, Group, ActionIcon, Modal,
  TextInput, Select, Textarea, Badge, Loader, Center,
} from '@mantine/core'
import { IconPlus, IconTrash, IconEdit } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listObjectGroups, createObjectGroup, updateObjectGroup, deleteObjectGroup } from '../api/objectgroups'
import type { ObjectGroup } from '../types'

const GROUP_TYPES = ['ipset', 'portset', 'hostset']

function defaultSpec(type: string): string {
  if (type === 'ipset') return JSON.stringify({ cidrs: [] }, null, 2)
  if (type === 'portset') return JSON.stringify({ ranges: [] }, null, 2)
  return JSON.stringify({ selector: {} }, null, 2)
}

export default function ObjectGroups() {
  const qc = useQueryClient()
  const [activeTab, setActiveTab] = useState<string | null>('ipset')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ObjectGroup | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const [formName, setFormName] = useState('')
  const [formType, setFormType] = useState('ipset')
  const [formSpec, setFormSpec] = useState(defaultSpec('ipset'))

  const { data: groups = [], isLoading } = useQuery({
    queryKey: ['objectgroups'],
    queryFn: listObjectGroups,
  })

  const saveMut = useMutation({
    mutationFn: async () => {
      let spec: unknown = {}
      try { spec = JSON.parse(formSpec) } catch { /* keep {} */ }
      if (editing) {
        return updateObjectGroup(editing.id, spec)
      }
      return createObjectGroup({ type: formType, name: formName, spec })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['objectgroups'] })
      setModalOpen(false)
      notifications.show({ message: 'Saved', color: 'green' })
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteObjectGroup,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['objectgroups'] })
      setDeleteId(null)
      notifications.show({ message: 'Deleted', color: 'green' })
    },
  })

  function openCreate() {
    setEditing(null)
    setFormName('')
    setFormType(activeTab ?? 'ipset')
    setFormSpec(defaultSpec(activeTab ?? 'ipset'))
    setModalOpen(true)
  }

  function openEdit(g: ObjectGroup) {
    setEditing(g)
    setFormName(g.name)
    setFormType(g.type)
    setFormSpec(JSON.stringify(g.spec, null, 2))
    setModalOpen(true)
  }

  const filtered = groups.filter((g) => g.type === activeTab)

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={2}>Object Groups</Title>
        <Button leftSection={<IconPlus size={14} />} onClick={openCreate}>New group</Button>
      </Group>

      <Tabs value={activeTab} onChange={setActiveTab}>
        <Tabs.List>
          {GROUP_TYPES.map((t) => (
            <Tabs.Tab key={t} value={t}>
              {t.charAt(0).toUpperCase() + t.slice(1)}
              <Badge ml="xs" size="xs" variant="outline">
                {groups.filter((g) => g.type === t).length}
              </Badge>
            </Tabs.Tab>
          ))}
        </Tabs.List>

        <Tabs.Panel value={activeTab ?? 'ipset'} pt="md">
          <Table highlightOnHover>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Spec</Table.Th>
                <Table.Th>Updated</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {filtered.map((g) => (
                <Table.Tr key={g.id}>
                  <Table.Td>{g.name}</Table.Td>
                  <Table.Td>
                    <Text size="xs" style={{ fontFamily: 'monospace' }}>
                      {JSON.stringify(g.spec).slice(0, 60)}…
                    </Text>
                  </Table.Td>
                  <Table.Td>{new Date(g.updated_at).toLocaleString()}</Table.Td>
                  <Table.Td>
                    <Group gap={4}>
                      <ActionIcon size="sm" variant="subtle" onClick={() => openEdit(g)}>
                        <IconEdit size={12} />
                      </ActionIcon>
                      <ActionIcon size="sm" variant="subtle" color="red" onClick={() => setDeleteId(g.id)}>
                        <IconTrash size={12} />
                      </ActionIcon>
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
              {filtered.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={4}>
                    <Text c="dimmed" ta="center" size="sm" py="md">No {activeTab} groups</Text>
                  </Table.Td>
                </Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Tabs.Panel>
      </Tabs>

      <Modal
        opened={modalOpen}
        onClose={() => setModalOpen(false)}
        title={editing ? 'Edit group' : 'New group'}
      >
        <Stack gap="sm">
          {!editing && (
            <>
              <TextInput
                label="Name"
                value={formName}
                onChange={(e) => setFormName(e.currentTarget.value)}
                required
              />
              <Select
                label="Type"
                data={GROUP_TYPES}
                value={formType}
                onChange={(v) => {
                  setFormType(v ?? 'ipset')
                  setFormSpec(defaultSpec(v ?? 'ipset'))
                }}
              />
            </>
          )}
          <Textarea
            label="Spec (JSON)"
            value={formSpec}
            onChange={(e) => setFormSpec(e.currentTarget.value)}
            rows={6}
            styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
          />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button loading={saveMut.isPending} onClick={() => saveMut.mutate()}>Save</Button>
          </Group>
        </Stack>
      </Modal>

      <Modal opened={!!deleteId} onClose={() => setDeleteId(null)} title="Delete group" size="sm">
        <Stack gap="md">
          <Text>Delete this group?</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setDeleteId(null)}>Cancel</Button>
            <Button color="red" loading={deleteMut.isPending} onClick={() => deleteId && deleteMut.mutate(deleteId)}>
              Delete
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
