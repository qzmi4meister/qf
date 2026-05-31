package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/handler"
	"github.com/qf/qf/agent/internal/policy"
)

// newBootTestAgent creates an Agent with a mock applier and a no-op event
// source.  Suitable for boot tests that don't need real BPF.
func newBootTestAgent(applier handler.RuleApplier) *Agent {
	return &Agent{
		policy: handler.NewPolicyHandler(applier),
		log:    log.New(io.Discard, "", 0),
		newReader: func() (eventSource, error) {
			return newBlockingSource(), nil
		},
	}
}

func genKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	return pub, priv
}

func writeBundle(t *testing.T, dir string, bundle *qfv1.PolicyBundle) string {
	t.Helper()
	path := filepath.Join(dir, "policy.blob")
	if err := policy.SaveBundle(path, bundle); err != nil {
		t.Fatalf("SaveBundle: %v", err)
	}
	return path
}

// TestBoot_LoadAndApply_Success verifies the golden path: bundle on disk →
// LoadAndApply → agent has the bundle applied.
func TestBoot_LoadAndApply_Success(t *testing.T) {
	pub, priv := genKeyPair(t)
	m := &mockApplier{}
	a := newBootTestAgent(m)

	bundle := validBundle(42)
	if err := policy.SignBundle(bundle, priv); err != nil {
		t.Fatalf("SignBundle: %v", err)
	}

	dir := t.TempDir()
	path := writeBundle(t, dir, bundle)

	if err := a.LoadAndApply(path, pub); err != nil {
		t.Fatalf("LoadAndApply: %v", err)
	}

	st := a.PolicyStatus()
	if st == nil {
		t.Fatal("PolicyStatus should not be nil after LoadAndApply")
	}
	if st.Generation != 42 {
		t.Errorf("Generation: want 42, got %d", st.Generation)
	}
	if m.callCount() != 1 {
		t.Errorf("PushRules call count: want 1, got %d", m.callCount())
	}
}

// TestBoot_LoadAndApply_NoFile verifies that a missing bundle is not an error
// (agent starts with no policy applied).
func TestBoot_LoadAndApply_NoFile(t *testing.T) {
	pub, _ := genKeyPair(t)
	m := &mockApplier{}
	a := newBootTestAgent(m)

	err := a.LoadAndApply("/nonexistent/policy.blob", pub)
	if err != nil {
		t.Fatalf("want nil for missing bundle, got %v", err)
	}
	if a.PolicyStatus() != nil {
		t.Error("PolicyStatus should be nil when no bundle was loaded")
	}
}

// TestBoot_LoadAndApply_InvalidSignature verifies that a bundle with a bad
// signature is rejected and the policy is not applied.
func TestBoot_LoadAndApply_InvalidSignature(t *testing.T) {
	pub, _ := genKeyPair(t)   // key pair A (for verify)
	_, priv2 := genKeyPair(t) // key pair B (for sign — mismatch)

	m := &mockApplier{}
	a := newBootTestAgent(m)

	bundle := validBundle(7)
	if err := policy.SignBundle(bundle, priv2); err != nil {
		t.Fatalf("SignBundle: %v", err)
	}

	dir := t.TempDir()
	path := writeBundle(t, dir, bundle)

	err := a.LoadAndApply(path, pub)
	if err == nil {
		t.Fatal("want error for signature mismatch, got nil")
	}
	if !errors.Is(err, policy.ErrInvalidSignature) {
		t.Errorf("want ErrInvalidSignature wrapped in error, got %v", err)
	}
	if a.PolicyStatus() != nil {
		t.Error("PolicyStatus must be nil when signature rejected")
	}
}

// TestBoot_LoadAndApply_NoSignatureCheck verifies that passing pubKey=nil
// skips signature verification entirely.
func TestBoot_LoadAndApply_NoSignatureCheck(t *testing.T) {
	m := &mockApplier{}
	a := newBootTestAgent(m)

	// Bundle with no signature at all.
	bundle := validBundle(3)
	dir := t.TempDir()
	path := writeBundle(t, dir, bundle)

	if err := a.LoadAndApply(path, nil); err != nil {
		t.Fatalf("LoadAndApply without sig check: %v", err)
	}
	if a.PolicyStatus() == nil {
		t.Error("PolicyStatus should not be nil")
	}
}

// TestBoot_LoadAndApply_CorruptFile verifies that a corrupt bundle file
// returns an error and does not apply any policy.
func TestBoot_LoadAndApply_CorruptFile(t *testing.T) {
	m := &mockApplier{}
	a := newBootTestAgent(m)

	dir := t.TempDir()
	path := filepath.Join(dir, "policy.blob")
	if err := os.WriteFile(path, []byte("not a valid protobuf"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}

	err := a.LoadAndApply(path, nil)
	if err == nil {
		t.Fatal("want error for corrupt bundle, got nil")
	}
}

// TestBoot_SaveLoad_RoundTrip verifies that SaveBundle + LoadBundle preserve
// all fields exactly.
func TestBoot_SaveLoad_RoundTrip(t *testing.T) {
	pub, priv := genKeyPair(t)

	bundle := validBundle(99)
	if err := policy.SignBundle(bundle, priv); err != nil {
		t.Fatalf("SignBundle: %v", err)
	}

	dir := t.TempDir()
	path := writeBundle(t, dir, bundle)

	loaded, err := policy.LoadBundle(path)
	if err != nil {
		t.Fatalf("LoadBundle: %v", err)
	}

	if loaded.GetGeneration() != 99 {
		t.Errorf("Generation: want 99, got %d", loaded.GetGeneration())
	}
	if len(loaded.GetSignature()) == 0 {
		t.Error("Signature should be preserved after round-trip")
	}
	if err := policy.VerifyBundle(loaded, pub); err != nil {
		t.Errorf("VerifyBundle after round-trip: %v", err)
	}
}
