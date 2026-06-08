package loader

import (
	"fmt"
	"os"
	"strings"

	"github.com/cilium/ebpf"
)

// BpfObjects is a kernel-version-agnostic handle over loaded BPF programs and maps.
// Populated from TcFilterObjects (kernel ≥5.17) or TcFilterCompatObjects (5.15–5.16).
type BpfObjects struct {
	// Programs
	QfTcIngress *ebpf.Program
	QfTcEgress  *ebpf.Program
	// Maps
	QfRules           *ebpf.Map
	QfRuleCount       *ebpf.Map
	QfRuleCounters    *ebpf.Map
	QfConfig          *ebpf.Map
	QfEvents          *ebpf.Map
	QfIpsets          *ebpf.Map
	QfConntrack       *ebpf.Map
	QfSuppressedCount *ebpf.Map
	// maxRules is the compile-time EVAL_MAX_RULES for this variant.
	maxRules int
	// close releases all underlying BPF resources.
	close func()
}

// Close releases all BPF resources held by this object set.
func (b *BpfObjects) Close() {
	if b.close != nil {
		b.close()
	}
}

func bpfObjectsFromFull(o *TcFilterObjects) BpfObjects {
	return BpfObjects{
		QfTcIngress:       o.QfTcIngress,
		QfTcEgress:        o.QfTcEgress,
		QfRules:           o.QfRules,
		QfRuleCount:       o.QfRuleCount,
		QfRuleCounters:    o.QfRuleCounters,
		QfConfig:          o.QfConfig,
		QfEvents:          o.QfEvents,
		QfIpsets:          o.QfIpsets,
		QfConntrack:       o.QfConntrack,
		QfSuppressedCount: o.QfSuppressedCount,
		maxRules:          64,
		close:             func() { o.Close() }, //nolint:errcheck
	}
}

func bpfObjectsFromCompat(o *TcFilterCompatObjects) BpfObjects {
	return BpfObjects{
		QfTcIngress:       o.QfTcIngress,
		QfTcEgress:        o.QfTcEgress,
		QfRules:           o.QfRules,
		QfRuleCount:       o.QfRuleCount,
		QfRuleCounters:    o.QfRuleCounters,
		QfConfig:          o.QfConfig,
		QfEvents:          o.QfEvents,
		QfIpsets:          o.QfIpsets,
		QfConntrack:       o.QfConntrack,
		QfSuppressedCount: o.QfSuppressedCount,
		maxRules:          32,
		close:             func() { o.Close() }, //nolint:errcheck
	}
}

// KernelVer encodes major.minor.patch into a single uint32 for comparison.
func KernelVer(major, minor, patch uint32) uint32 {
	return (major << 16) | (minor << 8) | patch
}

// KernelVersion returns the running kernel version encoded by KernelVer.
// Returns 0 on error (treated as ancient kernel → compat path).
func KernelVersion() uint32 {
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return 0
	}
	return ParseKernelVersion(strings.TrimSpace(string(data)))
}

// ParseKernelVersion extracts major.minor.patch from a kernel release string.
// Handles non-standard suffixes like "5.15.0-179-generic" or "6.12-rc1".
func ParseKernelVersion(rel string) uint32 {
	// Split on any non-digit character and collect the first three numeric tokens.
	tokens := strings.FieldsFunc(rel, func(r rune) bool { return r < '0' || r > '9' })
	nums := make([]uint32, 0, 3)
	for _, t := range tokens {
		if len(t) == 0 {
			continue
		}
		var n uint32
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			nums = append(nums, n)
		}
		if len(nums) == 3 {
			break
		}
	}
	for len(nums) < 3 {
		nums = append(nums, 0)
	}
	return KernelVer(nums[0], nums[1], nums[2])
}
