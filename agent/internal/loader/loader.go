package loader

import (
	"fmt"
	"net"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
)

// Full build (kernel ≥5.17): bpf_loop(), 64 rules.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang TcFilter ../../bpf/tc_filter.c -- -g -O2 -Wall -target bpf -D__TARGET_ARCH_x86 -I../../bpf -DEVAL_MAX_RULES=64 -DUSE_BPF_LOOP

// Compat build (kernel 5.15–5.16): bounded loop, 32 rules.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang TcFilterCompat ../../bpf/tc_filter.c -- -g -O2 -Wall -target bpf -D__TARGET_ARCH_x86 -I../../bpf -DEVAL_MAX_RULES=32

// Loader holds loaded BPF objects and their TCX attachment links.
type Loader struct {
	objs  BpfObjects
	links []link.Link
}

// Load loads BPF programs from the embedded object and attaches them to
// iface via TCX (requires kernel ≥6.6).
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

	ing, err := link.AttachTCX(link.TCXOptions{
		Interface: ni.Index,
		Program:   l.objs.QfTcIngress,
		Attach:    ebpf.AttachTCXIngress,
	})
	if err != nil {
		l.objs.Close()
		if isTCXUnsupported(err) {
			return nil, fmt.Errorf("TCX not supported on %s (kernel ≥6.6 required; legacy TC fallback is planned for Phase 2): %w", iface, err)
		}
		return nil, fmt.Errorf("attach ingress tcx on %s: %w", iface, err)
	}
	l.links = append(l.links, ing)

	egr, err := link.AttachTCX(link.TCXOptions{
		Interface: ni.Index,
		Program:   l.objs.QfTcEgress,
		Attach:    ebpf.AttachTCXEgress,
	})
	if err != nil {
		for _, lnk := range l.links {
			lnk.Close()
		}
		l.objs.Close()
		if isTCXUnsupported(err) {
			return nil, fmt.Errorf("TCX not supported on %s (kernel ≥6.6 required; legacy TC fallback is planned for Phase 2): %w", iface, err)
		}
		return nil, fmt.Errorf("attach egress tcx on %s: %w", iface, err)
	}
	l.links = append(l.links, egr)

	return l, nil
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
// does not support TCX (pre-6.6). Checked by error string because the exact
// errno varies by kernel version.
func isTCXUnsupported(err error) bool {
	s := err.Error()
	return strings.Contains(s, "not supported") ||
		strings.Contains(s, "operation not supported") ||
		strings.Contains(s, "no such file")
}

// Close detaches all TC programs and releases BPF resources.
func (l *Loader) Close() {
	for _, lnk := range l.links {
		lnk.Close()
	}
	l.objs.Close()
}

// MaxRules returns the compile-time rule limit for the loaded BPF variant.
func (l *Loader) MaxRules() int {
	return l.objs.maxRules
}
