import client from './client'
import type { Policy, Rule, PolicyVersion, PreviewResult } from '../types'

interface PolicyDetail extends Policy {
  rules: Rule[]
}

export async function listPolicies(): Promise<Policy[]> {
  const { data } = await client.get<Policy[]>('/policies')
  return data
}

export async function getPolicy(id: string): Promise<PolicyDetail> {
  const { data } = await client.get<PolicyDetail>(`/policies/${id}`)
  return data
}

export async function createPolicy(body: {
  name: string
  description: string
  priority: number
  selector: unknown
}): Promise<Policy> {
  const { data } = await client.post<Policy>('/policies', body)
  return data
}

export async function updatePolicy(id: string, body: {
  name: string
  description: string
  priority: number
  selector: unknown
  rules: Omit<Rule, 'id' | 'policy_id' | 'created_at' | 'updated_at'>[]
}): Promise<PolicyDetail> {
  const { data } = await client.put<PolicyDetail>(`/policies/${id}`, body)
  return data
}

export async function deletePolicy(id: string): Promise<void> {
  await client.delete(`/policies/${id}`)
}

export async function previewPolicy(id: string, rules: unknown[]): Promise<PreviewResult> {
  const { data } = await client.post<PreviewResult>(`/policies/${id}/preview`, { rules })
  return data
}

export async function listVersions(id: string): Promise<PolicyVersion[]> {
  const { data } = await client.get<PolicyVersion[]>(`/policies/${id}/versions`)
  return data
}

export async function revertVersion(id: string, version: number): Promise<PolicyDetail> {
  const { data } = await client.post<PolicyDetail>(`/policies/${id}/versions/${version}/revert`)
  return data
}
