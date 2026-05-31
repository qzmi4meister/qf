#pragma once

/* vmlinux.h + bpf_helpers.h + maps.h must be included before this. */
#include "common.h"

/* Internal TCP FSM sub-states stored in ct_entry.tcp_state.
 * Distinct from the public CT_* values in ct_entry.state. */
#define TCP_CS_NONE        0
#define TCP_CS_SYN_SENT    1  /* SYN seen, awaiting SYN-ACK */
#define TCP_CS_SYN_RCVD    2  /* SYN-ACK seen, awaiting final ACK */
#define TCP_CS_ESTABLISHED 3  /* three-way handshake complete */
#define TCP_CS_FIN_WAIT    4  /* first FIN seen */
#define TCP_CS_LAST_ACK    5  /* second FIN seen */
#define TCP_CS_CLOSED      6  /* RST or both FINs acknowledged */

/* ── Canonical key ──────────────────────────────────────────────────── */

/* Build a direction-agnostic conntrack key so both directions of a flow
 * hash to the same entry.  Lower IP is placed in src.
 * Sets *is_reply=1 when packet travels responder→initiator. */
static __always_inline void
ct_build_key(struct pkt_ctx *ctx, struct ct_key *key, __u8 *is_reply)
{
	if (ctx->src_ip4 <= ctx->dst_ip4) {
		key->src_ip   = ctx->src_ip4;
		key->dst_ip   = ctx->dst_ip4;
		key->src_port = ctx->src_port;
		key->dst_port = ctx->dst_port;
		*is_reply = 0;
	} else {
		key->src_ip   = ctx->dst_ip4;
		key->dst_ip   = ctx->src_ip4;
		key->src_port = ctx->dst_port;
		key->dst_port = ctx->src_port;
		*is_reply = 1;
	}
	key->proto   = ctx->proto;
	key->_pad[0] = 0;
	key->_pad[1] = 0;
	key->_pad[2] = 0;
}

/* ── TCP FSM ────────────────────────────────────────────────────────── */

/* Advance TCP sub-state given current state, flags, and direction.
 * Returns the new TCP_CS_* value. */
static __always_inline __u8
tcp_fsm(__u8 cur, __u8 flags, __u8 is_reply)
{
	if (flags & TCP_FLAG_RST)
		return TCP_CS_CLOSED;

	switch (cur) {
	case TCP_CS_NONE:
		if ((flags & TCP_FLAG_SYN) && !(flags & TCP_FLAG_ACK))
			return TCP_CS_SYN_SENT;
		break;
	case TCP_CS_SYN_SENT:
		if (is_reply &&
		    (flags & (TCP_FLAG_SYN | TCP_FLAG_ACK)) ==
		     (TCP_FLAG_SYN | TCP_FLAG_ACK))
			return TCP_CS_SYN_RCVD;
		break;
	case TCP_CS_SYN_RCVD:
		if (!is_reply && (flags & TCP_FLAG_ACK) && !(flags & TCP_FLAG_SYN))
			return TCP_CS_ESTABLISHED;
		break;
	case TCP_CS_ESTABLISHED:
		if (flags & TCP_FLAG_FIN)
			return TCP_CS_FIN_WAIT;
		break;
	case TCP_CS_FIN_WAIT:
		if (flags & TCP_FLAG_FIN)
			return TCP_CS_LAST_ACK;
		break;
	case TCP_CS_LAST_ACK:
		if (flags & TCP_FLAG_ACK)
			return TCP_CS_CLOSED;
		break;
	}
	return cur;
}

/* Map internal TCP_CS_* sub-state → public CT_* state. */
static __always_inline __u8
tcp_cs_to_ct(__u8 tcp_state)
{
	if (tcp_state == TCP_CS_NONE)
		return 0;
	if (tcp_state <= TCP_CS_SYN_RCVD)
		return CT_NEW;
	if (tcp_state == TCP_CS_CLOSED)
		return CT_INVALID;
	return CT_ESTABLISHED; /* ESTABLISHED, FIN_WAIT, LAST_ACK */
}

/* ── Conntrack lookup ───────────────────────────────────────────────── */

/* Returns the current CT_* state for this flow, or 0 if not tracked. */
static __always_inline __u8
ct_lookup(struct pkt_ctx *ctx)
{
	struct ct_key key = {};
	__u8 is_reply = 0;
	ct_build_key(ctx, &key, &is_reply);

	struct ct_entry *e = bpf_map_lookup_elem(&qf_conntrack, &key);
	if (!e)
		return 0;
	return e->state;
}

/* ── Conntrack update ───────────────────────────────────────────────── */

/* Create or advance a conntrack entry for ctx.
 * Must be called only after the rule engine has ALLOW'd the packet.
 * Returns the new CT_* state, or 0 when the packet is not tracked
 * (e.g. mid-flow TCP with no existing entry). */
static __always_inline __u8
ct_update(struct pkt_ctx *ctx)
{
	struct ct_key key = {};
	__u8 is_reply = 0;
	ct_build_key(ctx, &key, &is_reply);

	struct ct_entry *e = bpf_map_lookup_elem(&qf_conntrack, &key);
	__u64 now = bpf_ktime_get_ns();

	if (e) {
		e->last_seen_ns = now;
		if (is_reply) {
			e->packets_rev++;
			e->bytes_rev += ctx->pkt_size;
		} else {
			e->packets_fwd++;
			e->bytes_fwd += ctx->pkt_size;
		}

		if (ctx->proto == PROTO_TCP) {
			__u8 new_tcp = tcp_fsm(e->tcp_state, ctx->tcp_flags,
			                       is_reply);
			if (new_tcp != e->tcp_state) {
				e->tcp_state = new_tcp;
				e->state     = tcp_cs_to_ct(new_tcp);
				if (new_tcp == TCP_CS_ESTABLISHED &&
				    !e->established_ns)
					e->established_ns = now;
			}
		} else {
			/* UDP / ICMP: promote to ESTABLISHED on first reply. */
			if (is_reply && e->state == CT_NEW)
				e->state = CT_ESTABLISHED;
		}
		return e->state;
	}

	/* No existing entry.  Skip mid-flow TCP — we can't track state
	 * without having seen the handshake. */
	if (ctx->proto == PROTO_TCP && !(ctx->tcp_flags & TCP_FLAG_SYN))
		return 0;

	struct ct_entry new_e = {};
	new_e.last_seen_ns = now;
	if (ctx->proto == PROTO_TCP) {
		__u8 ts = tcp_fsm(TCP_CS_NONE, ctx->tcp_flags, is_reply);
		new_e.tcp_state = ts;
		new_e.state     = tcp_cs_to_ct(ts);
	} else {
		new_e.state = CT_NEW;
	}
	bpf_map_update_elem(&qf_conntrack, &key, &new_e, BPF_ANY);
	return new_e.state;
}
