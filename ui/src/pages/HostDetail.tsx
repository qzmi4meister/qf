import { useMemo } from 'react'
import { fmtDateTime, fmtTime } from '../utils/date'
import { useParams, Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  Tabs, Group, Text, TextInput, Loader, Center,
  Select as MSelect, Button, Switch, Modal, ActionIcon,
} from '@mantine/core'
import { useState } from 'react'
import { IconSearch, IconDownload, IconEdit, IconPlus, IconX, IconShield, IconCopy } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useMutation } from '@tanstack/react-query'
import { getHost, listEvents, listFlows, latestCounters, getHostRuleset, patchHost } from '../api/hosts'
import { listPolicies } from '../api/policies'
import type { RulesetRuleItem, Policy } from '../types'
import { notifications } from '@mantine/notifications'
import StatusBadge from '../components/StatusBadge'
import Chip from '../components/Chip'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import { QFTable, TH, TD } from '../components/QFTable'

function protoName(p: number): string {
  const names: Record<number, string> = { 1: 'any', 2: 'TCP', 3: 'UDP', 4: 'ICMP', 5: 'ICMPv6' }
  return names[p] ?? String(p)
}

type RuleMap = Map<string, { rule_name: string; policy_name: string }>

function ruleLabel(ruleId: string | undefined, ruleMap: RuleMap): string {
  if (!ruleId) return '—'
  const r = ruleMap.get(ruleId)
  if (r?.rule_name) return r.policy_name ? `${r.rule_name} (${r.policy_name})` : r.rule_name
  return ruleId.slice(0, 8)
}

function actionTone(a: string) {
  return a === 'deny' ? 'bad' : a === 'allow' ? 'ok' : 'info'
}

export default function HostDetail() {
  const { id } = useParams<{ id: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') ?? 'overview'
  function setActiveTab(t: string | null) {
    setSearchParams(t && t !== 'overview' ? { tab: t } : {}, { replace: true })
  }

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

  const [editLabelsOpen, setEditLabelsOpen] = useState(false)
  const [editLabelPairs, setEditLabelPairs] = useState<{ key: string; val: string }[]>([])
  const labelsMut = useMutation({
    mutationFn: (pairs: { key: string; val: string }[]) =>
      patchHost(id!, { labels: Object.fromEntries(pairs.filter(p => p.key).map(p => [p.key, p.val])) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['host', id] })
      setEditLabelsOpen(false)
      notifications.show({ message: 'Labels saved', color: 'green' })
    },
    onError: () => notifications.show({ message: 'Save failed', color: 'red' }),
  })

  const { data: events = [] } = useQuery({
    queryKey: ['events', id],
    queryFn: () => listEvents(id!, { limit: 100 }),
    enabled: !!id && activeTab === 'events',
    refetchInterval: activeTab === 'events' ? 10_000 : false,
  })
  const { data: flows = [] } = useQuery({
    queryKey: ['flows', id],
    queryFn: () => listFlows(id!, { limit: 100 }),
    enabled: !!id && activeTab === 'flows',
  })
  const { data: counters = [] } = useQuery({
    queryKey: ['counters', id],
    queryFn: () => latestCounters(id!),
    enabled: !!id && activeTab === 'counters',
    refetchInterval: activeTab === 'counters' ? 30_000 : false,
  })
  const { data: ruleset, isLoading: rulesetLoading } = useQuery({
    queryKey: ['ruleset', id],
    queryFn: () => getHostRuleset(id!),
    enabled: !!id,
  })
  const { data: allPolicies = [] } = useQuery<Policy[]>({
    queryKey: ['policies'],
    queryFn: listPolicies,
    enabled: !!id,
  })

  const ruleMap: RuleMap = useMemo(() => {
    const m: RuleMap = new Map()
    for (const r of ruleset?.rules ?? []) {
      m.set(r.rule_id, { rule_name: r.rule_name, policy_name: r.policy_name })
    }
    return m
  }, [ruleset])

  if (isLoading) return <Center h={200}><Loader /></Center>
  if (!host) return <Text c="red">Host not found</Text>

  const matchedPolicies = allPolicies.filter(p => {
    const sel = (p.selector ?? {}) as Record<string, unknown>
    const ml: Record<string, string> = (sel.matchLabels && typeof sel.matchLabels === 'object')
      ? sel.matchLabels as Record<string, string>
      : sel as Record<string, string>
    const keys = Object.keys(ml)
    return keys.length > 0 && keys.every(k => host.labels[k] === ml[k])
  })

  const filteredEvents = events.filter((e) => {
    const matchAction = !actionFilter || e.action === actionFilter
    const matchSearch = !search ||
      (e.src_ip ?? '').includes(search) ||
      (e.dst_ip ?? '').includes(search) ||
      (e.rule_id ?? '').includes(search)
    return matchAction && matchSearch
  })

  return (
    <>
      {/* Breadcrumb + header */}
      <div style={{ marginBottom: 20 }}>
        <div style={{
          fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)',
          marginBottom: 8, display: 'flex', gap: 6, alignItems: 'center',
        }}>
          <Link to="/hosts" style={{ color: 'var(--qf-brand)', textDecoration: 'none' }}>Hosts</Link>
          <span>/</span>
          <span style={{ color: 'var(--qf-fg-3)', fontFamily: 'var(--qf-mono)' }}>{host.hostname}</span>
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <h1 style={{
            margin: 0, fontSize: 'var(--qf-t-xl)', fontWeight: 700,
            color: 'var(--qf-fg-1)', letterSpacing: '-0.02em',
            fontFamily: 'var(--qf-mono)',
          }}>
            {host.hostname}
          </h1>
          <ActionIcon
            size="sm" variant="subtle"
            title="Copy hostname"
            onClick={() => {
              navigator.clipboard.writeText(host.hostname)
              notifications.show({ message: 'Hostname copied', color: 'gray', autoClose: 1500 })
            }}
          >
            <IconCopy size={14} />
          </ActionIcon>
          <StatusBadge status={host.status} />
        </div>
      </div>

      <Tabs value={activeTab} onChange={setActiveTab}>
        <Tabs.List>
          <Tabs.Tab value="overview">Overview</Tabs.Tab>
          <Tabs.Tab value="policies" leftSection={<IconShield size={14} />}>
            Policies {matchedPolicies.length > 0 && (
              <span style={{
                marginLeft: 4, fontSize: 'var(--qf-t-xs)', fontWeight: 700,
                background: 'var(--qf-info-bg)', color: 'var(--qf-info-fg)',
                padding: '1px 6px', borderRadius: 'var(--qf-r-full)',
              }}>{matchedPolicies.length}</span>
            )}
          </Tabs.Tab>
          <Tabs.Tab value="ruleset">Ruleset</Tabs.Tab>
          <Tabs.Tab value="events">Events</Tabs.Tab>
          <Tabs.Tab value="flows">Flows</Tabs.Tab>
          <Tabs.Tab value="counters">Counters</Tabs.Tab>
        </Tabs.List>

        <Tabs.Panel value="overview" pt="md">
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
            <QFCard>
              <div style={{ fontWeight: 600, color: 'var(--qf-fg-1)', marginBottom: 12 }}>Agent info</div>
              {[
                ['Version', host.agent_version ?? '—', true],
                ['Kernel', host.kernel_version ?? '—', true],
                ['Generation', String(host.current_generation), true],
                ['Last heartbeat', fmtDateTime(host.last_heartbeat_at), false],
              ].map(([label, value, mono]) => (
                <div key={String(label)} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>{label}</span>
                  <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-2)', fontFamily: mono ? 'var(--qf-mono)' : 'inherit' }}>{value}</span>
                </div>
              ))}
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>Flow events</span>
                <Switch
                  size="sm"
                  checked={host.flow_events_enabled}
                  onChange={(e) => flowMut.mutate(e.currentTarget.checked)}
                  disabled={flowMut.isPending}
                />
              </div>
            </QFCard>

            <QFCard>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                <span style={{ fontWeight: 600, color: 'var(--qf-fg-1)' }}>Labels</span>
                <ActionIcon size="sm" variant="subtle" onClick={() => {
                  setEditLabelPairs(Object.entries(host.labels).map(([key, val]) => ({ key, val })))
                  setEditLabelsOpen(true)
                }}>
                  <IconEdit size={14} />
                </ActionIcon>
              </div>
              {Object.entries(host.labels).length === 0 ? (
                <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>No labels</span>
              ) : (
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {Object.entries(host.labels).map(([k, v]) => <Chip key={k} k={k} v={v} />)}
                </div>
              )}
            </QFCard>
          </div>
        </Tabs.Panel>

        <Tabs.Panel value="policies" pt="md">
          <QFCard pad={false}>
            <QFTable>
              <thead>
                <tr style={{ height: 38 }}>
                  <TH>Name</TH><TH w={80}>Priority</TH><TH>Description</TH><TH w={160} right>Updated</TH>
                </tr>
              </thead>
              <tbody>
                {matchedPolicies.map(p => (
                  <tr key={p.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD>
                      <Link to={`/policies/${p.id}`} style={{ color: 'var(--qf-brand)', textDecoration: 'none' }}>{p.name}</Link>
                    </TD>
                    <TD mono muted>{p.priority}</TD>
                    <TD muted>{p.description || '—'}</TD>
                    <TD mono muted right>{fmtDateTime(p.updated_at)}</TD>
                  </tr>
                ))}
                {matchedPolicies.length === 0 && (
                  <tr><td colSpan={4}>
                    <div style={{ padding: '32px 24px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>
                      No policies assigned to this host
                    </div>
                  </td></tr>
                )}
              </tbody>
            </QFTable>
          </QFCard>
        </Tabs.Panel>

        <Tabs.Panel value="ruleset" pt="md">
          {rulesetLoading ? (
            <Center h={100}><Loader /></Center>
          ) : !ruleset ? (
            <Text c="dimmed">No ruleset data</Text>
          ) : (
            <div>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                <div style={{ display: 'flex', gap: 16, alignItems: 'center' }}>
                  <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>Default ingress:</span>
                  <QFBadge tone={ruleset.default_ingress === 'allow' ? 'ok' : 'bad'}>{ruleset.default_ingress}</QFBadge>
                  <span style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)' }}>Default egress:</span>
                  <QFBadge tone={ruleset.default_egress === 'allow' ? 'ok' : 'bad'}>{ruleset.default_egress}</QFBadge>
                </div>
                <Button size="xs" variant="light" leftSection={<IconDownload size={14} />}
                  onClick={() => downloadRuleset(host.hostname, ruleset.rules)}>
                  Download JSON
                </Button>
              </div>
              <QFCard pad={false}>
                <QFTable minWidth={900}>
                  <thead>
                    <tr style={{ height: 38 }}>
                      <TH w={30}>#</TH>
                      <TH>Rule</TH><TH>Policy</TH><TH w={80}>Dir</TH>
                      <TH w={80}>Action</TH><TH w={60}>Proto</TH>
                      <TH>Src CIDRs</TH><TH>Dst CIDRs</TH>
                      <TH w={100}>Src Ports</TH><TH w={100}>Dst Ports</TH>
                    </tr>
                  </thead>
                  <tbody>
                    {ruleset.rules.map((r, i) => (
                      <tr key={r.rule_id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                        <TD muted>{i + 1}</TD>
                        <TD mono title={r.rule_id}>{r.rule_name || r.rule_id.slice(0, 8)}</TD>
                        <TD muted>{r.policy_name}</TD>
                        <TD muted>{r.direction}</TD>
                        <TD><QFBadge tone={r.action === 'deny' ? 'bad' : r.action === 'allow' ? 'ok' : 'info'}>{r.action}</QFBadge></TD>
                        <TD mono muted>{r.protocol || 'any'}</TD>
                        <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{r.src_cidrs?.join(', ') || '—'}</TD>
                        <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{r.dst_cidrs?.join(', ') || '—'}</TD>
                        <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{r.src_ports?.join(', ') || '—'}</TD>
                        <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{r.dst_ports?.join(', ') || '—'}</TD>
                      </tr>
                    ))}
                    {ruleset.rules.length === 0 && (
                      <tr><td colSpan={10}>
                        <div style={{ padding: '32px 24px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>
                          No rules applied to this host
                        </div>
                      </td></tr>
                    )}
                  </tbody>
                </QFTable>
              </QFCard>
            </div>
          )}
        </Tabs.Panel>

        <Tabs.Panel value="events" pt="md">
          <div style={{ display: 'flex', gap: 10, marginBottom: 14 }}>
            <div style={{
              display: 'flex', alignItems: 'center', gap: 8, padding: '7px 11px', flex: 1,
              background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
              borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)',
            }}>
              <IconSearch size={14} />
              <input placeholder="Search IP or rule…" value={search} onChange={e => setSearch(e.currentTarget.value)}
                style={{ flex: 1, border: 'none', outline: 'none', background: 'transparent', color: 'var(--qf-fg-1)', fontSize: 'var(--qf-t-base)', fontFamily: 'inherit' }} />
            </div>
            <MSelect placeholder="Action" clearable data={['allow', 'deny', 'log']}
              value={actionFilter} onChange={setActionFilter} w={120} size="sm" />
          </div>
          <QFCard pad={false}>
            <QFTable minWidth={700}>
              <thead>
                <tr style={{ height: 38 }}>
                  <TH w={140}>Time</TH><TH w={80}>Action</TH><TH w={80}>Dir</TH>
                  <TH w={60}>Proto</TH><TH>Src</TH><TH>Dst</TH><TH>Rule</TH>
                </tr>
              </thead>
              <tbody>
                {filteredEvents.map(e => (
                  <tr key={e.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD mono muted>{fmtTime(e.created_at)}</TD>
                    <TD><QFBadge tone={actionTone(e.action) as any}>{e.action}</QFBadge></TD>
                    <TD muted>{e.direction}</TD>
                    <TD mono muted>{protoName(e.protocol)}</TD>
                    <TD mono muted>{e.src_ip ?? '—'}{e.src_port ? `:${e.src_port}` : ''}</TD>
                    <TD mono muted>{e.dst_ip ?? '—'}{e.dst_port ? `:${e.dst_port}` : ''}</TD>
                    <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{ruleLabel(e.rule_id, ruleMap)}</TD>
                  </tr>
                ))}
                {filteredEvents.length === 0 && (
                  <tr><td colSpan={7}>
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>No events</div>
                  </td></tr>
                )}
              </tbody>
            </QFTable>
          </QFCard>
        </Tabs.Panel>

        <Tabs.Panel value="flows" pt="md">
          <QFCard pad={false}>
            <QFTable minWidth={700}>
              <thead>
                <tr style={{ height: 38 }}>
                  <TH w={60}>Proto</TH><TH>Src</TH><TH>Dst</TH>
                  <TH w={80} right>Bytes↑</TH><TH w={80} right>Bytes↓</TH>
                  <TH w={90}>State</TH><TH w={140} right>Time</TH>
                </tr>
              </thead>
              <tbody>
                {flows.map(f => (
                  <tr key={f.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD mono muted>{protoName(f.protocol)}</TD>
                    <TD mono muted>{f.src_ip ?? '—'}{f.src_port ? `:${f.src_port}` : ''}</TD>
                    <TD mono muted>{f.dst_ip ?? '—'}{f.dst_port ? `:${f.dst_port}` : ''}</TD>
                    <TD mono muted right>{f.bytes_orig}</TD>
                    <TD mono muted right>{f.bytes_reply}</TD>
                    <TD muted>{f.final_state ?? '—'}</TD>
                    <TD mono muted right>{fmtTime(f.created_at)}</TD>
                  </tr>
                ))}
                {flows.length === 0 && (
                  <tr><td colSpan={7}>
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>No flows</div>
                  </td></tr>
                )}
              </tbody>
            </QFTable>
          </QFCard>
        </Tabs.Panel>

        <Tabs.Panel value="counters" pt="md">
          <QFCard pad={false}>
            <QFTable>
              <thead>
                <tr style={{ height: 38 }}>
                  <TH>Rule</TH>
                  <TH w={100} right>Packets</TH>
                  <TH w={100} right>Bytes</TH>
                  <TH w={160} right>Timestamp</TH>
                </tr>
              </thead>
              <tbody>
                {counters.map(c => (
                  <tr key={c.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                    <TD mono muted style={{ fontSize: 'var(--qf-t-xs)' }}>{ruleLabel(c.rule_id, ruleMap)}</TD>
                    <TD mono muted right>{c.packets}</TD>
                    <TD mono muted right>{c.bytes}</TD>
                    <TD mono muted right>{fmtDateTime(c.ts)}</TD>
                  </tr>
                ))}
                {counters.length === 0 && (
                  <tr><td colSpan={4}>
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--qf-fg-mute)', fontSize: 'var(--qf-t-sm)' }}>No counters</div>
                  </td></tr>
                )}
              </tbody>
            </QFTable>
          </QFCard>
        </Tabs.Panel>
      </Tabs>

      <Modal opened={editLabelsOpen} onClose={() => setEditLabelsOpen(false)} title="Edit labels">
        <Group gap="xs" style={{ flexDirection: 'column', alignItems: 'stretch' }}>
          {editLabelPairs.map((pair, i) => (
            <Group key={i} gap="xs">
              <TextInput placeholder="key" value={pair.key} size="sm" style={{ flex: 1 }}
                onChange={e => { const n = [...editLabelPairs]; n[i] = { ...pair, key: e.currentTarget.value }; setEditLabelPairs(n) }} />
              <TextInput placeholder="value" value={pair.val} size="sm" style={{ flex: 1 }}
                onChange={e => { const n = [...editLabelPairs]; n[i] = { ...pair, val: e.currentTarget.value }; setEditLabelPairs(n) }} />
              <ActionIcon color="red" variant="subtle" size="sm" onClick={() => setEditLabelPairs(editLabelPairs.filter((_, j) => j !== i))}>
                <IconX size={14} />
              </ActionIcon>
            </Group>
          ))}
          <Button size="xs" variant="subtle" leftSection={<IconPlus size={12} />}
            onClick={() => setEditLabelPairs([...editLabelPairs, { key: '', val: '' }])}>
            Add label
          </Button>
          <Group justify="flex-end" mt="sm">
            <Button variant="subtle" onClick={() => setEditLabelsOpen(false)}>Cancel</Button>
            <Button loading={labelsMut.isPending} onClick={() => labelsMut.mutate(editLabelPairs)}>Save</Button>
          </Group>
        </Group>
      </Modal>
    </>
  )
}

function downloadRuleset(hostname: string, rules: RulesetRuleItem[]) {
  const blob = new Blob([JSON.stringify(rules, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url; a.download = `${hostname}-ruleset.json`; a.click()
  URL.revokeObjectURL(url)
}
