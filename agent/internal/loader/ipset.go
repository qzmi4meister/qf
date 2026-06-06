package loader

import (
	"fmt"

	"github.com/cilium/ebpf"
)

const (
	// IPSetInlineMax is the maximum number of CIDRs that fit inline in a
	// rule_match struct. Exceeding this triggers an IPSet spill in the compiler.
	IPSetInlineMax = 8
	// MaxIPSetEntries is the total LPM trie capacity shared across all IPSets.
	MaxIPSetEntries = 65536
)

// PushIPSet writes all cidrs for IPSet id into the qf_ipsets LPM trie.
// id must be > 0. Existing entries for this id are NOT cleared first —
// call ClearIPSets before a full reload.
func PushIPSet(objs *BpfObjects, id uint32, cidrs []CIDR4) error {
	if id == 0 {
		return fmt.Errorf("ipset id 0 is reserved")
	}
	for i, c := range cidrs {
		if c.IP.To4() == nil {
			return fmt.Errorf("ipset[%d][%d]: IPv6 not supported", id, i)
		}
		if c.Ones < 0 || c.Ones > 32 {
			return fmt.Errorf("ipset[%d][%d]: invalid prefix length %d", id, i, c.Ones)
		}
		key := TcFilterIpsetLpmKey{
			Prefixlen: uint32(32 + c.Ones),
			IpsetId:   id,
			Addr:      encodeIP4(c.IP),
		}
		if err := objs.QfIpsets.Update(key, uint8(1), ebpf.UpdateAny); err != nil {
			return fmt.Errorf("ipset[%d] update: %w", id, err)
		}
	}
	return nil
}

// ClearIPSets deletes all entries from the qf_ipsets LPM trie.
func ClearIPSets(objs *BpfObjects) error {
	var key TcFilterIpsetLpmKey
	var val uint8
	var toDelete []TcFilterIpsetLpmKey
	iter := objs.QfIpsets.Iterate()
	for iter.Next(&key, &val) {
		toDelete = append(toDelete, key)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("iterate ipsets: %w", err)
	}
	for _, k := range toDelete {
		if err := objs.QfIpsets.Delete(k); err != nil {
			return fmt.Errorf("delete ipset entry: %w", err)
		}
	}
	return nil
}

// Loader convenience wrappers.

func (l *Loader) PushIPSet(id uint32, cidrs []CIDR4) error {
	return PushIPSet(&l.objs, id, cidrs)
}

func (l *Loader) ClearIPSets() error {
	return ClearIPSets(&l.objs)
}
