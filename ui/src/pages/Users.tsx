import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Button, Table, Text, Group, Badge, ActionIcon, Modal,
  TextInput, PasswordInput, Select, Loader, Center,
} from '@mantine/core'
import { IconPlus, IconEdit, IconTrash } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listUsers, createUser, patchUser, updateUserRole, deleteUser } from '../api/misc'
import { useAuth } from '../hooks/useAuth'
import type { User } from '../types'
import { useSortState } from '../hooks/useSortState'
import { SortTh } from '../components/SortTh'

const ROLES = ['admin', 'editor', 'operator', 'auditor']

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

  const { data: users = [], isLoading } = useQuery({
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

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={2}>Users & Roles</Title>
        <Button leftSection={<IconPlus size={14} />} onClick={() => {
          setFormEmail(''); setFormPassword(''); setFormRole('auditor')
          setCreateOpen(true)
        }}>
          New user
        </Button>
      </Group>

      <Table highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <SortTh sortKey="email" sort={sort} onSort={toggle}>Email</SortTh>
            <SortTh sortKey="role" sort={sort} onSort={toggle}>Role</SortTh>
            <Table.Th>Status</Table.Th>
            <Table.Th>Type</Table.Th>
            <SortTh sortKey="last_login_at" sort={sort} onSort={toggle}>Last login</SortTh>
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {rows.map((u) => (
            <Table.Tr key={u.id}>
              <Table.Td>{u.email}</Table.Td>
              <Table.Td>
                <Badge size="sm" color={u.role === 'admin' ? 'red' : u.role === 'editor' ? 'blue' : 'gray'}>
                  {u.role ?? '—'}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Badge size="sm" color={u.status === 'active' ? 'green' : 'gray'}>{u.status}</Badge>
              </Table.Td>
              <Table.Td>
                {u.is_oidc ? <Badge size="xs" variant="outline">OIDC</Badge> : <Text size="sm">Local</Text>}
              </Table.Td>
              <Table.Td>
                <Text size="sm">{fmtDateTime(u.last_login_at)}</Text>
              </Table.Td>
              <Table.Td>
                <Group gap={4}>
                  <ActionIcon
                    size="sm"
                    variant="subtle"
                    onClick={() => {
                      setEditUser(u)
                      setEditRole(u.role ?? 'auditor')
                      setEditPassword('')
                    }}
                  >
                    <IconEdit size={12} />
                  </ActionIcon>
                  {u.id !== me?.id && (
                    <ActionIcon size="sm" variant="subtle" color="red" onClick={() => setDeleteId(u.id)}>
                      <IconTrash size={12} />
                    </ActionIcon>
                  )}
                </Group>
              </Table.Td>
            </Table.Tr>
          ))}
          {users.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={6}>
                <Text c="dimmed" ta="center" size="sm" py="md">No users</Text>
              </Table.Td>
            </Table.Tr>
          )}
        </Table.Tbody>
      </Table>

      <Modal opened={createOpen} onClose={() => setCreateOpen(false)} title="Create user">
        <Stack gap="sm">
          <TextInput label="Email" type="email" value={formEmail} onChange={(e) => setFormEmail(e.currentTarget.value)} required />
          <PasswordInput label="Password" value={formPassword} onChange={(e) => setFormPassword(e.currentTarget.value)} required />
          <Select label="Role" data={ROLES} value={formRole} onChange={(v) => setFormRole(v ?? 'auditor')} />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button loading={createMut.isPending} onClick={() => createMut.mutate()}>Create</Button>
          </Group>
        </Stack>
      </Modal>

      <Modal opened={!!editUser} onClose={() => setEditUser(null)} title="Edit user">
        <Stack gap="sm">
          <Text size="sm" c="dimmed">{editUser?.email}</Text>
          <Select label="Role" data={ROLES} value={editRole} onChange={(v) => setEditRole(v ?? 'auditor')} />
          <PasswordInput label="New password (optional)" value={editPassword} onChange={(e) => setEditPassword(e.currentTarget.value)} />
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
            <Button color="red" loading={deleteMut.isPending} onClick={() => deleteId && deleteMut.mutate(deleteId)}>
              Delete
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
