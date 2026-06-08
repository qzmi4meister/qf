import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Modal, Button, Text, Group, Stack, Select, NumberInput, Code, Alert, ActionIcon, TextInput,
} from '@mantine/core'
import { IconPlus, IconTrash, IconKey, IconAlertCircle } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listTokens, createToken, revokeToken } from '../api/misc'
import { useSortState } from '../hooks/useSortState'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import type { Tone } from '../components/QFBadge'

function tokenStatus(t: { uses_count: number; max_uses: number; expires_at: string }): { label: string; tone: Tone } {
  if (t.uses_count >= t.max_uses) return { label: 'revoked', tone: 'bad' }
  const exp = new Date(t.expires_at)
  const now = new Date()
  const diffH = (exp.getTime() - now.getTime()) / 3_600_000
  if (diffH < 0) return { label: 'expired', tone: 'bad' }
  if (diffH < 24) return { label: 'expiring', tone: 'warn' }
  return { label: 'active', tone: 'ok' }
}

export default function Tokens() {
  const qc = useQueryClient()
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

  const snippet = (tok: string) => `curl -fsSL https://<CP_HOST>/install.sh | QF_TOKEN=${tok} sh`

  return (
    <>
      <PageHead
        title="Tokens"
        sub={!isLoading ? `${tokens.length} total` : undefined}
        actions={
          <button
            onClick={() => setModalOpen(true)}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '8px 14px', fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none' }}
          >
            <IconPlus size={14} /> New token
          </button>
        }
      />

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
                    Failed to load tokens.{' '}
                    <button onClick={() => refetch()} style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'inherit' }}>Retry</button>
                  </div>
                </td></tr>
              )
              : rows.length === 0
              ? (
                <tr><td colSpan={6}>
                  <EmptyState
                    icon={<IconKey size={48} />}
                    title="No tokens yet"
                    body="Create an enrollment token to let new hosts join the fleet, or an API token for CI to push bundles."
                    action={
                      <button onClick={() => setModalOpen(true)} style={{ padding: '8px 16px', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600 }}>
                        Create enrollment token
                      </button>
                    }
                  />
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
          Token secrets are shown once at creation and never again. Rotate regularly; revoke immediately if leaked.
        </p>
      )}

      <Modal opened={modalOpen} onClose={() => setModalOpen(false)} title="Create bootstrap token">
        <Stack gap="sm">
          <Select label="Type" data={['single_host', 'bulk']} value={formType} onChange={v => setFormType(v ?? 'single_host')} />
          {formType === 'single_host' && (
            <TextInput label="Target host ID (optional)" value={formHostId} onChange={e => setFormHostId(e.currentTarget.value)} />
          )}
          <Group>
            <TextInput label="Label key" value={formLabel} onChange={e => setFormLabel(e.currentTarget.value)} style={{ flex: 1 }} />
            <TextInput label="Label value" value={formLabelVal} onChange={e => setFormLabelVal(e.currentTarget.value)} style={{ flex: 1 }} />
          </Group>
          <NumberInput label="TTL (seconds)" value={formTTL} onChange={v => setFormTTL(Number(v))} />
          <NumberInput label="Max uses" value={formMaxUses} onChange={v => setFormMaxUses(Number(v))} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button loading={createMut.isPending} onClick={() => createMut.mutate()}>Create</Button>
          </Group>
        </Stack>
      </Modal>

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

      <Modal opened={!!revokeId} onClose={() => setRevokeId(null)} title="Revoke token" size="sm">
        <Stack gap="md">
          <Text>Revoke this token? Enrolled hosts are unaffected.</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setRevokeId(null)}>Cancel</Button>
            <Button color="red" loading={revokeMut.isPending} onClick={() => revokeId && revokeMut.mutate(revokeId)}>Revoke</Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
