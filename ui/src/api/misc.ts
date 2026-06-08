import client from './client'
import type { DefaultPolicy, AuditLog, Token, APIToken, User } from '../types'

// Default policy
export async function getDefaultPolicy(): Promise<DefaultPolicy> {
  const { data } = await client.get<DefaultPolicy>('/default-policy')
  return data
}

export async function updateDefaultPolicy(body: { default_ingress_action: string; default_egress_action: string }): Promise<DefaultPolicy> {
  const { data } = await client.put<DefaultPolicy>('/default-policy', body)
  return data
}

// Audit log
export async function listAuditLog(params?: { limit?: number; actor_id?: string; object_type?: string; since?: string }): Promise<AuditLog[]> {
  const { data } = await client.get<AuditLog[]>('/audit-log', { params })
  return data
}

// Bootstrap tokens
export async function listTokens(): Promise<Token[]> {
  const { data } = await client.get<Token[]>('/tokens')
  return data
}

export async function createToken(body: { type: string; target_host_id?: string; label_template?: Record<string, string>; ttl_seconds: number; max_uses: number }): Promise<Token> {
  const { data } = await client.post<Token>('/tokens', body)
  return data
}

export async function revokeToken(id: string): Promise<void> {
  await client.delete(`/tokens/${id}`)
}

// API tokens
export async function listAPITokens(): Promise<APIToken[]> {
  const { data } = await client.get<APIToken[]>('/api-tokens')
  return data
}

export async function createAPIToken(body: { name: string; role: string; expires_at?: string }): Promise<APIToken> {
  const { data } = await client.post<APIToken>('/api-tokens', body)
  return data
}

export async function deleteAPIToken(id: string): Promise<void> {
  await client.delete(`/api-tokens/${id}`)
}

// Users
export async function listUsers(): Promise<User[]> {
  const { data } = await client.get<User[]>('/users')
  return data
}

export async function createUser(body: { username: string; email: string; password: string; role: string }): Promise<User> {
  const { data } = await client.post<User>('/users', body)
  return data
}

export async function patchUser(id: string, body: { password?: string; status?: string }): Promise<User> {
  const { data } = await client.patch<User>(`/users/${id}`, body)
  return data
}

export async function updateUserRole(id: string, role: string): Promise<void> {
  await client.put(`/users/${id}/roles`, { role })
}

export async function deleteUser(id: string): Promise<void> {
  await client.delete(`/users/${id}`)
}
