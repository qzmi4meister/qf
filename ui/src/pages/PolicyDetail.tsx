import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Tabs, Button, TextInput, Textarea, NumberInput, Table, Badge,
  Group, Text, Anchor, ActionIcon, Modal, Select, Checkbox, Loader, Center,
  Accordion, Code, Paper, Alert,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconPlayerPlay, IconHistory, IconArrowBack,
  IconAlertCircle,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { getPolicy, updatePolicy, createPolicy, previewPolicy, listVersions, revertVersion } from '../api/policies'
import type { Rule, PreviewResult } from '../types'

const EMPTY_RULE = (): Omit<Rule, 'id' | 'policy_id' | 'created_at' | 'updated_at'> => ({
  name: '',
  priority: 100,
  direction: 'ingress',
  match: {},
  action: 'allow',
  log: false,
  silent: false,
})

export default function PolicyDetail() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isNew = id === 'new'

  const { data: policy, isLoading } = useQuery({
    queryKey: ['policy', id],
    queryFn: () => getPolicy(id!),
    enabled: !isNew && !!id,
  })

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [priority, setPriority] = useState<number>(100)
  const [selectorRaw, setSelectorRaw] = useState('{}')
  const [rules, setRules] = useState<Omit<Rule, 'id' | 'policy_id' | 'created_at' | 'updated_at'>[]>([])
  const [editRuleIdx, setEditRuleIdx] = useState<number | null>(null)
  const [previewResult, setPreviewResult] = useState<PreviewResult | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [previewLoading, setPreviewLoading] = useState(false)

  useEffect(() => {
    if (policy) {
      setName(policy.name)
      setDescription(policy.description)
      setPriority(policy.priority)
      setSelectorRaw(JSON.stringify(policy.selector ?? {}, null, 2))
      setRules(policy.rules.map(({ id: _id, policy_id: _pid, created_at: _c, updated_at: _u, ...r }) => r))
    }
  }, [policy])

  const saveMut = useMutation({
    mutationFn: async () => {
      let selector: unknown = {}
      try { selector = JSON.parse(selectorRaw) } catch { /* keep {} */ }
      if (isNew) {
        const p = await createPolicy({ name, description, priority, selector })
        await updatePolicy(p.id, { name, description, priority, selector, rules })
        return p.id
      } else {
        await updatePolicy(id!, { name, description, priority, selector, rules })
        return id!
      }
    },
    onSuccess: (savedId) => {
      qc.invalidateQueries({ queryKey: ['policies'] })
      qc.invalidateQueries({ queryKey: ['policy', savedId] })
      notifications.show({ message: 'Policy saved', color: 'green' })
      if (isNew) navigate(`/app/policies/${savedId}`)
    },
    onError: () => notifications.show({ message: 'Save failed', color: 'red' }),
  })

  const { data: versions = [] } = useQuery({
    queryKey: ['policy-versions', id],
    queryFn: () => listVersions(id!),
    enabled: !isNew && !!id,
  })

  const revertMut = useMutation({
    mutationFn: (v: number) => revertVersion(id!, v),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['policy', id] })
      qc.invalidateQueries({ queryKey: ['policy-versions', id] })
      notifications.show({ message: 'Reverted', color: 'green' })
    },
  })

  async function handlePreview() {
    if (!id || isNew) return
    setPreviewLoading(true)
    setPreviewOpen(true)
    try {
      const result = await previewPolicy(id, rules)
      setPreviewResult(result)
    } catch {
      setPreviewResult(null)
    } finally {
      setPreviewLoading(false)
    }
  }

  function addRule() {
    setRules([...rules, EMPTY_RULE()])
    setEditRuleIdx(rules.length)
  }

  function removeRule(i: number) {
    setRules(rules.filter((_, idx) => idx !== i))
  }

  function updateRule(i: number, patch: Partial<typeof rules[0]>) {
    setRules(rules.map((r, idx) => idx === i ? { ...r, ...patch } : r))
  }

  if (!isNew && isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group>
        <Anchor component={Link} to="/app/policies">Policies</Anchor>
        <Text c="dimmed">/</Text>
        <Title order={2}>{isNew ? 'New policy' : (policy?.name ?? '…')}</Title>
      </Group>

      <Tabs defaultValue="edit">
        <Tabs.List>
          <Tabs.Tab value="edit" leftSection={<IconEdit size={14} />}>Edit</Tabs.Tab>
          {!isNew && <Tabs.Tab value="versions" leftSection={<IconHistory size={14} />}>History</Tabs.Tab>}
        </Tabs.List>

        <Tabs.Panel value="edit" pt="md">
          <Stack gap="md">
            <Group align="flex-start">
              <TextInput
                label="Name"
                value={name}
                onChange={(e) => setName(e.currentTarget.value)}
                required
                style={{ flex: 1 }}
              />
              <NumberInput
                label="Priority"
                value={priority}
                onChange={(v) => setPriority(Number(v))}
                w={120}
              />
            </Group>
            <Textarea
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.currentTarget.value)}
              rows={2}
            />
            <Textarea
              label="Host selector (JSON)"
              value={selectorRaw}
              onChange={(e) => setSelectorRaw(e.currentTarget.value)}
              rows={3}
              styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
            />

            <div>
              <Group justify="space-between" mb="sm">
                <Text fw={600}>Rules</Text>
                <Button size="xs" leftSection={<IconPlus size={12} />} onClick={addRule}>Add rule</Button>
              </Group>
              <Table highlightOnHover>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>#</Table.Th>
                    <Table.Th>Name</Table.Th>
                    <Table.Th>Direction</Table.Th>
                    <Table.Th>Action</Table.Th>
                    <Table.Th>Log</Table.Th>
                    <Table.Th />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {rules.map((r, i) => (
                    <Table.Tr key={i} style={editRuleIdx === i ? { background: 'var(--mantine-color-blue-0)' } : undefined}>
                      <Table.Td>{r.priority}</Table.Td>
                      <Table.Td>{r.name || <Text c="dimmed" size="sm">unnamed</Text>}</Table.Td>
                      <Table.Td><Badge size="sm" variant="outline">{r.direction}</Badge></Table.Td>
                      <Table.Td>
                        <Badge size="sm" color={r.action === 'deny' ? 'red' : r.action === 'allow' ? 'green' : 'blue'}>
                          {r.action}
                        </Badge>
                      </Table.Td>
                      <Table.Td>{r.log ? '✓' : ''}</Table.Td>
                      <Table.Td>
                        <Group gap={4}>
                          <ActionIcon size="sm" variant="subtle" onClick={() => setEditRuleIdx(editRuleIdx === i ? null : i)}>
                            <IconEdit size={12} />
                          </ActionIcon>
                          <ActionIcon size="sm" variant="subtle" color="red" onClick={() => removeRule(i)}>
                            <IconTrash size={12} />
                          </ActionIcon>
                        </Group>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                  {rules.length === 0 && (
                    <Table.Tr>
                      <Table.Td colSpan={6}>
                        <Text c="dimmed" ta="center" size="sm" py="md">No rules — add one above</Text>
                      </Table.Td>
                    </Table.Tr>
                  )}
                </Table.Tbody>
              </Table>

              {editRuleIdx !== null && rules[editRuleIdx] && (
                <Paper withBorder p="md" mt="sm">
                  <Stack gap="sm">
                    <Text fw={600} size="sm">Edit rule #{editRuleIdx + 1}</Text>
                    <Group>
                      <TextInput
                        label="Name"
                        value={rules[editRuleIdx].name}
                        onChange={(e) => updateRule(editRuleIdx, { name: e.currentTarget.value })}
                        style={{ flex: 1 }}
                      />
                      <NumberInput
                        label="Priority"
                        value={rules[editRuleIdx].priority}
                        onChange={(v) => updateRule(editRuleIdx, { priority: Number(v) })}
                        w={100}
                      />
                    </Group>
                    <Group>
                      <Select
                        label="Direction"
                        data={['ingress', 'egress', 'both']}
                        value={rules[editRuleIdx].direction}
                        onChange={(v) => updateRule(editRuleIdx, { direction: v ?? 'ingress' })}
                        w={140}
                      />
                      <Select
                        label="Action"
                        data={['allow', 'deny', 'log']}
                        value={rules[editRuleIdx].action}
                        onChange={(v) => updateRule(editRuleIdx, { action: v ?? 'allow' })}
                        w={120}
                      />
                    </Group>
                    <Textarea
                      label="Match (JSON)"
                      value={JSON.stringify(rules[editRuleIdx].match, null, 2)}
                      onChange={(e) => {
                        try { updateRule(editRuleIdx, { match: JSON.parse(e.currentTarget.value) }) } catch { /* noop */ }
                      }}
                      rows={4}
                      styles={{ input: { fontFamily: 'monospace', fontSize: 12 } }}
                    />
                    <Group>
                      <Checkbox
                        label="Log"
                        checked={rules[editRuleIdx].log}
                        onChange={(e) => updateRule(editRuleIdx, { log: e.currentTarget.checked })}
                      />
                      <Checkbox
                        label="Silent"
                        checked={rules[editRuleIdx].silent}
                        onChange={(e) => updateRule(editRuleIdx, { silent: e.currentTarget.checked })}
                      />
                    </Group>
                    <Button size="xs" variant="subtle" onClick={() => setEditRuleIdx(null)}>Close</Button>
                  </Stack>
                </Paper>
              )}
            </div>

            <Group>
              {!isNew && (
                <Button
                  variant="outline"
                  leftSection={<IconPlayerPlay size={14} />}
                  onClick={handlePreview}
                >
                  Preview impact
                </Button>
              )}
              <Button loading={saveMut.isPending} onClick={() => saveMut.mutate()}>
                Save policy
              </Button>
            </Group>
          </Stack>
        </Tabs.Panel>

        {!isNew && (
          <Tabs.Panel value="versions" pt="md">
            <Stack gap="sm">
              {versions.map((v) => (
                <Paper key={v.id} withBorder p="md">
                  <Group justify="space-between">
                    <div>
                      <Text fw={600}>v{v.version}</Text>
                      <Text size="xs" c="dimmed">
                        {new Date(v.created_at).toLocaleString()} · by {v.created_by.slice(0, 8)}
                      </Text>
                    </div>
                    <Button
                      size="xs"
                      variant="outline"
                      leftSection={<IconArrowBack size={12} />}
                      loading={revertMut.isPending}
                      onClick={() => revertMut.mutate(v.version)}
                    >
                      Revert
                    </Button>
                  </Group>
                  <Code block mt="sm" style={{ fontSize: 11, maxHeight: 200, overflow: 'auto' }}>
                    {JSON.stringify(v.content, null, 2)}
                  </Code>
                </Paper>
              ))}
              {versions.length === 0 && (
                <Text c="dimmed" ta="center" size="sm" py="md">No versions yet</Text>
              )}
            </Stack>
          </Tabs.Panel>
        )}
      </Tabs>

      <Modal
        opened={previewOpen}
        onClose={() => setPreviewOpen(false)}
        title="Preview impact"
        size="lg"
      >
        {previewLoading && <Center py="xl"><Loader /></Center>}
        {!previewLoading && previewResult && (
          <Stack gap="md">
            <Alert icon={<IconAlertCircle size={16} />} color={previewResult.affected_count > 0 ? 'yellow' : 'green'}>
              {previewResult.affected_count} host{previewResult.affected_count !== 1 ? 's' : ''} affected
            </Alert>
            <Accordion>
              {previewResult.hosts.map((h) => (
                <Accordion.Item key={h.id} value={h.id}>
                  <Accordion.Control>
                    <Group gap="xs">
                      <Text size="sm">{h.hostname}</Text>
                      {h.added.length > 0 && <Badge size="xs" color="green">+{h.added.length}</Badge>}
                      {h.removed.length > 0 && <Badge size="xs" color="red">-{h.removed.length}</Badge>}
                      {h.changed.length > 0 && <Badge size="xs" color="yellow">~{h.changed.length}</Badge>}
                    </Group>
                  </Accordion.Control>
                  <Accordion.Panel>
                    {h.added.map((r) => <Text key={r} size="xs" c="green">+ {r}</Text>)}
                    {h.removed.map((r) => <Text key={r} size="xs" c="red">- {r}</Text>)}
                    {h.changed.map((r) => <Text key={r} size="xs" c="yellow">~ {r}</Text>)}
                  </Accordion.Panel>
                </Accordion.Item>
              ))}
            </Accordion>
          </Stack>
        )}
      </Modal>
    </Stack>
  )
}
