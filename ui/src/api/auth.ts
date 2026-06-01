import client from './client'
import type { Me } from '../types'

export async function login(email: string, password: string): Promise<Me> {
  const { data } = await client.post<Me>('/auth/login', { email, password })
  return data
}

export async function logout(): Promise<void> {
  await client.post('/auth/logout')
}

export async function getMe(): Promise<Me> {
  const { data } = await client.get<Me>('/auth/me')
  return data
}

export async function oidcEnabled(): Promise<boolean> {
  const { data } = await client.get<{ enabled: boolean }>('/auth/oidc/enabled')
  return data.enabled
}
