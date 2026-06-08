import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Modal, Button, Text, Group, Stack, Select, TextInput, PasswordInput, ActionIcon,
} from '@mantine/core'
import { IconPlus, IconEdit, IconTrash, IconUsers } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listUsers, createUser, patchUser, updateUserRole, deleteUser } from '../api/misc'
import { useAuth } from '../hooks/useAuth'
import type { User } from '../types'
import { useSortState } from '../hooks/useSortState'
import PageHead from '../components/PageHead'
import QFCard from '../components/QFCard'
import QFBadge from '../components/QFBadge'
import EmptyState from '../components/EmptyState'
import { QFTable, TH, TD, SortTH } from '../components/QFTable'
import { SkeletonRow } from '../components/Skeleton'
import type { Tone } from '../components/QFBadge'

const ROLES = ['admin', 'editor', 'operator', 'auditor']
const ROLE_TONE: Record<string, Tone> = { admin: 'pol', editor: 'info', operator: 'warn', auditor: 'neutral' }

function UserAvatar({ user, size = 30 }: { user: User; size?: number }) {
  const pending = user.status === 'pending'
  const isOidc = user.is_oidc
  const initials = user.email.split('@')[0].split(/[.\-_]/).map(s => s[0]).join('').toUpperCase().slice(0, 2)
  return (
    <span style={{
      width: size, height: size, borderRadius: '50%', flexShrink: 0,
      background: pending ? 'var(--qf-bg-muted)' : 'var(--qf-indigo-600)',
      color: pending ? 'var(--qf-fg-mute)' : '#fff',
      display: 'grid', placeItems: 'center',
      fontSize: size > 24 ? 11 : 9, fontWeight: 700,
    }}>{initials || '?'}</span>
  )
}

export default function Users() {
  const { user: me } = useAuth()
  const qc = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [editUser, setEditUser] = useState<User | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const [formEmail, setFormEmail] = useState('')
  const [formPassword, setFormPassword] = useState('')
  const [formRole, setFormRole] = useState('auditor')
  const [editRole, setEditRole] = useState('')
  const [editPassword, setEditPassword] = useState('')

  const { data: users = [], isLoading, isError, refetch } = useQuery({
    queryKey: ['users'],
    queryFn: listUsers,
  })
  const { sort, toggle, sorted } = useSortState({ key: 'email', dir: 'asc' })
  const rows = sorted(users, (u, k) => {
    if (k === 'email') return u.email
    if (k === 'role') return u.role ?? ''
    if (k === 'last_login_at') return u.last_login_at ?? ''
    return undefined
  })

  const createMut = useMutation({
    mutationFn: () => createUser({ email: formEmail, password: formPassword, role: formRole }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      setCreateOpen(false)
      notifications.show({ message: 'User created', color: 'green' })
    },
    onError: () => notifications.show({ message: 'Create failed', color: 'red' }),
  })

  const editMut = useMutation({
    mutationFn: async () => {
      if (!editUser) return
      if (editPassword) await patchUser(editUser.id, { password: editPassword })
      if (editRole && editRole !== editUser.role) await updateUserRole(editUser.id, editRole)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      setEditUser(null)
      notifications.show({ message: 'User updated', color: 'green' })
    },
  })

  const deleteMut = useMutation({
    mutationFn: deleteUser,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      setDeleteId(null)
      notifications.show({ message: 'User deleted', color: 'green' })
    },
  })

  if (me?.role !== 'admin') {
    return <Text c="red">Admin access required</Text>
  }

  return (
    <>
      <PageHead
        title="Users"
        sub={!isLoading ? `${users.length} total` : undefined}
        actions={
          <button
            onClick={() => { setFormEmail(''); setFormPassword(''); setFormRole('auditor'); setCreateOpen(true) }}
            style={{ display: 'inline-flex', alignItems: 'center', gap: 6, padding: '8px 14px', fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit', cursor: 'pointer', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none' }}
          >
            <IconPlus size={14} /> Invite user
          </button>
        }
      />

      <QFCard pad={false}>
        <QFTable minWidth={700}>
          <thead>
            <tr style={{ height: 38 }}>
              <SortTH sortKey="email" sort={sort} onSort={toggle}>User</SortTH>
              <SortTH sortKey="role" sort={sort} onSort={toggle} w={120}>Role</SortTH>
              <TH w={100}>Auth</TH>
              <SortTH sortKey="last_login_at" sort={sort} onSort={toggle} w={140}>Last login</SortTH>
              <TH w={60} />
            </tr>
          </thead>
          <tbody>
            {isLoading
              ? Array(5).fill(0).map((_, i) => <SkeletonRow key={i} cols={5} />)
              : isError
              ? (
                <tr><td colSpan={5}>
                  <div style={{ padding: 20, textAlign: 'center', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)', fontFamily: 'var(--qf-mono)' }}>
                    Failed to load users.{' '}
                    <button onClick={() => refetch()} style={{ background: 'none', border: 'none', color: 'var(--qf-brand)', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'inherit' }}>Retry</button>
                  </div>
                </td></tr>
              )
              : rows.length === 0
              ? (
                <tr><td colSpan={5}>
                  <EmptyState
                    icon={<IconUsers size={48} />}
                    title="Just you so far"
                    body="Invite teammates and assign roles — Admins manage everything, Editors author policies, Viewers have read-only access."
                    action={
                      <button onClick={() => { setFormEmail(''); setFormPassword(''); setFormRole('auditor'); setCreateOpen(true) }} style={{ padding: '8px 16px', borderRadius: 'var(--qf-r-md)', background: 'var(--qf-brand-solid)', color: '#fff', border: 'none', cursor: 'pointer', fontFamily: 'inherit', fontSize: 'var(--qf-t-base)', fontWeight: 600 }}>
                        Invite user
                      </button>
                    }
                  />
                </td></tr>
              )
              : rows.map(u => (
                <tr key={u.id} className="qf-row" style={{ borderTop: '1px solid var(--qf-border-2)' }}>
                  <td style={{ padding: '0 12px', height: 44 }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 11 }}>
                      <UserAvatar user={u} />
                      <div>
                        <div style={{ fontSize: 'var(--qf-t-md)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>{u.email.split('@')[0]}</div>
                        <div style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', fontFamily: 'var(--qf-mono)' }}>{u.email}</div>
                      </div>
                    </div>
                  </td>
                  <TD><QFBadge tone={ROLE_TONE[u.role ?? ''] ?? 'neutral'}>{u.role ?? '—'}</QFBadge></TD>
                  <TD><QFBadge tone={u.is_oidc ? 'info' : 'neutral'}>{u.is_oidc ? 'SSO' : 'Local'}</QFBadge></TD>
                  <TD mono muted>{fmtDateTime(u.last_login_at)}</TD>
                  <td style={{ padding: '0 12px', height: 44, textAlign: 'right' }}>
                    <Group gap={4} justify="flex-end">
                      <ActionIcon size="sm" variant="subtle" onClick={() => { setEditUser(u); setEditRole(u.role ?? 'auditor'); setEditPassword('') }}>
                        <IconEdit size={12} />
                      </ActionIcon>
                      {u.id !== me?.id && (
                        <ActionIcon size="sm" variant="subtle" color="red" onClick={() => setDeleteId(u.id)}>
                          <IconTrash size={12} />
                        </ActionIcon>
                      )}
                    </Group>
                  </td>
                </tr>
              ))
            }
          </tbody>
        </QFTable>
      </QFCard>

      <Modal opened={createOpen} onClose={() => setCreateOpen(false)} title="Invite user">
        <Stack gap="sm">
          <TextInput label="Email" type="email" value={formEmail} onChange={e => setFormEmail(e.currentTarget.value)} required />
          <PasswordInput label="Password" value={formPassword} onChange={e => setFormPassword(e.currentTarget.value)} required />
          <Select label="Role" data={ROLES} value={formRole} onChange={v => setFormRole(v ?? 'auditor')} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button loading={createMut.isPending} onClick={() => createMut.mutate()}>Create</Button>
          </Group>
        </Stack>
      </Modal>

      <Modal opened={!!editUser} onClose={() => setEditUser(null)} title="Edit user">
        <Stack gap="sm">
          <Text size="sm" c="dimmed">{editUser?.email}</Text>
          <Select label="Role" data={ROLES} value={editRole} onChange={v => setEditRole(v ?? 'auditor')} />
          <PasswordInput label="New password (optional)" value={editPassword} onChange={e => setEditPassword(e.currentTarget.value)} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setEditUser(null)}>Cancel</Button>
            <Button loading={editMut.isPending} onClick={() => editMut.mutate()}>Save</Button>
          </Group>
        </Stack>
      </Modal>

      <Modal opened={!!deleteId} onClose={() => setDeleteId(null)} title="Delete user" size="sm">
        <Stack gap="md">
          <Text>Delete this user?</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setDeleteId(null)}>Cancel</Button>
            <Button color="red" loading={deleteMut.isPending} onClick={() => deleteId && deleteMut.mutate(deleteId)}>Delete</Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
