package agent

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"

	"github.com/qf/qf/agent/internal/policy"
)

// LoadAndApply loads a PolicyBundle from path, optionally verifies its Ed25519
// signature when pubKey is non-nil, and applies it via a.Reload.
//
// Returns nil when the file does not exist (nothing to apply).
// Returns an error when the file exists but is corrupt, has an invalid
// signature, or fails to apply to the BPF datapath.
func (a *Agent) LoadAndApply(path string, pubKey ed25519.PublicKey) error {
	bundle, err := policy.LoadBundle(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("load bundle: %w", err)
	}

	if pubKey != nil {
		if err := policy.VerifyBundle(bundle, pubKey); err != nil {
			return fmt.Errorf("verify bundle: %w", err)
		}
	}

	if _, err := a.Reload(bundle); err != nil {
		return fmt.Errorf("apply bundle: %w", err)
	}
	return nil
}
