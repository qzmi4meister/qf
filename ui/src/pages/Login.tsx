import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useQuery } from '@tanstack/react-query'
import { IconShieldLock } from '@tabler/icons-react'
import { login, oidcEnabled } from '../api/auth'
import Mark from '../components/Mark'

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '10px 12px',
  background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
  borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-1)',
  fontSize: 'var(--qf-t-base)', fontFamily: 'inherit', outline: 'none',
  boxSizing: 'border-box',
}

const labelStyle: React.CSSProperties = {
  display: 'flex', flexDirection: 'column',
  fontSize: 11, fontWeight: 600, color: 'var(--qf-fg-mute)',
  textTransform: 'uppercase', letterSpacing: '0.06em',
}

export default function Login() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const { data: oidc } = useQuery({
    queryKey: ['oidc-enabled'],
    queryFn: oidcEnabled,
    retry: false,
  })

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await login(username, password)
      qc.invalidateQueries({ queryKey: ['me'] })
      navigate('/dashboard')
    } catch {
      setError('Invalid credentials or expired session.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{
      position: 'fixed', inset: 0,
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      background: 'var(--qf-bg-body)', color: 'var(--qf-fg-1)',
      fontFamily: 'var(--qf-font)', padding: 24,
    }}>
      <div style={{ width: '100%', maxWidth: 360 }}>

        {/* Logo */}
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 9, color: 'var(--qf-brand)' }}>
            <Mark size={34} />
            <span style={{ fontSize: 28, fontWeight: 700, letterSpacing: '-0.02em', color: 'var(--qf-fg-1)' }}>qf</span>
          </div>
          <div style={{ marginTop: 6, fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-faint)', fontFamily: 'var(--qf-mono)' }}>
            control-plane
          </div>
        </div>

        {/* Card */}
        <div style={{
          background: 'var(--qf-bg-surface)',
          border: '1px solid var(--qf-border-1)',
          borderRadius: 16,
          padding: 28,
          boxShadow: '0 8px 32px rgba(0,0,0,0.18)',
        }}>
          <h1 style={{ margin: '0 0 20px', fontSize: 'var(--qf-t-lg)', fontWeight: 600, color: 'var(--qf-fg-1)', letterSpacing: '-0.01em' }}>
            Sign in
          </h1>

          {error && (
            <div style={{
              padding: '9px 12px', marginBottom: 16,
              background: 'var(--qf-bad-bg)', border: '1px solid var(--qf-bad-fg)',
              borderRadius: 'var(--qf-r-md)', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)',
            }}>
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <label style={labelStyle}>
              Username
              <input
                type="text"
                value={username}
                onChange={e => setUsername(e.currentTarget.value)}
                required
                autoFocus
                autoCapitalize="none"
                autoCorrect="off"
                autoComplete="username"
                style={{ ...inputStyle, marginTop: 6, ...(error ? { borderColor: 'var(--qf-bad-fg)' } : {}) }}
              />
            </label>
            <label style={labelStyle}>
              Password
              <input
                type="password"
                value={password}
                onChange={e => setPassword(e.currentTarget.value)}
                required
                style={{ ...inputStyle, marginTop: 6 }}
              />
            </label>
            <button
              type="submit"
              disabled={loading}
              style={{
                width: '100%', padding: '10px', marginTop: 4,
                background: 'var(--qf-brand-solid)', color: '#fff',
                border: 'none', borderRadius: 'var(--qf-r-md)',
                fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit',
                cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.7 : 1,
                transition: 'opacity 0.15s',
              }}
            >
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
          </form>

          {oidc && (
            <>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '20px 0', color: 'var(--qf-fg-faint)', fontSize: 'var(--qf-t-sm)' }}>
                <span style={{ flex: 1, height: 1, background: 'var(--qf-border-1)' }} />
                or
                <span style={{ flex: 1, height: 1, background: 'var(--qf-border-1)' }} />
              </div>
              <a
                href="/auth/oidc/login"
                style={{
                  display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 9,
                  width: '100%', padding: '10px',
                  border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-md)',
                  background: 'transparent', color: 'var(--qf-fg-2)',
                  fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit',
                  textDecoration: 'none', cursor: 'pointer', boxSizing: 'border-box',
                }}
              >
                <IconShieldLock size={16} /> Continue with Okta SSO
              </a>
            </>
          )}
        </div>

        <p style={{ margin: '18px 0 0', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-faint)', textAlign: 'center' }}>
          Access is audited.
        </p>
      </div>
    </div>
  )
}
