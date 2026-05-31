package grpcclient

import (
	"crypto/ed25519"
	"errors"
	"fmt"

	"github.com/qf/qf/agent/internal/policy"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// HandleBundle verifies bundle signature, sends BundleAck, applies via applyFn,
// then sends BundleApplied. applyFn returns (durationMs, error).
// A bad signature sends BundleAck with signature_verified=false but does NOT apply.
func HandleBundle(
	stream qfv1.AgentService_StreamClient,
	bundle *qfv1.PolicyBundle,
	pubKey ed25519.PublicKey,
	applyFn func(*qfv1.PolicyBundle) (uint32, error),
) error {
	gen := bundle.GetGeneration()

	// Verify signature.
	sigErr := policy.VerifyBundle(bundle, pubKey)
	ack := &qfv1.BundleAck{
		Generation:        gen,
		SignatureVerified:  sigErr == nil,
	}
	if sigErr != nil && !errors.Is(sigErr, policy.ErrNoSignature) {
		ack.ErrorMessage = sigErr.Error()
	}
	if err := stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_BundleAck{BundleAck: ack},
	}); err != nil {
		return fmt.Errorf("bundle: send ack: %w", err)
	}

	// Reject unsigned/bad-sig bundles — do not apply.
	if sigErr != nil {
		applied := &qfv1.BundleApplied{
			Generation:   gen,
			Success:      false,
			ErrorMessage: fmt.Sprintf("signature: %v", sigErr),
		}
		return stream.Send(&qfv1.AgentMessage{
			Payload: &qfv1.AgentMessage_BundleApplied{BundleApplied: applied},
		})
	}

	// Apply bundle to BPF datapath.
	durMs, applyErr := applyFn(bundle)
	var applied *qfv1.BundleApplied
	if applyErr != nil {
		applied = &qfv1.BundleApplied{
			Generation:   gen,
			Success:      false,
			ErrorMessage: applyErr.Error(),
		}
	} else {
		applied = &qfv1.BundleApplied{
			Generation: gen,
			Success:    true,
			DurationMs: durMs,
		}
	}
	if err := stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_BundleApplied{BundleApplied: applied},
	}); err != nil {
		return fmt.Errorf("bundle: send applied: %w", err)
	}
	return nil
}
