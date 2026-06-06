import client from './client'
import type { ObjectGroup } from '../types'

export async function listObjectGroups(): Promise<ObjectGroup[]> {
  const { data } = await client.get<ObjectGroup[]>('/objectgroups')
  return data
}

export async function getObjectGroup(id: string): Promise<ObjectGroup> {
  const { data } = await client.get<ObjectGroup>(`/objectgroups/${id}`)
  return data
}

export async function createObjectGroup(body: { type: string; name: string; spec: unknown }): Promise<ObjectGroup> {
  const { data } = await client.post<ObjectGroup>('/objectgroups', body)
  return data
}

export async function updateObjectGroup(id: string, spec: unknown): Promise<ObjectGroup> {
  const { data } = await client.put<ObjectGroup>(`/objectgroups/${id}`, { spec })
  return data
}

export async function deleteObjectGroup(id: string): Promise<void> {
  await client.delete(`/objectgroups/${id}`)
}
