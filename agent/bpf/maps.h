#pragma once

/* vmlinux.h + bpf_helpers.h must be included before this header. */
#include "common.h"

/* Sorted effective ruleset. Populated by userspace from PolicyBundle.
 * Key: rule index (u32 position in sorted order), Value: struct rule_entry. */
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, MAX_RULES);
	__type(key, __u32);
	__type(value, struct rule_entry);
} qf_rules SEC(".maps");

/* Number of active rules at index [0]. Avoids scanning beyond real count. */
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u32);
} qf_rule_count SEC(".maps");

/* BPF conntrack table. LRU evicts least-recently-used on overflow. */
struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, CONNTRACK_MAX);
	__type(key, struct ct_key);
	__type(value, struct ct_entry);
} qf_conntrack SEC(".maps");

/* Per-rule packet/byte counters. Per-CPU for lock-free increments;
 * userspace aggregates across CPUs on readout. */
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, MAX_RULES);
	__type(key, __u32);
	__type(value, struct rule_counter);
} qf_rule_counters SEC(".maps");

/* Log event ring buffer shared with userspace. 4 MiB. */
struct {
	__uint(type, BPF_MAP_TYPE_RINGBUF);
	__uint(max_entries, 1 << 22);
} qf_events SEC(".maps");

/* Large IP sets via LPM trie. Each set is identified by a uint32 id.
 * Key: ipset_lpm_key (ipset_id exact + addr prefix); value: u8 presence flag.
 * Max 65536 entries total across all sets. Requires BPF_F_NO_PREALLOC. */
#define IPSET_MAX_ENTRIES 65536
struct {
	__uint(type, BPF_MAP_TYPE_LPM_TRIE);
	__uint(max_entries, IPSET_MAX_ENTRIES);
	__uint(map_flags, BPF_F_NO_PREALLOC);
	__type(key, struct ipset_lpm_key);
	__type(value, __u8);
} qf_ipsets SEC(".maps");

/* Per-rule token buckets for log-rate limiting. Per-CPU: no atomic ops needed.
 * Key: rule index; value: struct token_bucket.
 * Initialized lazily on first packet per CPU (last_ns==0 → full bucket). */
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, MAX_RULES);
	__type(key, __u32);
	__type(value, struct token_bucket);
} qf_rate_limits SEC(".maps");

/* Suppressed log-event count since last userspace readout. Single slot [0].
 * Per-CPU; userspace aggregates and resets across CPUs when draining events. */
struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(max_entries, 1);
	__type(key, __u32);
	__type(value, __u64);
} qf_suppressed_count SEC(".maps");

/* Global config. Slots:
 *   [0] flags: bit0=conntrack_enabled, bit1=flow_events_enabled
 *   [1] default ingress action (ACTION_*)
 *   [2] default egress action  (ACTION_*) */
struct {
	__uint(type, BPF_MAP_TYPE_ARRAY);
	__uint(max_entries, 3);
	__type(key, __u32);
	__type(value, __u32);
} qf_config SEC(".maps");
