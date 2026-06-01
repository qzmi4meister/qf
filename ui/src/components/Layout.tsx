import { Outlet, NavLink as RouterNavLink, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import {
  AppShell,
  NavLink,
  Group,
  Text,
  Avatar,
  Menu,
  ScrollArea,
  Burger,
  Stack,
  Modal,
  PasswordInput,
  Button,
  Alert,
} from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import {
  IconLayoutDashboard,
  IconServer,
  IconShield,
  IconBoxMultiple,
  IconActivity,
  IconArrowsExchange,
  IconClipboardList,
  IconSettings,
  IconUsers,
  IconKey,
  IconLogout,
  IconUser,
  IconShieldLock,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'
import client from '../api/client'

const NAV = [
  { label: 'Dashboard', icon: IconLayoutDashboard, to: '/dashboard' },
  { label: 'Hosts', icon: IconServer, to: '/hosts' },
  { label: 'Policies', icon: IconShield, to: '/policies' },
  { label: 'Object Groups', icon: IconBoxMultiple, to: '/object-groups' },
  { label: 'Default Policy', icon: IconSettings, to: '/default-policy' },
  { label: 'Events', icon: IconActivity, to: '/events' },
  { label: 'Flows', icon: IconArrowsExchange, to: '/flows' },
  { label: 'Audit Log', icon: IconClipboardList, to: '/audit' },
  { label: 'Tokens', icon: IconKey, to: '/tokens' },
  { label: 'Users', icon: IconUsers, to: '/users' },
]

function ProfileModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function handleSubmit() {
    setError('')
    if (next.length < 8) { setError('New password must be at least 8 characters'); return }
    if (next !== confirm) { setError('Passwords do not match'); return }
    setLoading(true)
    try {
      await client.post('/auth/change-password', { current_password: current, new_password: next })
      notifications.show({ message: 'Password changed', color: 'green' })
      setCurrent(''); setNext(''); setConfirm('')
      onClose()
    } catch (e: any) {
      setError(e?.response?.data ?? 'Failed to change password')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal opened={opened} onClose={onClose} title="Change password" size="sm">
      <Stack gap="sm">
        {error && <Alert color="red">{error}</Alert>}
        <PasswordInput label="Current password" value={current} onChange={e => setCurrent(e.target.value)} />
        <PasswordInput label="New password" value={next} onChange={e => setNext(e.target.value)} />
        <PasswordInput label="Confirm new password" value={confirm} onChange={e => setConfirm(e.target.value)} />
        <Button onClick={handleSubmit} loading={loading}>Change password</Button>
      </Stack>
    </Modal>
  )
}

export default function Layout() {
  const [opened, { toggle }] = useDisclosure()
  const [profileOpened, { open: openProfile, close: closeProfile }] = useDisclosure(false)
  const { user } = useAuth()
  const navigate = useNavigate()
  const qc = useQueryClient()

  async function handleLogout() {
    await logout()
    qc.clear()
    navigate('/login')
  }

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
      <ProfileModal opened={profileOpened} onClose={closeProfile} />

      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group gap="sm">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Group gap={6}>
              <IconShieldLock size={22} color="var(--mantine-color-blue-6)" />
              <Text fw={700} size="sm">qf</Text>
            </Group>
            <Text size="sm" c="dimmed">Default</Text>
          </Group>

          <Menu shadow="md" width={180}>
            <Menu.Target>
              <Avatar style={{ cursor: 'pointer' }} size="sm" radius="xl" color="blue">
                {user?.email?.[0]?.toUpperCase() ?? 'U'}
              </Avatar>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>{user?.email}</Menu.Label>
              <Menu.Label c="dimmed">{user?.role}</Menu.Label>
              <Menu.Divider />
              <Menu.Item leftSection={<IconUser size={14} />} onClick={openProfile}>
                Profile
              </Menu.Item>
              <Menu.Item
                leftSection={<IconLogout size={14} />}
                color="red"
                onClick={handleLogout}
              >
                Sign out
              </Menu.Item>
            </Menu.Dropdown>
          </Menu>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar p="xs">
        <ScrollArea>
          <Stack gap={2}>
            {NAV.map((item) => (
              <NavLink
                key={item.to}
                component={RouterNavLink}
                to={item.to}
                label={item.label}
                leftSection={<item.icon size={16} />}
                style={({ isActive }: { isActive: boolean }) => ({
                  borderRadius: 'var(--mantine-radius-sm)',
                  fontWeight: isActive ? 600 : undefined,
                })}
              />
            ))}
          </Stack>
        </ScrollArea>
      </AppShell.Navbar>

      <AppShell.Main>
        <Outlet />
      </AppShell.Main>
    </AppShell>
  )
}
