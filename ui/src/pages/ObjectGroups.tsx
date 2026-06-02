import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Tabs, Button, Table, Text, Group, ActionIcon, Modal,
  TextInput, Select, Badge, Loader, Center,
} from '@mantine/core'
import { IconPlus, IconTrash, IconEdit, IconX } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listObjectGroups, createObjectGroup, updateObjectGroup, deleteObjectGroup } from '../api/objectgroups'
import type { ObjectGroup } from '../types'

const GROUP_TYPES = ['ipset', 'portset', 'hostset']

// ── sub-editors ────────────────────────────────────────────────────────────

function StringListEditor({
  value, onChange, placeholder, addLabel,
}: {
  value: string[]
  onChange: (v: string[]) => void
  placeholder: string
  addLabel: string
}) {
  return (
    <Stack gap="xs">
      {value.map((item, i) => (
        <Group key={i} gap="xs">
          <TextInput
            placeholder={placeholder}
            value={item}
            onChange={(e) => { const n = [...value]; n[i] = e.currentTarget.value; onChange(n) }}
            style={{ flex: 1 }}
          />
          <ActionIcon color="red" variant="subtle" size="sm" onClick={() => onChange(value.filter((_, j) => j !== i))}>
            <IconX size={14} />
          </ActionIcon>
        </Group>
      ))}
      <Button size="xs" variant="subtle" leftSection={<IconPlus size={12} />} onClick={() => onChange([...value, ''])}>
        {addLabel}
      </Button>
    </Stack>
  )
}

interface KVPair { key: string; val: string }

function LabelSelector({ value, onChange }: { value: KVPair[]; onChange: (v: KVPair[]) => void }) {
  return (
    <Stack gap="xs">
      {value.map((pair, i) => (
        <Group key={i} gap="xs">
          <TextInput
            placeholder="key"
            value={pair.key}
            onChange={(e) => { const n = [...value]; n[i] = { ...pair, key: e.currentTarget.value }; onChange(n) }}
            style={{ flex: 1 }}
          />
          <TextInput
            placeholder="value"
            value={pair.val}
            onChange={(e) => { const n = [...value]; n[i] = { ...pair, val: e.currentTarget.value }; onChange(n) }}
            style={{ flex: 1 }}
          />
          <ActionIcon color="red" variant="subtle" size="sm" onClick={() => onChange(value.filter((_, j) => j !== i))}>
            <IconX size={14} />
          </ActionIcon>
        </Group>
      ))}
      <Button size="xs" variant="subtle" leftSection={<IconPlus size={12} />} onClick={() => onChange([...value, { key: '', val: '' }])}>
        Add label
      </Button>
    </Stack>
  )
}

function SpecSummary({ group }: { group: ObjectGroup }) {
  const spec = group.spec as Record<string, unknown>
  if (group.type === 'ipset') {
    const cidrs = (spec?.cidrs ?? []) as string[]
    if (!cidrs.length) return <Text size="xs" c="dimmed">empty</Text>
    return (
      <Group gap={4}>
        {cidrs.slice(0, 3).map((c) => <Badge key={c} size="xs" variant="outline">{c}</Badge>)}
        {cidrs.length > 3 && <Text size="xs" c="dimmed">+{cidrs.length - 3}</Text>}
      </Group>
    )
  }
  if (group.type === 'portset') {
    const ranges = (spec?.ranges ?? []) as string[]
    if (!ranges.length) return <Text size="xs" c="dimmed">empty</Text>
    return (
      <Group gap={4}>
        {ranges.slice(0, 4).map((r) => <Badge key={r} size="xs" variant="outline">{r}</Badge>)}
        {ranges.length > 4 && <Text size="xs" c="dimmed">+{ranges.length - 4}</Text>}
      </Group>
    )
  }
  if (group.type === 'hostset') {
    const sel = (spec?.selector ?? {}) as Record<string, string>
    const entries = Object.entries(sel)
    if (!entries.length) return <Text size="xs" c="dimmed">any host</Text>
    return (
      <Group gap={4}>
        {entries.slice(0, 3).map(([k, v]) => <Badge key={k} size="xs">{k}={v}</Badge>)}
        {entries.length > 3 && <Text size="xs" c="dimmed">+{entries.length - 3}</Text>}
      </Group>
    )
  }
  return null
}

// ── main page ──────────────────────────────────────────────────────────────

export default function ObjectGroups() {
  const qc = useQueryClient()
  const [activeTab, setActiveTab] = useState<string | null>('ipset')
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ObjectGroup | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const [formName, setFormName] = useState('')
  const [formType, setFormType] = useState('ipset')
  const [formCidrs, setFormCidrs] = useState<string[]>([])
  const [formRanges, setFormRanges] = useState<string[]>([])
  const [formLabels, setFormLabels] = useState<KVPair[]>([])

  const { data: groups = [], isLoading } = useQuery({
    queryKey: ['objectgroups'],
    queryFn: listObjectGroups,
  })

  function buildSpec() {
    if (formType === 'ipset') return { cidrs: formCidrs.filter(Boolean) }
    if (formType === 'portset') return { ranges: formRanges.filter(Boolean) }
    return { selector: Object.fromEntries(formLabels.filter(p => p.key).map(p => [p.key, p.val])) }
  }

  const saveMut = useMutation({
    mutationFn: async () => {
      const spec = buildSpec()
      if (editing) return updateObjectGroup(editing.id, spec)
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

  function initForm(type: string, spec?: unknown) {
    const s = (spec ?? {}) as Record<string, unknown>
    setFormCidrs(type === 'ipset' ? ((s.cidrs ?? []) as string[]) : [])
    setFormRanges(type === 'portset' ? ((s.ranges ?? []) as string[]) : [])
    const sel = (s.selector ?? {}) as Record<string, string>
    setFormLabels(type === 'hostset' ? Object.entries(sel).map(([key, val]) => ({ key, val })) : [])
  }

  function openCreate() {
    setEditing(null)
    setFormName('')
    const t = activeTab ?? 'ipset'
    setFormType(t)
    initForm(t)
    setModalOpen(true)
  }

  function openEdit(g: ObjectGroup) {
    setEditing(g)
    setFormName(g.name)
    setFormType(g.type)
    initForm(g.type, g.spec)
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
                <Table.Th>Content</Table.Th>
                <Table.Th>Updated</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {filtered.map((g) => (
                <Table.Tr key={g.id}>
                  <Table.Td>{g.name}</Table.Td>
                  <Table.Td><SpecSummary group={g} /></Table.Td>
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
        title={editing ? `Edit ${editing.type}` : 'New group'}
      >
        <Stack gap="sm">
          {!editing && (
            <>
              <TextInput label="Name" value={formName} onChange={(e) => setFormName(e.currentTarget.value)} required />
              <Select
                label="Type"
                data={[
                  { value: 'ipset', label: 'IP set (CIDRs)' },
                  { value: 'portset', label: 'Port set (ports / ranges)' },
                  { value: 'hostset', label: 'Host set (label selector)' },
                ]}
                value={formType}
                onChange={(v) => { const t = v ?? 'ipset'; setFormType(t); initForm(t) }}
              />
            </>
          )}

          {formType === 'ipset' && (
            <div>
              <Text size="sm" fw={500} mb={4}>CIDRs</Text>
              <StringListEditor value={formCidrs} onChange={setFormCidrs} placeholder="192.168.0.0/24" addLabel="Add CIDR" />
            </div>
          )}
          {formType === 'portset' && (
            <div>
              <Text size="sm" fw={500} mb={4}>Ports / ranges</Text>
              <StringListEditor value={formRanges} onChange={setFormRanges} placeholder="80  or  8000-9000" addLabel="Add port / range" />
            </div>
          )}
          {formType === 'hostset' && (
            <div>
              <Text size="sm" fw={500} mb={4}>Host labels</Text>
              <LabelSelector value={formLabels} onChange={setFormLabels} />
            </div>
          )}

          <Group justify="flex-end" mt="xs">
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
            <Button color="red" loading={deleteMut.isPending} onClick={() => deleteId && deleteMut.mutate(deleteId)}>Delete</Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
