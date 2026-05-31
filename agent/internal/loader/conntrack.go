package loader

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"

	"github.com/cilium/ebpf"
)

// Internal TCP FSM sub-state constants — mirrors conntrack.h TCP_CS_* values.
const (
	TCPCSNone        uint8 = 0
	TCPCSSynSent     uint8 = 1
	TCPCSSynRcvd     uint8 = 2
	TCPCSEstablished uint8 = 3
	TCPCSFinWait     uint8 = 4
	TCPCSLastAck     uint8 = 5
	TCPCSClosed      uint8 = 6
)

// ConntrackKey is the 5-tuple used to identify or look up a flow.
// Either direction can be supplied to ConntrackLookup — the key is
// canonicalised automatically (lower IP placed in SrcIP).
type ConntrackKey struct {
	SrcIP    net.IP
	DstIP    net.IP
	SrcPort  uint16
	DstPort  uint16
	Protocol uint8 // Proto* constants
}

// ConntrackEntry is a point-in-time snapshot of one tracked flow.
type ConntrackEntry struct {
	Key           ConntrackKey // canonical: lower IP in SrcIP
	State         uint8        // CT* public state
	TCPState      uint8        // TCPCS* internal sub-state; 0 for non-TCP flows
	LastSeenNs    uint64
	EstablishedNs uint64       // 0 until TCP reaches ESTABLISHED
	PacketsFwd    uint64       // initiator → responder
	BytesFwd      uint64
	PacketsRev    uint64       // responder → initiator
	BytesRev      uint64
}

// ConntrackDump returns all entries currently in the BPF conntrack table.
func ConntrackDump(objs *TcFilterObjects) ([]ConntrackEntry, error) {
	var out []ConntrackEntry
	iter := objs.QfConntrack.Iterate()
	var k TcFilterCtKey
	var v TcFilterCtEntry
	for iter.Next(&k, &v) {
		out = append(out, fromCtKV(k, v))
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("conntrack iterate: %w", err)
	}
	return out, nil
}

// ConntrackLookup looks up a single flow by 5-tuple.
// Returns nil (no error) when the flow is not in the table.
func ConntrackLookup(objs *TcFilterObjects, key ConntrackKey) (*ConntrackEntry, error) {
	bk := canonicalCtKey(key)
	var v TcFilterCtEntry
	if err := objs.QfConntrack.Lookup(bk, &v); err != nil {
		if errors.Is(err, ebpf.ErrKeyNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("conntrack lookup: %w", err)
	}
	e := fromCtKV(bk, v)
	return &e, nil
}

// Loader convenience wrappers.

func (l *Loader) ConntrackDump() ([]ConntrackEntry, error) {
	return ConntrackDump(&l.objs)
}

func (l *Loader) ConntrackLookup(key ConntrackKey) (*ConntrackEntry, error) {
	return ConntrackLookup(&l.objs, key)
}

// ── internals ────────────────────────────────────────────────────────────

func fromCtKV(k TcFilterCtKey, v TcFilterCtEntry) ConntrackEntry {
	return ConntrackEntry{
		Key: ConntrackKey{
			SrcIP:    decodeIP4(k.SrcIp),
			DstIP:    decodeIP4(k.DstIp),
			SrcPort:  k.SrcPort,
			DstPort:  k.DstPort,
			Protocol: k.Proto,
		},
		State:         v.State,
		TCPState:      v.TcpState,
		LastSeenNs:    v.LastSeenNs,
		EstablishedNs: v.EstablishedNs,
		PacketsFwd:    v.PacketsFwd,
		BytesFwd:      v.BytesFwd,
		PacketsRev:    v.PacketsRev,
		BytesRev:      v.BytesRev,
	}
}

// canonicalCtKey builds the direction-agnostic BPF key (lower IP in SrcIp).
// Mirrors ct_build_key() in conntrack.h.
func canonicalCtKey(k ConntrackKey) TcFilterCtKey {
	s := encodeIP4(k.SrcIP)
	d := encodeIP4(k.DstIP)
	bk := TcFilterCtKey{Proto: k.Protocol}
	if s <= d {
		bk.SrcIp, bk.DstIp = s, d
		bk.SrcPort, bk.DstPort = k.SrcPort, k.DstPort
	} else {
		bk.SrcIp, bk.DstIp = d, s
		bk.SrcPort, bk.DstPort = k.DstPort, k.SrcPort
	}
	return bk
}

// encodeIP4 converts an IPv4 address to the BPF __be32 uint32 representation.
// On x86_64, __be32 map fields store network-order bytes interpreted as a
// little-endian integer, so LittleEndian.Uint32(ip.To4()) gives the correct
// host integer value.
func encodeIP4(ip net.IP) uint32 {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(ip4)
}

// decodeIP4 is the inverse of encodeIP4.
func decodeIP4(v uint32) net.IP {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return net.IP(b)
}
