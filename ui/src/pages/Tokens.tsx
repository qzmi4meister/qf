import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Modal, Button, Text, Group, Stack, Select, NumberInput, Code, Alert, ActionIcon, TextInput,
} from '@mantine/core'
import { IconPlus, IconTrash, IconKey, IconAlertCircle } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listTokens, createToken, revokeToken, listAPITokens, createAPIToken, deleteAPIToken } from '../api/misc'
import { listHosts } from '../api/hosts'
import { useSortState } from '../hooks/useSortState'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import type { Tone } from '../components/QFBadge'
import type { APIToken } from '../types'

function tokenStatus(t: { uses_count: number; max_uses: number; expires_at: string }): { label: string; tone: Tone } {
  if (t.uses_count >= t.max_uses) return { label: 'revoked', tone: 'bad' }
  const exp = new Date(t.expires_at)
  const now = new Date()
  const diffH = (exp.getTime() - now.getTime()) / 3_600_000
  if (diffH < 0) return { label: 'expired', tone: 'bad' }
  if (diffH < 24) return { label: 'expiring', tone: 'warn' }
  return { label: 'active', tone: 'ok' }
}

function apiTokenStatus(t: APIToken): { label: string; tone: Tone } {
  if (!t.expires_at) return { label: 'active', tone: 'ok' }
  const diffH = (new Date(t.expires_at).getTime() - Date.now()) / 3_600_000
  if (diffH < 0) return { label: 'expired', tone: 'bad' }
  if (diffH < 24) return { label: 'expiring', tone: 'warn' }
  return { label: 'active', tone: 'ok' }
}

type Tab = 'enrollment' | 'api'

export default function Tokens() {
  const qc = useQueryClient()
  const [tab, setTab] = useState<Tab>('enrollment')

  /* ── Enrollment tokens ── */
  const [modalOpen, setModalOpen] = useState(false)
  const [revokeId, setRevokeId] = useState<string | null>(null)
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const [formType, setFormType] = useState('single_host')
  const [formHostId, setFormHostId] = useState('')
  const [formLabel, setFormLabel] = useState('')
  const [formLabelVal, setFormLabelVal] = useState('')
  const [formTTL, setFormTTL] = useState(86400)
  const [formMaxUses, setFormMaxUses] = useState(1)

  const { data: tokens = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['tokens'],
    queryFn: listTokens,
  })
  const { data: hosts = [] } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
  })
  const { sort, toggle, sorted } = useSortState({ key: 'expires_at', dir: 'asc' })
  const rows = sorted(tokens, (t, k) => {
    if (k === 'expires_at') return t.expires_at
    if (k === 'type') return t.type
    return undefined
  })

  const createMut = useMutation({
    mutationFn: () => createToken({
      type: formType,
      target_host_id: formHostId || undefined,
      label_template: formLabel ? { [formLabel]: formLabelVal } : undefined,
      ttl_seconds: formTTL,
      max_uses: formMaxUses,
    }),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setModalOpen(false)
      if (t.token) setCreatedToken(t.token)
    },
    onError: () => notifications.show({ message: 'Create failed', color: 'red' }),
  })

  const revokeMut = useMutation({
    mutationFn: revokeToken,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setRevokeId(null)
      notifications.show({ message: 'Token revoked', color: 'green' })
    },
  })

  /* ── API Tokens ── */
  const [apiModalOpen, setApiModalOpen] = useState(false)
  const [apiDeleteId, setApiDeleteId] = useState<string | null>(null)
  const [createdApiToken, setCreatedApiToken] = useState<string | null>(null)
  const [apiName, setApiName] = useState('')
  const [apiRole, setApiRole] = useState('auditor')
  const [apiExpires, setApiExpires] = useState('')

  const { data: apiTokens = [], isLoading: apiLoading, isError: apiError, refetch: apiRefetch } = useQuery({
    queryKey: ['api-tokens'],
    queryFn: listAPITokens,
  })

  const createApiMut = useMutation({
    mutationFn: () => createAPIToken({
      name: apiName,
      role: apiRole,
      expires_at: apiExpires || undefined,
    }),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['api-tokens'] })
      setApiModalOpen(false)
      setApiName(''); setApiRole('auditor'); setApiExpires('')
      if (t.token) setCreatedApiToken(t.token)
    },
    onError: () => notifications.show({ message: 'Create failed', color: 'red' }),
  })

  const deleteApiMut = useMutation({
    mutationFn: deleteAPIToken,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['api-tokens'] })
      setApiDeleteId(null)
      notifications.show({ message: 'API token deleted', color: 'green' })
    },
  })

  const snippet = (tok: string) => `curl -fsSL https://<CP_HOST>/install.sh | QF_TOKEN=${tok} sh`

  const tabStyle = (t: Tab): React.CSSProperties => ({
    padding: '7px 16px', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600,
    cursor: 'pointer', border: 'none', background: 'transparent',
    borderBottom: tab === t ? '2px solid var(--qf-brand-solid)' : '2px solid transparent',
    color: tab === t ? 'var(--qf-fg-1)' : 'var(--qf-fg-mute)',
  })

  return (
    <>
      <PageHead
        title="Tokens"
        actions={
          <button
            onClick={() => tab === 'enrollment' ? setModalOpen(true) : setApiModalOpen(true)}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '8px 14px', fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none' }}
          >
            <IconPlus size={14} /> New token
          </button>
        }
      />

      <div style={{ display: 'flex', borderBottom: '1px solid var(--qf-border-1)', marginBottom: 16 }}>
        <button style={tabStyle('enrollment')} onClick={() => setTab('enrollment')}>Enrollment</button>
        <button style={tabStyle('api')} onClick={() => setTab('api')}>API Tokens</button>
      </div>

      {tab === 'enrollment' && (
        <>
          <QFCard pad={false}>
            <QFTable minWidth={700}>
              <thead>
                <tr style={{ height: 38 }}>
                  <TH>Name</TH>
                  <SortTH sortKey="type" sort={sort} onSort={toggle} w={120}>Type</SortTH>
                  <TH w={140}>Labels</TH>
                  <TH w={80} right>Uses</TH>
                  <SortTH sortKey="expires_at" sort={sort} onSort={toggle} w={140}>Expires</SortTH>
                  <TH w={40} />
                </tr>
              </thead>
              <tbody>
                {isLoading
                  ? Array(5).fill(0).map((_, i) => <SkeletonRow key={i} cols={6} />)
                  : isError
                  ? (
                    <tr><td colSpan={6}>
                      <div style={{ padding: 20, textAlign: 'center', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)', fontFamily: 'var(--qf-mono)' }}>
                        Failed to load.{' '}
                        <button onClick={() => refetch()} style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'inherit' }}>Retry</button>
                      </div>
                    </td></tr>
                  )
                  : rows.length === 0
                  ? (
                    <tr><td colSpan={6}>
                      <EmptyState icon={<IconKey size={48} />} title="No enrollment tokens" body="Create a token to let new hosts join the fleet." />
                    </td></tr>
                  )
                  : rows.map(t => {
                    const st = tokenStatus(t)
                    const revoked = st.label === 'revoked' || st.label === 'expired'
                    return (
                      <tr key={t.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)', opacity: revoked ? 0.6 : 1 }}>
                        <td style={{ padding: '0 12px', height: 40 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                            <IconKey size={14} color="var(--qf-fg-mute)" />
                            <span style={{ fontFamily: 'var(--qf-mono)', color: 'var(--qf-fg-1)', fontWeight: 500, textDecoration: revoked ? 'line-through' : 'none', fontSize: 'var(--qf-t-base)' }}>
                              {t.id.slice(0, 12)}
                            </span>
                            <QFBadge tone={st.tone}>{st.label}</QFBadge>
                          </div>
                        </td>
                        <TD mono muted>{t.type}</TD>
                        <TD>
                          {t.label_template && Object.entries(t.label_template).map(([k, v]) => (
                            <QFBadge key={k} tone="neutral">{k}={v as string}</QFBadge>
                          ))}
                        </TD>
                        <TD mono muted right>{t.uses_count}/{t.max_uses}</TD>
                        <TD mono muted>{fmtDateTime(t.expires_at)}</TD>
                        <TD right>
                          <ActionIcon variant="subtle" color="red" size="sm" onClick={() => setRevokeId(t.id)}>
                            <IconTrash size={13} />
                          </ActionIcon>
                        </TD>
                      </tr>
                    )
                  })
                }
              </tbody>
            </QFTable>
          </QFCard>
          {rows.length > 0 && (
            <p style={{ margin: '14px 2px 0', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-faint)' }}>
              Token secrets shown once at creation. Rotate regularly; revoke immediately if leaked.
            </p>
          )}
        </>
      )}

      {tab === 'api' && (
        <QFCard pad={false}>
          <QFTable minWidth={600}>
            <thead>
              <tr style={{ height: 38 }}>
                <TH>Name</TH>
                <TH w={100}>Role</TH>
                <TH w={80}>Status</TH>
                <TH w={140}>Last used</TH>
                <TH w={140}>Expires</TH>
                <TH w={40} />
              </tr>
            </thead>
            <tbody>
              {apiLoading
                ? Array(3).fill(0).map((_, i) => <SkeletonRow key={i} cols={6} />)
                : apiError
                ? (
                  <tr><td colSpan={6}>
                    <div style={{ padding: 20, textAlign: 'center', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)', fontFamily: 'var(--qf-mono)' }}>
                      Failed to load.{' '}
                      <button onClick={() => apiRefetch()} style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'inherit' }}>Retry</button>
                    </div>
                  </td></tr>
                )
                : apiTokens.length === 0
                ? (
                  <tr><td colSpan={6}>
                    <EmptyState icon={<IconKey size={48} />} title="No API tokens" body="Create an API token for CI pipelines and automation." />
                  </td></tr>
                )
                : apiTokens.map(t => {
                  const st = apiTokenStatus(t)
                  return (
                    <tr key={t.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)', opacity: st.label === 'expired' ? 0.6 : 1 }}>
                      <td style={{ padding: '0 12px', height: 40 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                          <IconKey size={14} color="var(--qf-fg-mute)" />
                          <span style={{ color: 'var(--qf-fg-1)', fontWeight: 500, fontSize: 'var(--qf-t-base)' }}>{t.name}</span>
                        </div>
                      </td>
                      <TD><QFBadge tone="info">{t.role}</QFBadge></TD>
                      <TD><QFBadge tone={st.tone}>{st.label}</QFBadge></TD>
                      <TD mono muted>{t.last_used_at ? fmtDateTime(t.last_used_at) : '—'}</TD>
                      <TD mono muted>{t.expires_at ? fmtDateTime(t.expires_at) : '—'}</TD>
                      <TD right>
                        <ActionIcon variant="subtle" color="red" size="sm" onClick={() => setApiDeleteId(t.id)}>
                          <IconTrash size={13} />
                        </ActionIcon>
                      </TD>
                    </tr>
                  )
                })
              }
            </tbody>
          </QFTable>
        </QFCard>
      )}

      {/* Create enrollment token */}
      <Modal opened={modalOpen} onClose={() => setModalOpen(false)} title="Create enrollment token">
        <Stack gap="sm">
          <Select label="Type" data={['single_host', 'bulk']} value={formType} onChange={v => setFormType(v ?? 'single_host')} />
          {formType === 'single_host' && (
            <Select
              label="Target host"
              required
              placeholder="Select host..."
              data={hosts.map(h => ({ value: h.id, label: h.hostname }))}
              value={formHostId || null}
              onChange={v => setFormHostId(v ?? '')}
              searchable
            />
          )}
          <Group>
            <TextInput label="Label key" value={formLabel} onChange={e => setFormLabel(e.currentTarget.value)} style={{ flex: 1 }} />
            <TextInput label="Label value" value={formLabelVal} onChange={e => setFormLabelVal(e.currentTarget.value)} style={{ flex: 1 }} />
          </Group>
          <NumberInput label="TTL (seconds)" value={formTTL} onChange={v => setFormTTL(Number(v))} />
          <NumberInput label="Max uses" value={formMaxUses} onChange={v => setFormMaxUses(Number(v))} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button loading={createMut.isPending} onClick={() => {
              if (formType === 'single_host' && !formHostId) {
                notifications.show({ message: 'Select a target host', color: 'red' })
                return
              }
              createMut.mutate()
            }}>Create</Button>
          </Group>
        </Stack>
      </Modal>

      {/* Create API token */}
      <Modal opened={apiModalOpen} onClose={() => setApiModalOpen(false)} title="Create API token">
        <Stack gap="sm">
          <TextInput label="Name" placeholder="ci-deploy" required value={apiName} onChange={e => setApiName(e.currentTarget.value)} />
          <Select label="Role" data={['auditor', 'editor', 'admin']} value={apiRole} onChange={v => setApiRole(v ?? 'auditor')} />
          <TextInput label="Expires at (optional)" placeholder="2027-01-01T00:00:00Z" value={apiExpires} onChange={e => setApiExpires(e.currentTarget.value)} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setApiModalOpen(false)}>Cancel</Button>
            <Button loading={createApiMut.isPending} onClick={() => {
              if (!apiName.trim()) {
                notifications.show({ message: 'Name is required', color: 'red' })
                return
              }
              createApiMut.mutate()
            }}>Create</Button>
          </Group>
        </Stack>
      </Modal>

      {/* Token created — show secret */}
      <Modal opened={!!createdToken} onClose={() => setCreatedToken(null)} title="Token created" size="lg">
        <Stack gap="md">
          <Alert icon={<IconAlertCircle size={16} />} color="yellow">
            Copy this token now. It will not be shown again.
          </Alert>
          <Code block>{createdToken}</Code>
          <Text fw={600}>Install snippet:</Text>
          <Code block>{snippet(createdToken ?? '')}</Code>
          <Button onClick={() => setCreatedToken(null)}>Close</Button>
        </Stack>
      </Modal>

      {/* API token created */}
      <Modal opened={!!createdApiToken} onClose={() => setCreatedApiToken(null)} title="API token created" size="lg">
        <Stack gap="md">
          <Alert icon={<IconAlertCircle size={16} />} color="yellow">
            Copy this token now. It will not be shown again.
          </Alert>
          <Code block>{createdApiToken}</Code>
          <Button onClick={() => setCreatedApiToken(null)}>Close</Button>
        </Stack>
      </Modal>

      {/* Revoke enrollment token */}
      <Modal opened={!!revokeId} onClose={() => setRevokeId(null)} title="Revoke token" size="sm">
        <Stack gap="md">
          <Text>Revoke this token? Enrolled hosts are unaffected.</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setRevokeId(null)}>Cancel</Button>
            <Button color="red" loading={revokeMut.isPending} onClick={() => revokeId && revokeMut.mutate(revokeId)}>Revoke</Button>
          </Group>
        </Stack>
      </Modal>

      {/* Delete API token */}
      <Modal opened={!!apiDeleteId} onClose={() => setApiDeleteId(null)} title="Delete API token" size="sm">
        <Stack gap="md">
          <Text>Permanently delete this API token?</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setApiDeleteId(null)}>Cancel</Button>
            <Button color="red" loading={deleteApiMut.isPending} onClick={() => apiDeleteId && deleteApiMut.mutate(apiDeleteId)}>Delete</Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
