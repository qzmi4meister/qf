import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import {
  Center,
  Paper,
  Title,
  TextInput,
  PasswordInput,
  Button,
  Stack,
  Divider,
  Alert,
} from '@mantine/core'
import { IconAlertCircle, IconShieldLock } from '@tabler/icons-react'
import { login, oidcEnabled } from '../api/auth'
import { useQuery } from '@tanstack/react-query'

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
      await qc.invalidateQueries({ queryKey: ['me'] })
      navigate('/app/dashboard')
    } catch {
      setError('Invalid email or password')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Center h="100vh" bg="gray.0">
      <Paper w={360} p="xl" withBorder shadow="md">
        <Stack gap="md">
          <Stack gap={4} align="center">
            <IconShieldLock size={40} color="var(--mantine-color-blue-6)" />
            <Title order={2}>qf control plane</Title>
          </Stack>

          {error && (
            <Alert icon={<IconAlertCircle size={16} />} color="red">
              {error}
            </Alert>
          )}

          <form onSubmit={handleSubmit}>
            <Stack gap="sm">
              <TextInput
                label="Email"
                type="email"
                value={email}
                onChange={(e) => setEmail(e.currentTarget.value)}
                required
                autoFocus
              />
              <PasswordInput
                label="Password"
                value={password}
                onChange={(e) => setPassword(e.currentTarget.value)}
                required
              />
              <Button type="submit" loading={loading} fullWidth>
                Sign in
              </Button>
            </Stack>
          </form>

          {oidc && (
            <>
              <Divider label="or" labelPosition="center" />
              <Button
                variant="outline"
                fullWidth
                component="a"
                href="/auth/oidc/login"
              >
                Sign in with SSO
              </Button>
            </>
          )}
        </Stack>
      </Paper>
    </Center>
  )
}
