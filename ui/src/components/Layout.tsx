import { Outlet, NavLink as RouterNavLink, useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useState, useRef, useEffect, useCallback } from 'react'
import {
  AppShell,
  Burger,
  ScrollArea,
  Modal,
  PasswordInput,
  Button,
  Alert,
  Stack,
  useMantineColorScheme,
  useComputedColorScheme,
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
  IconSearch,
  IconSun,
  IconMoon,
  IconChevronDown,
} from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useQuery } from '@tanstack/react-query'
import { logout } from '../api/auth'
import { useAuth } from '../hooks/useAuth'
import { useWebSocket } from '../hooks/useWebSocket'
import client from '../api/client'
import { listHosts } from '../api/hosts'
import { listPolicies } from '../api/policies'
import Mark from './Mark'

const NAV = [
  { label: 'Dashboard',    icon: IconLayoutDashboard, to: '/dashboard' },
  { label: 'Hosts',        icon: IconServer,           to: '/hosts' },
  { label: 'Policies',     icon: IconShield,           to: '/policies' },
  { label: 'Object Groups',icon: IconBoxMultiple,      to: '/object-groups' },
  { label: 'Default Policy',icon: IconSettings,        to: '/default-policy' },
  { label: 'Events',       icon: IconActivity,         to: '/events' },
  { label: 'Flows',        icon: IconArrowsExchange,   to: '/flows' },
  { label: 'Audit Log',    icon: IconClipboardList,    to: '/audit' },
  { label: 'Tokens',       icon: IconKey,              to: '/tokens' },
  { label: 'Users',        icon: IconUsers,            to: '/users' },
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

function UserMenu({ email, onProfile, onSignOut }: { email: string; onProfile: () => void; onSignOut: () => void }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const h = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', h)
    return () => document.removeEventListener('mousedown', h)
  }, [])

  const [local] = email.split('@')
  const initials = local.slice(0, 2).toUpperCase()

  return (
    <div ref={ref} style={{ position: 'relative' }}>
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          width: 30, height: 30, borderRadius: '50%',
          background: 'var(--qf-indigo-600)', color: '#fff',
          display: 'grid', placeItems: 'center',
          fontSize: 11, fontWeight: 700, border: 'none', cursor: 'pointer',
        }}
      >
        {initials}
      </button>
      {open && (
        <div style={{
          position: 'absolute', top: 38, right: 0, width: 210,
          background: 'var(--qf-bg-raised)', border: '1px solid var(--qf-border-1)',
          borderRadius: 'var(--qf-r-lg)', boxShadow: 'var(--qf-sh-lg)',
          padding: 6, zIndex: 50,
        }}>
          <div style={{ padding: '8px 10px', borderBottom: '1px solid var(--qf-border-2)', marginBottom: 4 }}>
            <div style={{ fontSize: 'var(--qf-t-base)', fontWeight: 600, color: 'var(--qf-fg-1)' }}>{email}</div>
          </div>
          <button onClick={() => { setOpen(false); onProfile() }} style={menuItemStyle}>
            <IconUser size={15} style={{ color: 'var(--qf-fg-mute)' }} />
            Change password
          </button>
          <div style={{ borderTop: '1px solid var(--qf-border-2)', margin: '4px 0' }} />
          <button onClick={() => { setOpen(false); onSignOut() }} style={{ ...menuItemStyle, color: 'var(--qf-bad-fg)' }}>
            <IconLogout size={15} />
            Sign out
          </button>
        </div>
      )}
    </div>
  )
}

function GlobalSearch() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const { data: hosts = [] } = useQuery({ queryKey: ['hosts'], queryFn: listHosts, staleTime: 30_000 })
  const { data: policies = [] } = useQuery({ queryKey: ['policies'], queryFn: listPolicies, staleTime: 30_000 })

  const q = query.trim().toLowerCase()
  const matchedHosts = q ? hosts.filter(h => h.hostname.toLowerCase().includes(q)).slice(0, 5) : []
  const matchedPolicies = q ? policies.filter(p => p.name.toLowerCase().includes(q)).slice(0, 5) : []
  const hasResults = matchedHosts.length > 0 || matchedPolicies.length > 0

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [])

  const go = useCallback((path: string) => {
    setQuery('')
    setOpen(false)
    navigate(path)
  }, [navigate])

  return (
    <div ref={wrapRef} style={{ position: 'relative', width: 230 }}>
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '7px 11px',
        background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
        borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)',
      }}>
        <IconSearch size={14} style={{ flexShrink: 0 }} />
        <input
          ref={inputRef}
          value={query}
          onChange={e => { setQuery(e.currentTarget.value); setOpen(true) }}
          onFocus={() => setOpen(true)}
          onKeyDown={e => { if (e.key === 'Escape') { setQuery(''); setOpen(false) } }}
          placeholder="Search hosts, policies…"
          style={{
            flex: 1, border: 'none', outline: 'none',
            background: 'transparent', color: 'var(--qf-fg-1)',
            fontSize: 'var(--qf-t-base)', fontFamily: 'inherit',
          }}
        />
      </div>

      {open && hasResults && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 6px)', left: 0, right: 0,
          background: 'var(--qf-bg-raised)', border: '1px solid var(--qf-border-1)',
          borderRadius: 'var(--qf-r-lg)', boxShadow: 'var(--qf-sh-lg)',
          zIndex: 100, overflow: 'hidden',
        }}>
          {matchedHosts.length > 0 && (
            <>
              <div style={{ padding: '6px 12px 4px', fontSize: 'var(--qf-t-xs)', fontWeight: 600, color: 'var(--qf-fg-mute)', textTransform: 'uppercase', letterSpacing: '0.06em' }}>
                Hosts
              </div>
              {matchedHosts.map(h => (
                <button key={h.id} onMouseDown={() => go(`/hosts/${h.id}`)} style={searchItemStyle}>
                  <IconServer size={14} style={{ color: 'var(--qf-fg-mute)', flexShrink: 0 }} />
                  <span style={{ fontFamily: 'var(--qf-mono)', fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-1)' }}>{h.hostname}</span>
                </button>
              ))}
            </>
          )}
          {matchedPolicies.length > 0 && (
            <>
              <div style={{ padding: '6px 12px 4px', fontSize: 'var(--qf-t-xs)', fontWeight: 600, color: 'var(--qf-fg-mute)', textTransform: 'uppercase', letterSpacing: '0.06em', borderTop: matchedHosts.length > 0 ? '1px solid var(--qf-border-2)' : 'none' }}>
                Policies
              </div>
              {matchedPolicies.map(p => (
                <button key={p.id} onMouseDown={() => go(`/policies/${p.id}`)} style={searchItemStyle}>
                  <IconShield size={14} style={{ color: 'var(--qf-fg-mute)', flexShrink: 0 }} />
                  <span style={{ fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-1)' }}>{p.name}</span>
                </button>
              ))}
            </>
          )}
        </div>
      )}

      {open && q.length > 0 && !hasResults && (
        <div style={{
          position: 'absolute', top: 'calc(100% + 6px)', left: 0, right: 0,
          background: 'var(--qf-bg-raised)', border: '1px solid var(--qf-border-1)',
          borderRadius: 'var(--qf-r-lg)', boxShadow: 'var(--qf-sh-lg)',
          zIndex: 100, padding: '12px 14px',
          fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)',
        }}>
          No results for "{query}"
        </div>
      )}
    </div>
  )
}

const searchItemStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 9,
  width: '100%', textAlign: 'left',
  padding: '8px 12px', fontSize: 'var(--qf-t-base)',
  fontFamily: 'inherit', background: 'transparent',
  border: 'none', cursor: 'pointer', color: 'var(--qf-fg-2)',
}

const menuItemStyle: React.CSSProperties = {
  display: 'flex', alignItems: 'center', gap: 9,
  width: '100%', textAlign: 'left',
  padding: '8px 10px', fontSize: 'var(--qf-t-base)',
  fontFamily: 'inherit', color: 'var(--qf-fg-2)',
  background: 'transparent', border: 'none',
  borderRadius: 'var(--qf-r-sm)', cursor: 'pointer',
}

export default function Layout() {
  const [opened, { toggle }] = useDisclosure()
  const [profileOpened, { open: openProfile, close: closeProfile }] = useDisclosure(false)
  const { user } = useAuth()
  const navigate = useNavigate()
  const qc = useQueryClient()
  useWebSocket()

  const { data: versionData } = useQuery({
    queryKey: ['version'],
    queryFn: () => client.get<{ version: string }>('/version').then(r => r.data),
    staleTime: Infinity,
  })
  const { data: hosts = [] } = useQuery({
    queryKey: ['hosts'],
    queryFn: listHosts,
    refetchInterval: 30_000,
  })
  const offlineCount = hosts.filter(h => h.status !== 'active' && h.status !== 'enrolling').length

  const { setColorScheme } = useMantineColorScheme()
  const computed = useComputedColorScheme('light')

  async function handleLogout() {
    await logout()
    qc.clear()
    navigate('/login')
  }

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding={0}
    >
      <ProfileModal opened={profileOpened} onClose={closeProfile} />

      <AppShell.Header
        style={{
          background: 'var(--qf-bg-header)',
          borderBottom: '1px solid var(--qf-border-1)',
        }}
      >
        <div style={{
          height: '100%', display: 'flex', alignItems: 'center',
          gap: 14, padding: '0 16px',
        }}>
          <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />

          {/* Logo */}
          <div style={{ display: 'flex', alignItems: 'center', gap: 9, color: 'var(--qf-brand)' }}>
            <Mark size={22} />
            <span style={{ fontSize: 18, fontWeight: 700, letterSpacing: '-0.01em', color: 'var(--qf-fg-1)' }}>
              qf
            </span>
          </div>

          {/* Separator */}
          <span style={{ width: 1, height: 22, background: 'var(--qf-border-1)', flexShrink: 0 }} />

          {/* Tenant chip */}
          <div style={{
            display: 'inline-flex', alignItems: 'center', gap: 8,
            fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-3)',
            fontFamily: 'var(--qf-mono)',
          }}>
            <span style={{ width: 6, height: 6, borderRadius: 2, background: 'var(--qf-ok-solid)', flexShrink: 0 }} />
            Default
            <IconChevronDown size={14} style={{ color: 'var(--qf-fg-faint)' }} />
          </div>

          {/* Right side */}
          <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 10 }}>
            {/* Search */}
            <GlobalSearch />

            {/* Theme toggle */}
            <button
              onClick={() => setColorScheme(computed === 'dark' ? 'light' : 'dark')}
              title="Toggle color scheme"
              style={{
                width: 32, height: 32, display: 'grid', placeItems: 'center',
                background: 'transparent', border: '1px solid var(--qf-border-1)',
                borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-mute)',
                cursor: 'pointer',
              }}
            >
              {computed === 'dark' ? <IconSun size={16} /> : <IconMoon size={16} />}
            </button>

            {/* User menu */}
            {user && (
              <UserMenu
                email={user.email}
                onProfile={openProfile}
                onSignOut={handleLogout}
              />
            )}
          </div>
        </div>
      </AppShell.Header>

      <AppShell.Navbar
        style={{
          background: 'var(--qf-bg-nav)',
          borderRight: '1px solid var(--qf-border-1)',
          display: 'flex',
          flexDirection: 'column',
        }}
        p="xs"
      >
        <ScrollArea style={{ flex: 1 }}>
          {NAV.map(item => {
            const badge = item.to === '/hosts' && offlineCount > 0
              ? (
                <span style={{
                  fontSize: 'var(--qf-t-xs)', fontWeight: 700,
                  fontFamily: 'var(--qf-mono)',
                  background: 'var(--qf-bad-bg)', color: 'var(--qf-bad-fg)',
                  padding: '1px 6px', borderRadius: 'var(--qf-r-full)',
                }}>
                  {offlineCount}
                </span>
              )
              : null

            return (
              <RouterNavLink
                key={item.to}
                to={item.to}
                end={item.to === '/dashboard'}
                className="qf-nav-link"
              >
                <span className="qf-nav-icon">
                  <item.icon size={18} />
                </span>
                <span style={{ flex: 1 }}>{item.label}</span>
                {badge}
              </RouterNavLink>
            )
          })}
        </ScrollArea>

        {/* Version footer */}
        <div style={{
          padding: '10px 12px',
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          color: 'var(--qf-fg-faint)',
          fontSize: 'var(--qf-t-xs)',
          fontFamily: 'var(--qf-mono)',
          borderTop: '1px solid var(--qf-border-2)',
        }}>
          <span>control-plane</span>
          {versionData?.version && <span>v{versionData.version}</span>}
        </div>
      </AppShell.Navbar>

      <AppShell.Main style={{ background: 'var(--qf-bg-body)', minHeight: '100vh' }}>
        <div style={{ padding: 24 }}>
          <Outlet />
        </div>
      </AppShell.Main>
    </AppShell>
  )
}
