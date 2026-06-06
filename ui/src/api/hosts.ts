import client from './client'
import type { Host, LogEvent, FlowEvent, Counter, EffectiveRuleset } from '../types'

export async function listHosts(): Promise<Host[]> {
  const { data } = await client.get<Host[]>('/hosts')
  return data
}

export async function getHost(id: string): Promise<Host> {
  const { data } = await client.get<Host>(`/hosts/${id}`)
  return data
}

export async function patchHost(id: string, patch: { labels?: Record<string, string>; status?: string; flow_events_enabled?: boolean }): Promise<Host> {
  const { data } = await client.patch<Host>(`/hosts/${id}`, patch)
  return data
}

export async function listEvents(hostId: string, params?: { limit?: number; since?: string; action?: string }): Promise<LogEvent[]> {
  const { data } = await client.get<LogEvent[]>(`/hosts/${hostId}/events`, { params })
  return data
}

export async function listFlows(hostId: string, params?: { limit?: number; since?: string }): Promise<FlowEvent[]> {
  const { data } = await client.get<FlowEvent[]>(`/hosts/${hostId}/flows`, { params })
  return data
}

export async function latestCounters(hostId: string): Promise<Counter[]> {
  const { data } = await client.get<Counter[]>(`/hosts/${hostId}/counters/latest`)
  return data
}

export async function deleteHost(id: string): Promise<void> {
  await client.delete(`/hosts/${id}`)
}

export async function getHostRuleset(id: string): Promise<EffectiveRuleset> {
  const { data } = await client.get<EffectiveRuleset>(`/hosts/${id}/ruleset`)
  return data
}
