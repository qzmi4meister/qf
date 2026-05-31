// SPDX-License-Identifier: GPL-2.0
//
// qf TC ingress/egress filter — Phase 1 datapath.

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

#include "maps.h"
#include "parser.h"
#include "conntrack.h"
#include "matcher.h"

/* Emit a ring-buffer log event for the matched packet. */
static __always_inline void
emit_log(struct pkt_ctx *ctx, __u64 rule_id_hi, __u64 rule_id_lo,
         __u8 action, __u8 ct_state)
{
	struct log_event *ev = bpf_ringbuf_reserve(&qf_events, sizeof(*ev), 0);
	if (!ev)
		return;

	ev->ts_ns      = bpf_ktime_get_ns();
	ev->rule_id_hi = rule_id_hi;
	ev->rule_id_lo = rule_id_lo;
	ev->src_ip     = ctx->src_ip4;
	ev->dst_ip     = ctx->dst_ip4;
	ev->src_port   = ctx->src_port;
	ev->dst_port   = ctx->dst_port;
	ev->pkt_size   = ctx->pkt_size;
	ev->tcp_flags  = ctx->tcp_flags;
	ev->proto      = ctx->proto;
	ev->direction  = ctx->direction;
	ev->action     = action;
	ev->ct_state   = ct_state;
	bpf_ringbuf_submit(ev, 0);
}

/* Increment per-rule packet/byte counters (per-CPU, lock-free). */
static __always_inline void
bump_counter(__u32 idx, __u16 pkt_size)
{
	struct rule_counter *cnt = bpf_map_lookup_elem(&qf_rule_counters, &idx);
	if (cnt) {
		cnt->packets++;
		cnt->bytes += pkt_size;
	}
}

/* Apply action: update counters, maybe log, return TC verdict. */
static __always_inline int
apply_action(struct pkt_ctx *ctx, __u8 action,
             __u32 matched_idx, __u64 rule_id_hi, __u64 rule_id_lo,
             __u8 log_enabled, __u8 ct_state)
{
	bump_counter(matched_idx, ctx->pkt_size);

	if (log_enabled || action == ACTION_LOG || action == ACTION_DENY)
		emit_log(ctx, rule_id_hi, rule_id_lo, action, ct_state);

	return (action == ACTION_DENY) ? TC_ACT_SHOT : TC_ACT_OK;
}

/* Common datapath for ingress and egress. */
static __always_inline int
run_datapath(struct __sk_buff *skb, __u8 direction)
{
	struct pkt_ctx ctx = {};
	ctx.direction = direction;

	if (parse_packet(skb, &ctx) != PARSE_OK)
		return TC_ACT_OK; /* non-IP or unsupported: pass */

	/* Read conntrack_enabled flag (qf_config[0] bit0). */
	__u32 cfg_key = 0;
	__u32 *flags_ptr = bpf_map_lookup_elem(&qf_config, &cfg_key);
	__u8 ct_enabled = flags_ptr ? ((__u8)(*flags_ptr & 0x1)) : 0;

	/* Conntrack lookup before rule eval so CT-state rules can match. */
	__u8 ct_state = 0;
	if (ct_enabled) {
		ct_state = ct_lookup(&ctx);
		ctx.ct_state = ct_state;
	}

	__u32 matched_idx = 0;
	__u8  action      = eval_rules(&ctx, &matched_idx);

	if (action != 0) {
		/* Rule matched — fetch metadata for logging. */
		struct rule_entry *rule = bpf_map_lookup_elem(&qf_rules, &matched_idx);
		if (rule) {
			if (ct_enabled && action != ACTION_DENY)
				ct_update(&ctx);
			return apply_action(&ctx, action, matched_idx,
			                    rule->rule_id_hi, rule->rule_id_lo,
			                    rule->log_enabled, ct_state);
		}
		/* rule vanished between eval and fetch — treat as no-match */
	}

	/* No rule matched — apply default action (no counter, no log). */
	__u8 def = default_action(&ctx);
	if (ct_enabled && def != ACTION_DENY)
		ct_update(&ctx);
	return (def == ACTION_DENY) ? TC_ACT_SHOT : TC_ACT_OK;
}

SEC("tc")
int qf_tc_ingress(struct __sk_buff *skb)
{
	return run_datapath(skb, DIR_INGRESS);
}

SEC("tc")
int qf_tc_egress(struct __sk_buff *skb)
{
	return run_datapath(skb, DIR_EGRESS);
}

char __license[] SEC("license") = "GPL";
