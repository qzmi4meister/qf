package loader

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// Full build (kernel ≥5.17): bpf_loop(), 64 rules.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang TcFilter ../../bpf/tc_filter.c -- -g -O2 -Wall -target bpf -D__TARGET_ARCH_x86 -I../../bpf -DEVAL_MAX_RULES=64 -DUSE_BPF_LOOP

// Compat build (kernel 5.15–5.16): bounded loop, 32 rules.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang TcFilterCompat ../../bpf/tc_filter.c -- -g -O2 -Wall -target bpf -D__TARGET_ARCH_x86 -I../../bpf -DEVAL_MAX_RULES=32

// Loader holds loaded BPF objects and their attachment links.
// On kernels ≥6.6 TCX is used; on older kernels classic TC (clsact + cls_bpf)
// is used as a fallback.
type Loader struct {
	objs      BpfObjects
	links     []link.Link          // TCX links (kernel ≥6.6)
	tcFilters []*netlink.BpfFilter // classic TC filters (kernel <6.6)
}

// Load loads BPF programs and attaches them to iface.
// Tries TCX first; falls back to classic TC (clsact qdisc + cls_bpf) on
// kernels that do not support TCX (<6.6).
func Load(iface string) (*Loader, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("remove memlock: %w", err)
	}

	objs, err := loadBpfObjects()
	if err != nil {
		return nil, fmt.Errorf("load bpf objects: %w", err)
	}

	l := &Loader{objs: objs}

	if err := SetConfig(&l.objs, DefaultConfig); err != nil {
		l.objs.Close()
		return nil, fmt.Errorf("init config: %w", err)
	}

	ni, err := net.InterfaceByName(iface)
	if err != nil {
		l.objs.Close()
		return nil, fmt.Errorf("interface %q: %w", iface, err)
	}

	if err := l.attachTCX(ni); err != nil {
		if !isTCXUnsupported(err) {
			l.objs.Close()
			return nil, err
		}
		// TCX not available — fall back to classic TC.
		if err := l.attachClassicTC(ni); err != nil {
			l.objs.Close()
			return nil, err
		}
	}

	return l, nil
}

func (l *Loader) attachTCX(ni *net.Interface) error {
	ing, err := link.AttachTCX(link.TCXOptions{
		Interface: ni.Index,
		Program:   l.objs.QfTcIngress,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		return fmt.Errorf("attach ingress tcx on %s: %w", ni.Name, err)
	}
	l.links = append(l.links, ing)

	egr, err := link.AttachTCX(link.TCXOptions{
		Interface: ni.Index,
		Program:   l.objs.QfTcEgress,
		Attach:    ebpf.AttachTCXEgress,
	})
	if err != nil {
		ing.Close()
		l.links = l.links[:len(l.links)-1]
		return fmt.Errorf("attach egress tcx on %s: %w", ni.Name, err)
	}
	l.links = append(l.links, egr)
	return nil
}

// attachClassicTC attaches BPF programs via clsact qdisc + cls_bpf filters.
// Works on kernels ≥4.1 (clsact) without TCX support.
func (l *Loader) attachClassicTC(ni *net.Interface) error {
	// Ensure clsact qdisc exists on the interface.
	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: netlink.QdiscAttrs{
			LinkIndex: ni.Index,
			Handle:    netlink.MakeHandle(0xffff, 0),
			Parent:    netlink.HANDLE_CLSACT,
		},
		QdiscType: "clsact",
	}
	if err := netlink.QdiscAdd(qdisc); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("add clsact qdisc on %s: %w", ni.Name, err)
	}

	ingFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ni.Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  unix.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           l.objs.QfTcIngress.FD(),
		Name:         "qf_ingress",
		DirectAction: true,
	}
	if err := netlink.FilterReplace(ingFilter); err != nil {
		return fmt.Errorf("attach ingress tc filter on %s: %w", ni.Name, err)
	}
	l.tcFilters = append(l.tcFilters, ingFilter)

	egrFilter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: ni.Index,
			Parent:    netlink.HANDLE_MIN_EGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  unix.ETH_P_ALL,
			Priority:  1,
		},
		Fd:           l.objs.QfTcEgress.FD(),
		Name:         "qf_egress",
		DirectAction: true,
	}
	if err := netlink.FilterReplace(egrFilter); err != nil {
		netlink.FilterDel(ingFilter) //nolint:errcheck
		return fmt.Errorf("attach egress tc filter on %s: %w", ni.Name, err)
	}
	l.tcFilters = append(l.tcFilters, egrFilter)

	return nil
}

// loadBpfObjects selects the appropriate BPF variant based on kernel version.
func loadBpfObjects() (BpfObjects, error) {
	if KernelVersion() >= KernelVer(5, 17, 0) {
		var o TcFilterObjects
		if err := LoadTcFilterObjects(&o, nil); err != nil {
			return BpfObjects{}, err
		}
		return bpfObjectsFromFull(&o), nil
	}
	var o TcFilterCompatObjects
	if err := LoadTcFilterCompatObjects(&o, nil); err != nil {
		return BpfObjects{}, err
	}
	return bpfObjectsFromCompat(&o), nil
}

// isTCXUnsupported returns true when the TCX attach error indicates the kernel
// does not support TCX (pre-6.6). Uses errors.Is against syscall sentinels;
// EOPNOTSUPP covers "operation not supported", ENOSYS covers "function not
// implemented", ENOENT covers the bpf(BPF_LINK_CREATE) path on older kernels.
func isTCXUnsupported(err error) bool {
	return errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.ENOENT)
}

// Close detaches all TC programs and releases BPF resources.
func (l *Loader) Close() {
	for _, lnk := range l.links {
		lnk.Close()
	}
	for _, f := range l.tcFilters {
		netlink.FilterDel(f) //nolint:errcheck
	}
	l.objs.Close()
}

// MaxRules returns the compile-time rule limit for the loaded BPF variant.
func (l *Loader) MaxRules() int {
	return l.objs.maxRules
}
