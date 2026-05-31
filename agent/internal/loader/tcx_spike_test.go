//go:build integration

package loader

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestTcxAttach attaches qf programs to eth0 via TCX and verifies the chain
// through bpftool net. Requires root and a real network interface.
func TestTcxAttach(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root")
	}

	const iface = "eth0"

	l, err := Load(iface)
	if err != nil {
		t.Fatalf("Load(%q): %v", iface, err)
	}

	// --- verify programs appear in TCX chain ---
	out, err := exec.Command("bpftool", "net", "show", "dev", iface).CombinedOutput()
	if err != nil {
		l.Close()
		t.Fatalf("bpftool net: %v\n%s", err, out)
	}
	t.Logf("bpftool net (attached):\n%s", out)

	s := string(out)
	for _, want := range []string{"tcx", "qf_tc_ingress", "qf_tc_egress"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in bpftool output", want)
		}
	}

	// --- detach ---
	l.Close()

	out2, err := exec.Command("bpftool", "net", "show", "dev", iface).CombinedOutput()
	if err != nil {
		t.Fatalf("bpftool net after close: %v\n%s", err, out2)
	}
	t.Logf("bpftool net (detached):\n%s", out2)

	s2 := string(out2)
	if strings.Contains(s2, "qf_tc_ingress") || strings.Contains(s2, "qf_tc_egress") {
		t.Errorf("programs still present after Close()")
	}
}
