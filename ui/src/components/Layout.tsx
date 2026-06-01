import { Outlet, NavLink as RouterNavLink, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
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
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'

const NAV = [
  { label: 'Dashboard', icon: IconLayoutDashboard, to: '/app/dashboard' },
  { label: 'Hosts', icon: IconServer, to: '/app/hosts' },
  { label: 'Policies', icon: IconShield, to: '/app/policies' },
  { label: 'Object Groups', icon: IconBoxMultiple, to: '/app/object-groups' },
  { label: 'Default Policy', icon: IconSettings, to: '/app/default-policy' },
  { label: 'Events', icon: IconActivity, to: '/app/events' },
  { label: 'Flows', icon: IconArrowsExchange, to: '/app/flows' },
  { label: 'Audit Log', icon: IconClipboardList, to: '/app/audit' },
  { label: 'Tokens', icon: IconKey, to: '/app/tokens' },
  { label: 'Users', icon: IconUsers, to: '/app/users' },
]

export default function Layout() {
  const [opened, { toggle }] = useDisclosure()
  const { user } = useAuth()
  const navigate = useNavigate()
  const qc = useQueryClient()

  async function handleLogout() {
    await logout()
    qc.clear()
    navigate('/app/login')
  }

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
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
              <Menu.Item leftSection={<IconUser size={14} />}>Profile</Menu.Item>
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
