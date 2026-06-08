import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useQuery } from '@tanstack/react-query'
import { IconShieldLock } from '@tabler/icons-react'
import { login, oidcEnabled } from '../api/auth'
import Mark from '../components/Mark'

function KernelGrid() {
  return (
    <svg style={{ position: 'absolute', inset: 0, width: '100%', height: '100%', opacity: 0.5 }}>
      <defs>
        <pattern id="qf-login-grid" width="34" height="34" patternUnits="userSpaceOnUse">
          <path d="M34 0H0V34" fill="none" stroke="var(--qf-border-2)" strokeWidth="1" />
        </pattern>
      </defs>
      <rect width="100%" height="100%" fill="url(#qf-login-grid)" />
    </svg>
  )
}

function LoginAside() {
  return (
    <div style={{
      flex: '0 0 46%', background: 'var(--qf-bg-surface)',
      borderRight: '1px solid var(--qf-border-1)',
      display: 'flex', flexDirection: 'column', justifyContent: 'space-between',
      padding: 48, position: 'relative', overflow: 'hidden',
    }}>
      <KernelGrid />

      <div style={{ position: 'relative', display: 'flex', alignItems: 'center', gap: 9, color: 'var(--qf-brand)' }}>
        <Mark size={30} />
        <span style={{ fontSize: 26, fontWeight: 700, letterSpacing: '-0.01em', color: 'var(--qf-fg-1)' }}>qf</span>
      </div>

      <div style={{ position: 'relative' }}>
        <h2 style={{ margin: 0, fontSize: 30, fontWeight: 700, lineHeight: 1.18, color: 'var(--qf-fg-1)', letterSpacing: '-0.02em', maxWidth: 420 }}>
          The control plane for your<br />eBPF host firewall.
        </h2>
        <p style={{ margin: '16px 0 0', fontSize: 'var(--qf-t-lg)', color: 'var(--qf-fg-mute)', maxWidth: 400, lineHeight: 1.55 }}>
          Author policy once, push to thousands of hosts, and watch every packet verdict converge in real time.
        </p>
        <div style={{ display: 'flex', gap: 24, marginTop: 32 }}>
          {([['1,284', 'hosts enforcing'], ['412', 'rules live'], ['p95 1.2s', 'push latency']] as const).map(([n, l]) => (
            <div key={l}>
              <div style={{ fontSize: 'var(--qf-t-2xl)', fontWeight: 700, fontFamily: 'var(--qf-mono)', color: 'var(--qf-fg-1)', letterSpacing: '-0.02em' }}>{n}</div>
              <div style={{ fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-mute)', marginTop: 2 }}>{l}</div>
            </div>
          ))}
        </div>
      </div>

      <div style={{ position: 'relative', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-faint)', fontFamily: 'var(--qf-mono)' }}>
        qf control-plane · v0.8.15
      </div>
    </div>
  )
}

const inputStyle: React.CSSProperties = {
  width: '100%', padding: '10px 12px',
  background: 'var(--qf-bg-input)', border: '1px solid var(--qf-border-input)',
  borderRadius: 'var(--qf-r-md)', color: 'var(--qf-fg-1)',
  fontSize: 'var(--qf-t-md)', fontFamily: 'inherit', outline: 'none',
  boxSizing: 'border-box',
}

const labelStyle: React.CSSProperties = {
  display: 'flex', flexDirection: 'column',
  fontSize: 'var(--qf-t-sm)', fontWeight: 600, color: 'var(--qf-fg-2)',
}

export default function Login() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [email, setEmail] = useState('')
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
      await login(email, password)
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
      position: 'fixed', inset: 0, display: 'flex',
      background: 'var(--qf-bg-body)', color: 'var(--qf-fg-1)',
      fontFamily: 'var(--qf-font)',
    }}>
      {/* Left aside — hidden on narrow screens */}
      <div className="qf-login-aside">
        <LoginAside />
      </div>

      {/* Right pane — form */}
      <div style={{ flex: 1, display: 'grid', placeItems: 'center', padding: 24, overflowY: 'auto' }}>
        <div style={{ width: 360 }}>
          <h1 style={{ margin: 0, fontSize: 'var(--qf-t-2xl)', fontWeight: 700, color: 'var(--qf-fg-1)', letterSpacing: '-0.01em' }}>
            Sign in
          </h1>
          <p style={{ margin: '6px 0 26px', fontSize: 'var(--qf-t-base)', color: 'var(--qf-fg-mute)' }}>
            Use your organization SSO or operator credentials.
          </p>

          {oidc && (
            <>
              <a
                href="/auth/oidc/login"
                style={{
                  display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 9,
                  width: '100%', padding: '11px', marginBottom: 10,
                  border: '1px solid var(--qf-border-1)', borderRadius: 'var(--qf-r-md)',
                  background: 'transparent', color: 'var(--qf-fg-2)',
                  fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit',
                  textDecoration: 'none', cursor: 'pointer', boxSizing: 'border-box',
                }}
              >
                <IconShieldLock size={17} /> Continue with Okta SSO
              </a>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12, margin: '18px 0', color: 'var(--qf-fg-faint)', fontSize: 'var(--qf-t-sm)' }}>
                <span style={{ flex: 1, height: 1, background: 'var(--qf-border-1)' }} />
                or
                <span style={{ flex: 1, height: 1, background: 'var(--qf-border-1)' }} />
              </div>
            </>
          )}

          {error && (
            <div style={{
              display: 'flex', alignItems: 'center', gap: 9,
              padding: '9px 12px', marginBottom: 14,
              background: 'var(--qf-bad-bg)', border: '1px solid var(--qf-bad-fg)',
              borderRadius: 'var(--qf-r-md)', color: 'var(--qf-bad-fg)', fontSize: 'var(--qf-t-sm)',
            }}>
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
            <label style={labelStyle}>
              Email
              <input
                type="email"
                value={email}
                onChange={e => setEmail(e.currentTarget.value)}
                required
                autoFocus
                style={{ ...inputStyle, marginTop: 6, ...(error ? { borderColor: 'var(--qf-bad-fg)' } : {}) }}
              />
            </label>
            <label style={labelStyle}>
              <span style={{ display: 'flex', justifyContent: 'space-between' }}>
                Password
                <a href="#" style={{ color: 'var(--qf-brand)', textDecoration: 'none', fontWeight: 500 }}>Forgot?</a>
              </span>
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
                width: '100%', padding: '11px', marginTop: 4,
                background: 'var(--qf-brand-solid)', color: '#fff',
                border: 'none', borderRadius: 'var(--qf-r-md)',
                fontSize: 'var(--qf-t-base)', fontWeight: 600, fontFamily: 'inherit',
                cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.7 : 1,
              }}
            >
              {loading ? 'Signing in…' : 'Sign in'}
            </button>
          </form>

          <p style={{ margin: '22px 0 0', fontSize: 'var(--qf-t-sm)', color: 'var(--qf-fg-faint)', textAlign: 'center', lineHeight: 1.5 }}>
            Access is audited.
          </p>
        </div>
      </div>
    </div>
  )
}
