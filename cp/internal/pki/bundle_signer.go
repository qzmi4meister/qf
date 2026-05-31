package pki

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BundleSigner holds the Ed25519 keypair used to sign policy bundles.
type BundleSigner struct {
	PublicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

// LoadOrInitBundleSigner loads the Ed25519 keypair from dir if files exist;
// otherwise generates a new keypair and saves it.
// Files: {dir}/bundle-signing.key (PKCS8 PEM) and {dir}/bundle-signing.pub (PKIX PEM).
func LoadOrInitBundleSigner(dir string) (*BundleSigner, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "bundle-signing.key")
	pubPath := filepath.Join(dir, "bundle-signing.pub")

	if _, err := os.Stat(keyPath); err == nil {
		return loadBundleSigner(keyPath, pubPath)
	}
	return generateBundleSigner(keyPath, pubPath)
}

func loadBundleSigner(keyPath, pubPath string) (*BundleSigner, error) {
	keyPEMBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyPEMBytes)
	if block == nil {
		return nil, fmt.Errorf("pki: invalid PEM in %s", keyPath)
	}
	priv, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ed, ok := priv.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("pki: bundle-signing.key is not Ed25519")
	}
	return &BundleSigner{
		PublicKey:  ed.Public().(ed25519.PublicKey),
		privateKey: ed,
	}, nil
}

func generateBundleSigner(keyPath, pubPath string) (*BundleSigner, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, err
	}

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if err := os.WriteFile(pubPath, pubPEM, 0644); err != nil {
		return nil, err
	}

	return &BundleSigner{PublicKey: pub, privateKey: priv}, nil
}

// SignBundle signs bundle in-place: clears Signature, deterministic-marshals,
// Ed25519-signs, sets bundle.Signature and bundle.SignedAt.
func (bs *BundleSigner) SignBundle(bundle *qfv1.PolicyBundle) error {
	bundle.Signature = nil
	bundle.SignedAt = timestamppb.New(time.Now())

	canonical, err := proto.MarshalOptions{Deterministic: true}.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("pki: marshal bundle: %w", err)
	}

	bundle.Signature = ed25519.Sign(bs.privateKey, canonical)
	return nil
}
