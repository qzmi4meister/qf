package policy

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// ErrInvalidSignature is returned by VerifyBundle when the signature does
// not match the bundle contents.
var ErrInvalidSignature = errors.New("bundle signature invalid")

// ErrNoSignature is returned by VerifyBundle when the bundle has no
// signature field set.
var ErrNoSignature = errors.New("bundle has no signature")

// SaveBundle serializes bundle to path using proto wire format, writing
// atomically via a temp file in the same directory.
func SaveBundle(path string, bundle *qfv1.PolicyBundle) error {
	b, err := proto.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("marshal bundle: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".policy-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write bundle: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename bundle file: %w", err)
	}
	return nil
}

// LoadBundle reads and deserializes a bundle from path.
// Returns os.ErrNotExist when the file does not exist.
func LoadBundle(path string) (*qfv1.PolicyBundle, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bundle file: %w", err)
	}
	var bundle qfv1.PolicyBundle
	if err := proto.Unmarshal(b, &bundle); err != nil {
		return nil, fmt.Errorf("unmarshal bundle: %w", err)
	}
	return &bundle, nil
}

// SignBundle signs bundle with privKey, setting the Signature and SignedAt
// fields in place.  The signature covers the canonical serialization of the
// bundle with Signature and SignedAt cleared (see canonicalBytes).
func SignBundle(bundle *qfv1.PolicyBundle, privKey ed25519.PrivateKey) error {
	bundle.SignedAt = timestamppb.New(time.Now().UTC())
	bundle.Signature = nil

	msg, err := canonicalBytes(bundle)
	if err != nil {
		return fmt.Errorf("canonical bytes: %w", err)
	}
	bundle.Signature = ed25519.Sign(privKey, msg)
	return nil
}

// VerifyBundle verifies the Ed25519 signature on bundle against pubKey.
// Returns ErrNoSignature when bundle.Signature is empty, ErrInvalidSignature
// on a bad signature, or a wrapped error on serialization failure.
func VerifyBundle(bundle *qfv1.PolicyBundle, pubKey ed25519.PublicKey) error {
	if len(bundle.Signature) == 0 {
		return ErrNoSignature
	}
	// Guard against nil/wrong-length key — ed25519.Verify panics on invalid key size.
	if len(pubKey) != ed25519.PublicKeySize {
		return ErrInvalidSignature
	}
	sig := bundle.Signature

	// Compute canonical bytes: same as what was signed (Signature cleared).
	cp := proto.Clone(bundle).(*qfv1.PolicyBundle)
	cp.Signature = nil
	msg, err := canonicalBytes(cp)
	if err != nil {
		return fmt.Errorf("canonical bytes: %w", err)
	}
	if !ed25519.Verify(pubKey, msg, sig) {
		return ErrInvalidSignature
	}
	return nil
}

// canonicalBytes returns a deterministic protobuf serialization of bundle
// with the Signature field cleared.  Used for both signing and verification.
func canonicalBytes(bundle *qfv1.PolicyBundle) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(bundle)
}
