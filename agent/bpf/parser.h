#pragma once

/* vmlinux.h + bpf_helpers.h + bpf_endian.h must be included before this. */
#include "common.h"

#define PARSE_OK    0
#define PARSE_SKIP (-1) /* non-IP, fragment, unsupported L4, truncated */

/* ── IPv4 ─────────────────────────────────────────────────────────────── */

static __always_inline int
parse_ipv4(void *l3, void *data_end, struct pkt_ctx *ctx)
{
	struct iphdr *ip = l3;
	if ((void *)(ip + 1) > data_end)
		return PARSE_SKIP;

	/* Drop fragments: MF bit or non-zero fragment offset. */
	if (ip->frag_off & bpf_htons(0x3FFF))
		return PARSE_SKIP;

	__u8 ihl = ip->ihl & 0xF;
	if (ihl < 5)
		return PARSE_SKIP;

	ctx->src_ip4  = ip->saddr;
	ctx->dst_ip4  = ip->daddr;
	ctx->pkt_size = bpf_ntohs(ip->tot_len);
	ctx->is_ipv6  = 0;

	void *l4 = (void *)ip + ((__u32)ihl << 2);

	switch (ip->protocol) {
	case IPPROTO_TCP: {
		struct tcphdr *tcp = l4;
		if ((void *)(tcp + 1) > data_end)
			return PARSE_SKIP;
		ctx->src_port  = bpf_ntohs(tcp->source);
		ctx->dst_port  = bpf_ntohs(tcp->dest);
		ctx->tcp_flags = (tcp->fin ? TCP_FLAG_FIN : 0) |
		                 (tcp->syn ? TCP_FLAG_SYN : 0) |
		                 (tcp->rst ? TCP_FLAG_RST : 0) |
		                 (tcp->psh ? TCP_FLAG_PSH : 0) |
		                 (tcp->ack ? TCP_FLAG_ACK : 0) |
		                 (tcp->urg ? TCP_FLAG_URG : 0);
		ctx->proto = PROTO_TCP;
		break;
	}
	case IPPROTO_UDP: {
		struct udphdr *udp = l4;
		if ((void *)(udp + 1) > data_end)
			return PARSE_SKIP;
		ctx->src_port = bpf_ntohs(udp->source);
		ctx->dst_port = bpf_ntohs(udp->dest);
		ctx->proto    = PROTO_UDP;
		break;
	}
	case IPPROTO_ICMP: {
		struct icmphdr *icmp = l4;
		if ((void *)(icmp + 1) > data_end)
			return PARSE_SKIP;
		ctx->icmp_type = icmp->type;
		ctx->icmp_code = icmp->code;
		ctx->proto     = PROTO_ICMP;
		/* Store echo identifier in both src_port and dst_port so the
		 * canonical conntrack key is symmetric across request/reply. */
		if (icmp->type == ICMP_ECHO || icmp->type == ICMP_ECHOREPLY) {
			ctx->src_port = bpf_ntohs(icmp->un.echo.id);
			ctx->dst_port = bpf_ntohs(icmp->un.echo.id);
		}
		break;
	}
	default:
		return PARSE_SKIP;
	}
	return PARSE_OK;
}

/* ── IPv6 ─────────────────────────────────────────────────────────────── */

static __always_inline int
parse_ipv6(void *l3, void *data_end, struct pkt_ctx *ctx)
{
	struct ipv6hdr *ip6 = l3;
	if ((void *)(ip6 + 1) > data_end)
		return PARSE_SKIP;

	/* Phase 1: no IPv6 CIDR matching — zero out IPv4 address fields.
	 * Rule engine will skip CIDR checks when is_ipv6 == 1. */
	ctx->src_ip4  = 0;
	ctx->dst_ip4  = 0;
	ctx->pkt_size = bpf_ntohs(ip6->payload_len) + sizeof(*ip6);
	ctx->is_ipv6  = 1;

	void *l4 = (void *)(ip6 + 1);

	/* Note: extension headers not followed — nexthdr assumed to be L4. */
	switch (ip6->nexthdr) {
	case IPPROTO_TCP: {
		struct tcphdr *tcp = l4;
		if ((void *)(tcp + 1) > data_end)
			return PARSE_SKIP;
		ctx->src_port  = bpf_ntohs(tcp->source);
		ctx->dst_port  = bpf_ntohs(tcp->dest);
		ctx->tcp_flags = (tcp->fin ? TCP_FLAG_FIN : 0) |
		                 (tcp->syn ? TCP_FLAG_SYN : 0) |
		                 (tcp->rst ? TCP_FLAG_RST : 0) |
		                 (tcp->psh ? TCP_FLAG_PSH : 0) |
		                 (tcp->ack ? TCP_FLAG_ACK : 0) |
		                 (tcp->urg ? TCP_FLAG_URG : 0);
		ctx->proto = PROTO_TCP;
		break;
	}
	case IPPROTO_UDP: {
		struct udphdr *udp = l4;
		if ((void *)(udp + 1) > data_end)
			return PARSE_SKIP;
		ctx->src_port = bpf_ntohs(udp->source);
		ctx->dst_port = bpf_ntohs(udp->dest);
		ctx->proto    = PROTO_UDP;
		break;
	}
	case IPPROTO_ICMPV6: {
		struct icmp6hdr *icmp6 = l4;
		if ((void *)(icmp6 + 1) > data_end)
			return PARSE_SKIP;
		ctx->icmp_type = icmp6->icmp6_type;
		ctx->icmp_code = icmp6->icmp6_code;
		ctx->proto     = PROTO_ICMPV6;
		if (icmp6->icmp6_type == ICMPV6_ECHO_REQUEST ||
		    icmp6->icmp6_type == ICMPV6_ECHO_REPLY) {
			ctx->src_port = bpf_ntohs(
			    icmp6->icmp6_dataun.u_echo.identifier);
			ctx->dst_port = ctx->src_port;
		}
		break;
	}
	default:
		return PARSE_SKIP;
	}
	return PARSE_OK;
}

/* ── Entry point ──────────────────────────────────────────────────────── */

/* parse_packet — extract L3/L4 fields from skb into ctx.
 * Caller must initialise ctx to zero and set ctx->direction before calling.
 * Returns PARSE_OK on success, PARSE_SKIP if the packet should be passed
 * without rule evaluation (non-IP, fragment, unsupported protocol). */
static __always_inline int
parse_packet(struct __sk_buff *skb, struct pkt_ctx *ctx)
{
	void *data     = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;

	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return PARSE_SKIP;

	void  *l3         = (void *)(eth + 1);
	__be16 eth_proto  = eth->h_proto;

	if (eth_proto == bpf_htons(ETH_P_IP))
		return parse_ipv4(l3, data_end, ctx);

	if (eth_proto == bpf_htons(ETH_P_IPV6))
		return parse_ipv6(l3, data_end, ctx);

	return PARSE_SKIP;
}
