package agent

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"

	"github.com/qf/qf/agent/internal/policy"
)

// LoadAndApply loads a PolicyBundle from path, verifies its Ed25519 signature
// against pubKey, and applies it via a.Reload.
//
// Returns nil when the file does not exist (nothing to apply).
// Returns an error when the file exists but is corrupt, fails signature
// verification, or fails to apply to the BPF datapath.
// Signature verification is always performed; passing a nil pubKey results
// in ErrInvalidSignature for signed bundles and ErrNoSignature for unsigned ones.
func (a *Agent) LoadAndApply(path string, pubKey ed25519.PublicKey) error {
	bundle, err := policy.LoadBundle(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load bundle: %w", err)
	}

	if err := policy.VerifyBundle(bundle, pubKey); err != nil {
		return fmt.Errorf("verify bundle: %w", err)
	}

	if _, err := a.Reload(bundle); err != nil {
		return fmt.Errorf("apply bundle: %w", err)
	}
	return nil
}
