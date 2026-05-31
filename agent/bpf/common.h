#pragma once

/* Types come from vmlinux.h included before this header.
 * Constants below are #defines not present in vmlinux.h. */

/* TC verdicts (linux/pkt_cls.h) */
#ifndef TC_ACT_OK
#define TC_ACT_OK   0
#endif
#ifndef TC_ACT_SHOT
#define TC_ACT_SHOT 2
#endif

/* EtherType (linux/if_ether.h) */
#ifndef ETH_P_IP
#define ETH_P_IP   0x0800
#endif
#ifndef ETH_P_IPV6
#define ETH_P_IPV6 0x86DD
#endif

/* IP protocols (linux/in.h) */
#ifndef IPPROTO_ICMP
#define IPPROTO_ICMP   1
#endif
#ifndef IPPROTO_TCP
#define IPPROTO_TCP    6
#endif
#ifndef IPPROTO_UDP
#define IPPROTO_UDP    17
#endif
#ifndef IPPROTO_ICMPV6
#define IPPROTO_ICMPV6 58
#endif

/* ICMP types (linux/icmp.h) */
#ifndef ICMP_ECHOREPLY
#define ICMP_ECHOREPLY      0
#endif
#ifndef ICMP_ECHO
#define ICMP_ECHO           8
#endif

/* ICMPv6 types (linux/icmpv6.h) */
#ifndef ICMPV6_ECHO_REQUEST
#define ICMPV6_ECHO_REQUEST 128
#endif
#ifndef ICMPV6_ECHO_REPLY
#define ICMPV6_ECHO_REPLY   129
#endif

/* TCP flag bitmask used in pkt_ctx.tcp_flags */
#define TCP_FLAG_FIN 0x01
#define TCP_FLAG_SYN 0x02
#define TCP_FLAG_RST 0x04
#define TCP_FLAG_PSH 0x08
#define TCP_FLAG_ACK 0x10
#define TCP_FLAG_URG 0x20

#define MAX_RULES       2048  /* qf_rules / qf_rule_counters map capacity */
#define CONNTRACK_MAX   65536

/* Maximum rules evaluated per packet in the BPF datapath.
 * Capped by the BPF verifier's jump-sequence complexity limit (~8192).
 * Each rule iteration costs ~72 verifier branches (nested CIDR/port loops);
 * 8192/72 ≈ 113 → use 64 as a safe Phase-1 bound.
 * Phase 2: replace the loop with bpf_loop() (kernel ≥5.17) to lift this. */
#define EVAL_MAX_RULES  64

/* Protocol constants — mirror proto.Protocol enum */
#define PROTO_ANY    1
#define PROTO_TCP    2
#define PROTO_UDP    3
#define PROTO_ICMP   4
#define PROTO_ICMPV6 5

/* Action constants — mirror proto.Action enum */
#define ACTION_ALLOW 1
#define ACTION_DENY  2
#define ACTION_LOG   3

/* Direction constants */
#define DIR_INGRESS 1
#define DIR_EGRESS  2

/* Conntrack state constants — mirror proto.ConntrackState */
#define CT_NONE        1
#define CT_NEW         2
#define CT_ESTABLISHED 3
#define CT_RELATED     4
#define CT_INVALID     5

/* IPv4 CIDR for rule matching */
struct cidr4 {
	__be32 addr; /* network byte order */
	__be32 mask;
};

/* Port range — inclusive [start, end]; single port: start == end */
struct port_range {
	__u16 start;
	__u16 end;
};

/* Compound key for the qf_ipsets LPM trie.
 * prefixlen = 32 (ipset_id exact match) + CIDR prefix length (0–32).
 * The kernel LPM engine exact-matches ipset_id then prefix-matches addr. */
struct ipset_lpm_key {
	__u32  prefixlen; /* 32 + /n */
	__u32  ipset_id;
	__be32 addr;      /* network address, big-endian */
};

/* Canonicalized 5-tuple used as conntrack key.
 * Canonical form: (initiator_ip, responder_ip, initiator_port, responder_port)
 * so both directions of a flow hash to the same key. */
struct ct_key {
	__be32 src_ip;
	__be32 dst_ip;
	__u16  src_port;
	__u16  dst_port;
	__u8   proto;
	__u8   _pad[3];
};

/* Conntrack table entry */
struct ct_entry {
	__u64 last_seen_ns;
	__u64 established_ns;
	__u64 packets_fwd;
	__u64 bytes_fwd;
	__u64 packets_rev;
	__u64 bytes_rev;
	__u8  state;     /* CT_* */
	__u8  tcp_state; /* internal TCP FSM state */
	__u8  _pad[6];
};

/* Match criteria embedded in each rule.
 * Small CIDR/port sets are inlined; large IPSets reference an LPM trie. */
struct rule_match {
	__u8  protocol;      /* PROTO_* */
	__u8  direction;     /* DIR_* */
	__u8  n_src_cidrs;
	__u8  n_dst_cidrs;
	__u8  n_src_ports;
	__u8  n_dst_ports;
	__u8  state;         /* CT_* predicate; CT_NONE = stateless */
	__u8  _pad;
	struct cidr4      src_cidrs[8];
	struct cidr4      dst_cidrs[8];
	struct port_range src_ports[8];
	struct port_range dst_ports[8];
	__u32 src_ipset_id;  /* 0 = unused; >0 = LPM trie map id */
	__u32 dst_ipset_id;
};

/* One entry in the qf_rules array map */
struct rule_entry {
	__u8  action;               /* ACTION_* */
	__u8  log_enabled;
	__u16 log_rate_limit_per_sec;
	__u32 _pad;
	__u64 rule_id_hi;           /* UUID upper 64 bits, for telemetry */
	__u64 rule_id_lo;
	struct rule_match match;
};

/* Per-rule traffic counter (per-CPU map value) */
struct rule_counter {
	__u64 packets;
	__u64 bytes;
};

/* Per-rule token bucket for log rate limiting (per-CPU map value).
 * last_ns=0 means uninitialized; tokens refills at log_rate_limit_per_sec/s. */
struct qf_token_bucket {
	__u64 last_ns;
	__u32 tokens;
	__u32 _pad;
};

/* Parsed packet context — filled by parse_packet() in parser.h.
 * Caller sets direction before calling; all other fields are output. */
struct pkt_ctx {
	__be32  src_ip4;    /* network byte order; 0 for IPv6 (Phase 1: no v6 CIDR) */
	__be32  dst_ip4;    /* network byte order */
	__u16   src_port;   /* host byte order; ICMP echo id for ICMP/ICMPv6 */
	__u16   dst_port;   /* host byte order */
	__u16   pkt_size;   /* total IP datagram size in bytes */
	__u8    tcp_flags;  /* TCP_FLAG_* bitmask; valid iff proto == PROTO_TCP */
	__u8    icmp_type;  /* valid iff proto == PROTO_ICMP or PROTO_ICMPV6 */
	__u8    icmp_code;  /* valid iff proto == PROTO_ICMP or PROTO_ICMPV6 */
	__u8    proto;      /* PROTO_* */
	__u8    direction;  /* DIR_* — set by caller before parse_packet() */
	__u8    is_ipv6;    /* 1 = IPv6; CIDR matching skipped in rule engine */
	__u8    ct_state;   /* CT_* from conntrack lookup; 0 = not tracked */
	__u8    _pad;
};

/* Ring buffer log event — emitted on LOG action or DENY with log_enabled */
struct log_event {
	__u64  ts_ns;
	__u64  rule_id_hi;
	__u64  rule_id_lo;
	__be32 src_ip;
	__be32 dst_ip;
	__u16  src_port;
	__u16  dst_port;
	__u16  pkt_size;
	__u16  tcp_flags;
	__u8   proto;
	__u8   direction;
	__u8   action;
	__u8   ct_state;
};
