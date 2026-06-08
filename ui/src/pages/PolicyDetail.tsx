import { useState, useEffect } from 'react'
import { fmtDateTime } from '../utils/date'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Tabs, Button, TextInput, Textarea, NumberInput, Badge,
  Group, Text, Anchor, ActionIcon, Modal, Select, Checkbox, Loader, Center,
  Accordion, Code, Paper, Alert, Divider,
} from '@mantine/core'
import {
  IconPlus, IconTrash, IconEdit, IconPlayerPlay, IconHistory, IconArrowBack,
  IconAlertCircle, IconX, IconUsers,
} from '@tabler/icons-react'
import QFBadge from '../components/QFBadge'
import QFCard from '../components/QFCard'
import { QFTable, TH, TD } from '../components/QFTable'
import PageHead from '../components/PageHead'
import { notifications } from '@mantine/notifications'
import { getPolicy, updatePolicy, createPolicy, previewPolicy, listVersions, revertVersion } from '../api/policies'
import { listObjectGroups } from '../api/objectgroups'
import { listHosts, patchHost } from '../api/hosts'
import type { Rule, PreviewResult, Host, ObjectGroup } from '../types'

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

interface RuleMatch {
  protocol?: string
  src_cidrs?: string[]
  dst_cidrs?: string[]
  src_ports?: string[]
  dst_ports?: string[]
  src_ip_set_id?: string
  dst_ip_set_id?: string
  src_port_set_id?: string
  dst_port_set_id?: string
  src_host_set_id?: string
  dst_host_set_id?: string
  // legacy field names — read-only, migrated to src_cidrs/dst_cidrs on save
  src_ip?: string
  dst_ip?: string
}

function MatchEditor({ value, onChange }: { value: unknown; onChange: (v: unknown) => void }) {
  const m = (value ?? {}) as RuleMatch

  const { data: groups = [] } = useQuery({ queryKey: ['objectgroups'], queryFn: listObjectGroups })
  const ipGroups = groups
    .filter(g => g.type === 'ipset' || g.type === 'hostset')
    .map(g => ({ value: g.id, label: `${g.name} (${g.type})` }))
  const portGroups = groups
    .filter(g => g.type === 'portset')
    .map(g => ({ value: g.id, label: g.name }))

  function set(patch: Partial<RuleMatch>) {
    const next: RuleMatch = { ...m, ...patch }
    delete next.src_ip
    delete next.dst_ip
    if (!next.protocol) delete next.protocol
    if (!next.src_cidrs?.length) delete next.src_cidrs
    if (!next.dst_cidrs?.length) delete next.dst_cidrs
    if (!next.src_ports?.length) delete next.src_ports
    if (!next.dst_ports?.length) delete next.dst_ports
    if (!next.src_ip_set_id) delete next.src_ip_set_id
    if (!next.dst_ip_set_id) delete next.dst_ip_set_id
    if (!next.src_port_set_id) delete next.src_port_set_id
    if (!next.dst_port_set_id) delete next.dst_port_set_id
    if (!next.src_host_set_id) delete next.src_host_set_id
    if (!next.dst_host_set_id) delete next.dst_host_set_id
    onChange(next)
  }

  const srcIPGroupId = m.src_ip_set_id ?? m.src_host_set_id ?? null
  const dstIPGroupId = m.dst_ip_set_id ?? m.dst_host_set_id ?? null

  function setSrcIPGroup(id: string | null) {
    const g = groups.find(x => x.id === id)
    set({ src_ip_set_id: g?.type === 'ipset' ? (id ?? undefined) : undefined,
          src_host_set_id: g?.type === 'hostset' ? (id ?? undefined) : undefined,
          src_cidrs: undefined })
  }
  function setDstIPGroup(id: string | null) {
    const g = groups.find(x => x.id === id)
    set({ dst_ip_set_id: g?.type === 'ipset' ? (id ?? undefined) : undefined,
          dst_host_set_id: g?.type === 'hostset' ? (id ?? undefined) : undefined,
          dst_cidrs: undefined })
  }

  // handle legacy src_ip/dst_ip field for display
  const srcCIDRsDisplay = (m.src_cidrs ?? (m.src_ip ? [m.src_ip] : [])).join(', ')
  const dstCIDRsDisplay = (m.dst_cidrs ?? (m.dst_ip ? [m.dst_ip] : [])).join(', ')

  return (
    <Stack gap="xs">
      <Select
        label="Protocol"
        size="xs"
        data={[{ value: '', label: 'Any' }, { value: 'tcp', label: 'TCP' }, { value: 'udp', label: 'UDP' }, { value: 'icmp', label: 'ICMP' }]}
        value={m.protocol ?? ''}
        onChange={(v) => set({ protocol: v ?? '' })}
      />
      <Group grow align="flex-start">
        <Stack gap={4}>
          <TextInput size="xs" label="Src IP / CIDR" placeholder="10.0.0.0/8, 192.168.1.1"
            disabled={!!srcIPGroupId}
            value={srcCIDRsDisplay}
            onChange={(e) => set({ src_cidrs: e.currentTarget.value.split(',').map(s => s.trim()).filter(Boolean), src_ip_set_id: undefined, src_host_set_id: undefined })}
          />
          <Select size="xs" label="or src IP/Host group" clearable
            data={ipGroups} value={srcIPGroupId} onChange={setSrcIPGroup}
          />
        </Stack>
        <Stack gap={4}>
          <TextInput size="xs" label="Dst IP / CIDR" placeholder="10.0.0.0/8, 192.168.1.1"
            disabled={!!dstIPGroupId}
            value={dstCIDRsDisplay}
            onChange={(e) => set({ dst_cidrs: e.currentTarget.value.split(',').map(s => s.trim()).filter(Boolean), dst_ip_set_id: undefined, dst_host_set_id: undefined })}
          />
          <Select size="xs" label="or dst IP/Host group" clearable
            data={ipGroups} value={dstIPGroupId} onChange={setDstIPGroup}
          />
        </Stack>
      </Group>
      <Group grow align="flex-start">
        <Stack gap={4}>
          <TextInput size="xs" label="Src ports" placeholder="80, 443, 8000-9000"
            disabled={!!m.src_port_set_id}
            value={(m.src_ports ?? []).join(', ')}
            onChange={(e) => set({ src_ports: e.currentTarget.value.split(',').map(s => s.trim()).filter(Boolean), src_port_set_id: undefined })}
          />
          <Select size="xs" label="or src port group" clearable
            data={portGroups} value={m.src_port_set_id ?? null}
            onChange={(id) => set({ src_port_set_id: id ?? undefined, src_ports: undefined })}
          />
        </Stack>
        <Stack gap={4}>
          <TextInput size="xs" label="Dst ports" placeholder="80, 443, 8000-9000"
            disabled={!!m.dst_port_set_id}
            value={(m.dst_ports ?? []).join(', ')}
            onChange={(e) => set({ dst_ports: e.currentTarget.value.split(',').map(s => s.trim()).filter(Boolean), dst_port_set_id: undefined })}
          />
          <Select size="xs" label="or dst port group" clearable
            data={portGroups} value={m.dst_port_set_id ?? null}
            onChange={(id) => set({ dst_port_set_id: id ?? undefined, dst_ports: undefined })}
          />
        </Stack>
      </Group>
    </Stack>
  )
}

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
  const [selectorLabels, setSelectorLabels] = useState<KVPair[]>([])
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
      const sel = (policy.selector as Record<string, string>) ?? {}
      setSelectorLabels(Object.entries(sel).map(([key, val]) => ({ key, val })))
      setRules(policy.rules.map(({ id: _id, policy_id: _pid, created_at: _c, updated_at: _u, ...r }) => r))
    }
  }, [policy])

  const saveMut = useMutation({
    mutationFn: async () => {
      const selector = Object.fromEntries(selectorLabels.filter(p => p.key).map(p => [p.key, p.val]))
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
      if (isNew) navigate(`/policies/${savedId}`)
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

  const [assignOpen, setAssignOpen] = useState(false)
  const [selectedHostId, setSelectedHostId] = useState<string | null>(null)
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null)

  const { data: allHosts = [] } = useQuery<Host[]>({
    queryKey: ['hosts'],
    queryFn: listHosts,
    enabled: assignOpen,
  })
  const { data: allGroups = [] } = useQuery<ObjectGroup[]>({
    queryKey: ['objectgroups'],
    queryFn: listObjectGroups,
    enabled: assignOpen,
  })

  const hostSetGroups = allGroups.filter(g => g.type === 'hostset')

  // Extract flat matchLabels from policy selector regardless of nesting format.
  // Policies created via UI: {hostname: 'foo'}
  // Policies created via API: {matchLabels: {hostname: 'foo'}}
  function getPolicyMatchLabels(): Record<string, string> {
    const sel = (policy?.selector ?? {}) as Record<string, unknown>
    if (sel.matchLabels && typeof sel.matchLabels === 'object') {
      return sel.matchLabels as Record<string, string>
    }
    return sel as Record<string, string>
  }

  const matchedHosts = allHosts.filter(h => {
    const ml = getPolicyMatchLabels()
    return Object.keys(ml).length > 0 && Object.entries(ml).every(([k, v]) => h.labels[k] === v)
  })

  async function savePolicyWithSelector(newMatchLabels: Record<string, string>) {
    const newSelector = { matchLabels: newMatchLabels }
    await updatePolicy(id!, { name, description, priority, selector: newSelector, rules })
    setSelectorLabels(Object.entries(newMatchLabels).map(([key, val]) => ({ key, val })))
    qc.invalidateQueries({ queryKey: ['policy', id] })
  }

  async function assignHost(host: Host) {
    await patchHost(host.id, { labels: { ...host.labels, hostname: host.hostname } })
    const newMatchLabels = { ...getPolicyMatchLabels(), hostname: host.hostname }
    await savePolicyWithSelector(newMatchLabels)
    qc.invalidateQueries({ queryKey: ['hosts'] })
    setSelectedHostId(null)
    notifications.show({ message: `Assigned to ${host.hostname}`, color: 'green' })
  }

  async function assignGroup(group: ObjectGroup) {
    const spec = (group.spec ?? {}) as Record<string, unknown>
    const groupMatchLabels = ((spec.selector as Record<string, unknown>)?.matchLabels ?? {}) as Record<string, string>
    if (Object.keys(groupMatchLabels).length === 0) {
      notifications.show({ message: 'Host group has no selector labels', color: 'red' })
      return
    }
    const newMatchLabels = { ...getPolicyMatchLabels(), ...groupMatchLabels }
    await savePolicyWithSelector(newMatchLabels)
    setSelectedGroupId(null)
    notifications.show({ message: `Selector updated from group "${group.name}"`, color: 'green' })
  }

  async function unassignHost(host: Host) {
    const ml = getPolicyMatchLabels()
    const newLabels = { ...host.labels }
    Object.entries(ml).forEach(([k, v]) => { if (newLabels[k] === v) delete newLabels[k] })
    await patchHost(host.id, { labels: newLabels })
    qc.invalidateQueries({ queryKey: ['hosts'] })
    notifications.show({ message: `Unassigned from ${host.hostname}`, color: 'green' })
  }

  async function handlePreview() {
    if (!id || isNew) return
    setPreviewLoading(true)
    setPreviewOpen(true)
    try {
      const result = await previewPolicy(id, rules)
      setPreviewResult(result)
    } catch (e: unknown) {
      setPreviewResult(null)
      setPreviewOpen(false)
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Preview failed'
      notifications.show({ message: msg, color: 'red' })
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
      <div style={{ marginBottom: 20 }}>
        <div style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', marginBottom: 8, display: 'flex', gap: 6, alignItems: 'center' }}>
          <Anchor component={Link} to="/policies" style={{ color: 'var(--qf-brand)', textDecoration: 'none', fontSize: 'var(--qf-t-sm)' }}>Policies</Anchor>
          <span>/</span>
          <span style={{ color: 'var(--qf-fg-3)' }}>{isNew ? 'New policy' : (policy?.name ?? '…')}</span>
        </div>
        <PageHead title={isNew ? 'New policy' : (policy?.name ?? '…')} />
      </div>

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
            <div>
              <Text size="sm" fw={500} mb={4}>Host selector <Text span c="dimmed" size="xs">(apply policy to hosts matching all labels)</Text></Text>
              <LabelSelector value={selectorLabels} onChange={setSelectorLabels} />
            </div>

            <div>
              <Group justify="space-between" mb="sm">
                <Text fw={600}>Rules</Text>
                <Button size="xs" leftSection={<IconPlus size={12} />} onClick={addRule}>Add rule</Button>
              </Group>

              {/* Rule table + Inspector side panel */}
              <QFCard pad={false}>
                <div style={{ display: 'flex' }}>
                  {/* Table */}
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <QFTable minWidth={400}>
                      <thead>
                        <tr style={{ height: 38 }}>
                          <TH w={60}>Priority</TH>
                          <TH>Name</TH>
                          <TH w={90}>Direction</TH>
                          <TH w={80}>Action</TH>
                          <TH w={50}>Log</TH>
                          <TH w={64} />
                        </tr>
                      </thead>
                      <tbody>
                        {rules.map((r, i) => {
                          const selected = editRuleIdx === i
                          return (
                            <tr
                              key={i}
                              className="qf-row"
                              onClick={() => setEditRuleIdx(selected ? null : i)}
                              style={{
                                borderTop: '1px solid var(--qf-border-2)',
                                cursor: 'pointer',
                                background: selected ? 'var(--qf-bg-muted)' : undefined,
                                boxShadow: selected ? 'inset 2px 0 0 var(--qf-brand-solid)' : undefined,
                              }}
                            >
                              <TD mono muted>{r.priority}</TD>
                              <TD style={{ color: r.name ? 'var(--qf-fg-2)' : 'var(--qf-fg-faint)' }}>
                                {r.name || 'unnamed'}
                              </TD>
                              <TD muted>{r.direction}</TD>
                              <TD>
                                <QFBadge tone={r.action === 'deny' ? 'bad' : r.action === 'allow' ? 'ok' : 'info'}>
                                  {r.action}
                                </QFBadge>
                              </TD>
                              <TD muted>{r.log ? '✓' : ''}</TD>
                              <TD>
                                <ActionIcon
                                  size="sm" variant="subtle" color="red"
                                  onClick={(e) => { e.stopPropagation(); removeRule(i) }}
                                >
                                  <IconTrash size={12} />
                                </ActionIcon>
                              </TD>
                            </tr>
                          )
                        })}
                        {rules.length === 0 && (
                          <tr>
                            <td colSpan={6}>
                              <div style={{ padding: '40px 24px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>
                                No rules — click "Add rule" above
                              </div>
                            </td>
                          </tr>
                        )}
                      </tbody>
                    </QFTable>
                  </div>

                  {/* Inspector panel */}
                  {editRuleIdx !== null && rules[editRuleIdx] && (
                    <div style={{
                      width: 320, flexShrink: 0,
                      borderLeft: '1px solid var(--qf-border-1)',
                      padding: 20, overflowY: 'auto',
                    }}>
                      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
                        <span style={{ fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>
                          Rule #{editRuleIdx + 1}
                        </span>
                        <ActionIcon size="sm" variant="subtle" onClick={() => setEditRuleIdx(null)}>
                          <IconX size={14} />
                        </ActionIcon>
                      </div>
                      <Stack gap="sm">
                        <Group>
                          <TextInput
                            label="Name" size="xs" style={{ flex: 1 }}
                            value={rules[editRuleIdx].name}
                            onChange={(e) => updateRule(editRuleIdx, { name: e.currentTarget.value })}
                          />
                          <NumberInput
                            label="Priority" size="xs" w={80}
                            value={rules[editRuleIdx].priority}
                            onChange={(v) => updateRule(editRuleIdx, { priority: Number(v) })}
                          />
                        </Group>
                        <Group grow>
                          <Select
                            label="Direction" size="xs"
                            data={['ingress', 'egress', 'both']}
                            value={rules[editRuleIdx].direction}
                            onChange={(v) => updateRule(editRuleIdx, { direction: v ?? 'ingress' })}
                          />
                          <Select
                            label="Action" size="xs"
                            data={['allow', 'deny', 'log']}
                            value={rules[editRuleIdx].action}
                            onChange={(v) => updateRule(editRuleIdx, { action: v ?? 'allow' })}
                          />
                        </Group>
                        <div>
                          <Text size="xs" fw={500} mb={4}>Match conditions</Text>
                          <MatchEditor value={rules[editRuleIdx].match} onChange={(v) => updateRule(editRuleIdx, { match: v })} />
                        </div>
                        <Group>
                          <Checkbox label="Log" size="xs"
                            checked={rules[editRuleIdx].log}
                            onChange={(e) => updateRule(editRuleIdx, { log: e.currentTarget.checked })}
                          />
                          <Checkbox label="Silent" size="xs"
                            checked={rules[editRuleIdx].silent}
                            onChange={(e) => updateRule(editRuleIdx, { silent: e.currentTarget.checked })}
                          />
                        </Group>
                      </Stack>
                    </div>
                  )}
                </div>
              </QFCard>
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
              {!isNew && (
                <Button
                  variant="outline"
                  leftSection={<IconUsers size={14} />}
                  onClick={() => setAssignOpen(true)}
                >
                  Assign to hosts
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
                        {fmtDateTime(v.created_at)} · by {v.created_by.slice(0, 8)}
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
        opened={assignOpen}
        onClose={() => setAssignOpen(false)}
        title="Assign to hosts"
        size="md"
      >
        <Stack gap="md">
          {matchedHosts.length > 0 && (
            <div>
              <Text size="sm" fw={500} mb="xs">Currently assigned ({matchedHosts.length})</Text>
              <Stack gap="xs">
                {matchedHosts.map(h => (
                  <Group key={h.id} justify="space-between">
                    <Badge variant="outline">{h.hostname}</Badge>
                    <ActionIcon
                      size="sm"
                      color="red"
                      variant="subtle"
                      title="Unassign"
                      onClick={() => unassignHost(h)}
                    >
                      <IconX size={12} />
                    </ActionIcon>
                  </Group>
                ))}
              </Stack>
              <Divider mt="sm" />
            </div>
          )}

          <div>
            <Text size="sm" fw={500} mb="xs">Assign single host</Text>
            <Group>
              <Select
                placeholder="Search host by name…"
                searchable
                clearable
                data={allHosts.map(h => ({ value: h.id, label: h.hostname }))}
                value={selectedHostId}
                onChange={setSelectedHostId}
                style={{ flex: 1 }}
                size="sm"
              />
              <Button
                size="sm"
                disabled={!selectedHostId}
                loading={saveMut.isPending}
                onClick={() => {
                  const h = allHosts.find(x => x.id === selectedHostId)
                  if (h) assignHost(h)
                }}
              >
                Assign
              </Button>
            </Group>
          </div>

          <Divider />

          <div>
            <Text size="sm" fw={500} mb="xs">Assign host group</Text>
            <Text size="xs" c="dimmed" mb="xs">Copies the group's label selector into the policy selector</Text>
            <Group>
              <Select
                placeholder="Select host group…"
                clearable
                data={hostSetGroups.map(g => ({ value: g.id, label: g.name }))}
                value={selectedGroupId}
                onChange={setSelectedGroupId}
                style={{ flex: 1 }}
                size="sm"
              />
              <Button
                size="sm"
                disabled={!selectedGroupId}
                loading={saveMut.isPending}
                onClick={() => {
                  const g = hostSetGroups.find(x => x.id === selectedGroupId)
                  if (g) assignGroup(g)
                }}
              >
                Assign
              </Button>
            </Group>
          </div>
        </Stack>
      </Modal>

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
              {previewResult.hosts.map((h) => {
                const added = h.added ?? []
                const removed = h.removed ?? []
                const changed = h.changed ?? []
                return (
                  <Accordion.Item key={h.id} value={h.id}>
                    <Accordion.Control>
                      <Group gap="xs">
                        <Text size="sm">{h.hostname}</Text>
                        {added.length > 0 && <Badge size="xs" color="green">+{added.length}</Badge>}
                        {removed.length > 0 && <Badge size="xs" color="red">-{removed.length}</Badge>}
                        {changed.length > 0 && <Badge size="xs" color="yellow">~{changed.length}</Badge>}
                      </Group>
                    </Accordion.Control>
                    <Accordion.Panel>
                      {added.map((r) => <Text key={r} size="xs" c="green">+ {r}</Text>)}
                      {removed.map((r) => <Text key={r} size="xs" c="red">- {r}</Text>)}
                      {changed.map((r) => <Text key={r} size="xs" c="yellow">~ {r}</Text>)}
                    </Accordion.Panel>
                  </Accordion.Item>
                )
              })}
            </Accordion>
          </Stack>
        )}
      </Modal>
    </Stack>
  )
}
