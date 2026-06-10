export interface Host {
  id: string
  tenant_id: string
  hostname: string
  labels: Record<string, string>
  status: string
  current_generation: number
  desired_generation: number
  last_heartbeat_at?: string
  agent_version?: string
  kernel_version?: string
  flow_events_enabled: boolean
  created_at: string
  updated_at: string
}

export interface Policy {
  id: string
  tenant_id: string
  name: string
  description: string
  priority: number
  selector: unknown
  current_version: number
  created_at: string
  updated_at: string
}

export interface Rule {
  id: string
  policy_id: string
  name: string
  priority: number
  direction: string
  match: unknown
  state?: string
  action: string
  log: boolean
  silent: boolean
  created_at: string
  updated_at: string
}

export interface PolicyDetail extends Policy {
  rules: Rule[]
}

export interface ObjectGroup {
  id: string
  tenant_id: string
  type: string
  name: string
  spec: unknown
  resolved_at?: string
  created_at: string
  updated_at: string
}

export interface LogEvent {
  id: string
  host_id: string
  rule_id?: string
  policy_id?: string
  direction: string
  action: string
  protocol: number
  src_ip?: string
  src_port?: number
  dst_ip?: string
  dst_port?: number
  packet_size?: number
  tcp_flags?: number
  ct_state?: string
  created_at: string
}

export interface FlowEvent {
  id: string
  host_id: string
  protocol: number
  src_ip?: string
  src_port?: number
  dst_ip?: string
  dst_port?: number
  bytes_orig: number
  bytes_reply: number
  packets_orig: number
  packets_reply: number
  final_state?: string
  started_at?: string
  ended_at?: string
  created_at: string
}

export interface Counter {
  id: string
  host_id: string
  rule_id: string
  policy_id?: string
  packets: number
  bytes: number
  ts: string
}

export interface DefaultPolicy {
  id: string
  tenant_id: string
  default_ingress_action: string
  default_egress_action: string
  updated_at: string
}

export interface AuditLog {
  id: string
  tenant_id: string
  actor_type: string
  actor_id?: string
  actor_username?: string
  action: string
  object_type: string
  object_id?: string
  before: unknown
  after: unknown
  created_at: string
}

export interface Token {
  id: string
  tenant_id: string
  type: string
  target_host_id?: string
  label_template?: Record<string, string>
  max_uses: number
  uses_count: number
  expires_at: string
  token?: string
}

export interface User {
  id: string
  username: string
  email: string
  status: string
  role?: string
  is_oidc: boolean
  last_login_at?: string
  created_at: string
}

export interface APIToken {
  id: string
  tenant_id: string
  name: string
  role: string
  expires_at?: string
  last_used_at?: string
  created_at: string
  token?: string
}

export interface Me {
  id: string
  username: string
  email: string
  role: string
  tenant_id: string
}

export interface PreviewResult {
  affected_count: number
  hosts: HostDiff[]
}

export interface HostDiff {
  id: string
  hostname: string
  added: string[]
  removed: string[]
  changed: string[]
}

export interface PolicyVersion {
  id: string
  version: number
  content: unknown
  created_by: string
  created_at: string
}

export interface RulesetRuleItem {
  rule_id: string
  rule_name: string
  policy_id: string
  policy_name: string
  priority: number
  direction: string
  action: string
  protocol?: string
  src_cidrs?: string[]
  dst_cidrs?: string[]
  src_ports?: string[]
  dst_ports?: string[]
}

export interface EffectiveRuleset {
  host_id: string
  default_ingress: string
  default_egress: string
  rules: RulesetRuleItem[]
}
