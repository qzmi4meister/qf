#pragma once

/* vmlinux.h + bpf_helpers.h + maps.h must be included before this. */
#include "common.h"

/* ── CIDR matching ──────────────────────────────────────────────────── */

/* Returns 1 if ip matches any CIDR in the list.
 * n == 0 means "any" (no restriction, matches IPv6 too). */
static __always_inline int
match_cidrs4(__be32 ip, struct cidr4 *cidrs, __u8 n, __u8 is_ipv6)
{
	if (n == 0)
		return 1;
	if (is_ipv6)
		return 0; /* IPv4 CIDRs cannot match IPv6 addresses */

	/* max 8 inline CIDRs; loop unrolled by verifier. */
	for (int i = 0; i < 8; i++) {
		if (i >= n)
			break;
		if ((ip & cidrs[i].mask) == (cidrs[i].addr & cidrs[i].mask))
			return 1;
	}
	return 0;
}

/* ── Port range matching ────────────────────────────────────────────── */

/* Returns 1 if port falls within any range.
 * n == 0 means "any port". */
static __always_inline int
match_port_ranges(__u16 port, struct port_range *ranges, __u8 n)
{
	if (n == 0)
		return 1;

	for (int i = 0; i < 8; i++) {
		if (i >= n)
			break;
		if (port >= ranges[i].start && port <= ranges[i].end)
			return 1;
	}
	return 0;
}

/* ── Single-rule match ──────────────────────────────────────────────── */

/* Returns 1 if all match criteria in m are satisfied by ctx.
 * __noinline: verified once as a BPF subprogram; keeps eval_rules loop simple. */
static __noinline int
match_rule(struct pkt_ctx *ctx, struct rule_match *m)
{
	/* Direction must match exactly. */
	if (m->direction != ctx->direction)
		return 0;

	/* Protocol: PROTO_ANY matches everything. */
	if (m->protocol != PROTO_ANY && m->protocol != ctx->proto)
		return 0;

	/* Source CIDR — inline up to 8. */
	if (!match_cidrs4(ctx->src_ip4, m->src_cidrs, m->n_src_cidrs, ctx->is_ipv6))
		return 0;

	/* Destination CIDR — inline up to 8. */
	if (!match_cidrs4(ctx->dst_ip4, m->dst_cidrs, m->n_dst_cidrs, ctx->is_ipv6))
		return 0;

	/* Large IPSet via LPM trie. Exact match on ipset_id, prefix match on addr. */
	if (m->src_ipset_id != 0) {
		if (ctx->is_ipv6)
			return 0;
		struct ipset_lpm_key k = {
			.prefixlen = 64,
			.ipset_id  = m->src_ipset_id,
			.addr      = ctx->src_ip4,
		};
		if (!bpf_map_lookup_elem(&qf_ipsets, &k))
			return 0;
	}
	if (m->dst_ipset_id != 0) {
		if (ctx->is_ipv6)
			return 0;
		struct ipset_lpm_key k = {
			.prefixlen = 64,
			.ipset_id  = m->dst_ipset_id,
			.addr      = ctx->dst_ip4,
		};
		if (!bpf_map_lookup_elem(&qf_ipsets, &k))
			return 0;
	}

	/* Source port ranges. */
	if (!match_port_ranges(ctx->src_port, m->src_ports, m->n_src_ports))
		return 0;

	/* Destination port ranges. */
	if (!match_port_ranges(ctx->dst_port, m->dst_ports, m->n_dst_ports))
		return 0;

	/* Conntrack state predicate.
	 * CT_NONE = stateless rule — always matches regardless of CT state.
	 * All other values require a tracked flow with a matching state. */
	if (m->state != CT_NONE) {
		if (ctx->ct_state == 0)
			return 0;           /* untracked — stateful rules never match */
		if (m->state == CT_RELATED)
			return 0;           /* CT_RELATED unimplemented (P1-BPF-07) */
		if (m->state != ctx->ct_state)
			return 0;
	}

	return 1;
}

/* ── Rule evaluation loop ───────────────────────────────────────────── */

/* eval_rules: scan qf_rules in priority order; first match wins.
 *
 * Returns the matched rule's ACTION_* constant, or 0 if no rule matched.
 * On match, *matched_idx is set to the rule's array index so the caller
 * can update per-rule counters and emit a log event.
 *
 * Two implementations selected at compile time:
 *   USE_BPF_LOOP=1  — bpf_loop() callback (kernel ≥5.17), up to 64 rules.
 *   default         — bounded for-loop (kernel ≥5.15),     up to 32 rules. */

#ifdef USE_BPF_LOOP

/* bpf_loop helper was added in libbpf 0.8.0 (kernel 5.17).
 * Declare it manually so we compile correctly against older libbpf (e.g. 0.5.0 on Ubuntu 22.04). */
#ifndef bpf_loop
static long (*bpf_loop)(__u32 nr_loops, long (*callback_fn)(unsigned int idx, void *ctx),
                        void *callback_ctx, __u64 flags) = (void *)181;
#endif

struct eval_ctx {
	struct pkt_ctx *pkt;
	__u32 matched_idx;
	__u8  action;
};

static long eval_one(__u32 i, void *data)
{
	struct eval_ctx *ectx = data;
	struct rule_entry *rule = bpf_map_lookup_elem(&qf_rules, &i);
	if (!rule)
		return 1; /* stop — sparse map, no more rules */
	if (!match_rule(ectx->pkt, &rule->match))
		return 0; /* continue */
	ectx->matched_idx = i;
	ectx->action      = rule->action;
	return 1; /* stop — first match wins */
}

static __always_inline __u8
eval_rules(struct pkt_ctx *ctx, __u32 *matched_idx)
{
	__u32 zero     = 0;
	__u32 *cnt_ptr = bpf_map_lookup_elem(&qf_rule_count, &zero);
	__u32 n        = cnt_ptr ? *cnt_ptr : 0;
	if (n > MAX_RULES)
		n = MAX_RULES;

	struct eval_ctx ectx = { .pkt = ctx };
	bpf_loop(n, eval_one, &ectx, 0);
	if (ectx.action != 0) {
		*matched_idx = ectx.matched_idx;
		return ectx.action;
	}
	return 0;
}

#else /* unrolled loop — kernel 5.15/5.16, EVAL_MAX_RULES=32 */

/* match_rule_compat: always-inline wrapper for the compat path.
 * match_rule is __noinline (BPF subprogram); this thin wrapper allows
 * the verifier to see the call without nested loop complexity. */
static __always_inline int
match_rule_compat(struct pkt_ctx *ctx, struct rule_match *m)
{
	return match_rule(ctx, m);
}

static __always_inline __u8
eval_rules(struct pkt_ctx *ctx, __u32 *matched_idx)
{
	__u32 zero     = 0;
	__u32 *cnt_ptr = bpf_map_lookup_elem(&qf_rule_count, &zero);
	__u32 n        = cnt_ptr ? *cnt_ptr : 0;
	if (n > EVAL_MAX_RULES)
		n = EVAL_MAX_RULES;

	/* Use a separate key variable so the counter i is never address-taken.
	 * This keeps i in a callee-saved register across the match_rule call,
	 * letting the 5.15 BPF verifier track the bounded increment directly. */
	for (__u32 i = 0; i < EVAL_MAX_RULES; i++) {
		if (i >= n)
			break;
		__u32 key = i;
		struct rule_entry *rule = bpf_map_lookup_elem(&qf_rules, &key);
		if (!rule)
			break; /* stop — sparse map, no more rules (matches full/bpf_loop behaviour) */
		if (!match_rule_compat(ctx, &rule->match))
			continue;
		*matched_idx = i;
		return rule->action;
	}
	return 0;
}

#endif /* USE_BPF_LOOP */

/* ── Default action helper ──────────────────────────────────────────── */

/* Returns the configured default action for the packet's direction,
 * falling back to ACTION_ALLOW if the config map is unreadable. */
static __always_inline __u8
default_action(struct pkt_ctx *ctx)
{
	__u32 slot      = (ctx->direction == DIR_INGRESS) ? 1 : 2;
	__u32 *act_ptr  = bpf_map_lookup_elem(&qf_config, &slot);
	if (act_ptr && *act_ptr != 0)
		return (__u8)*act_ptr;
	return ACTION_ALLOW;
}
