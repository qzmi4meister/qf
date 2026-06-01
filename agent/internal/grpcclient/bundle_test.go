package grpcclient

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/qf/qf/agent/internal/policy"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc/metadata"
)

// mockAgentStream captures sent AgentMessages and implements AgentService_StreamClient.
type mockAgentStream struct {
	sent []*qfv1.AgentMessage
}

func (m *mockAgentStream) Send(msg *qfv1.AgentMessage) error          { m.sent = append(m.sent, msg); return nil }
func (m *mockAgentStream) Recv() (*qfv1.ServerMessage, error)         { return nil, nil }
func (m *mockAgentStream) CloseSend() error                           { return nil }
func (m *mockAgentStream) Context() context.Context                   { return context.Background() }
func (m *mockAgentStream) Header() (metadata.MD, error)               { return nil, nil }
func (m *mockAgentStream) Trailer() metadata.MD                       { return nil }
func (m *mockAgentStream) SendMsg(v any) error                        { return nil }
func (m *mockAgentStream) RecvMsg(v any) error                        { return nil }

func genKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	return pub, priv
}

func signedBundle(t *testing.T, gen int64, priv ed25519.PrivateKey) *qfv1.PolicyBundle {
	t.Helper()
	b := &qfv1.PolicyBundle{Generation: gen}
	if err := policy.SignBundle(b, priv); err != nil {
		t.Fatalf("SignBundle: %v", err)
	}
	return b
}

// TestHandleBundle_ValidSignature verifies the golden path: correctly-signed bundle
// is verified, applyFn is called, BundleApplied{Success:true} is sent.
func TestHandleBundle_ValidSignature(t *testing.T) {
	pub, priv := genKey(t)
	stream := &mockAgentStream{}
	bundle := signedBundle(t, 10, priv)

	applied := false
	err := HandleBundle(stream, bundle, pub, func(b *qfv1.PolicyBundle) (uint32, error) {
		applied = true
		return 5, nil
	})
	if err != nil {
		t.Fatalf("HandleBundle: %v", err)
	}
	if !applied {
		t.Error("applyFn must be called for valid signature")
	}
	assertBundleAck(t, stream, true)
	assertBundleApplied(t, stream, true)
}

// TestHandleBundle_TamperedPayload verifies that a bundle whose payload was
// modified after signing is rejected: applyFn is NOT called, BundleAck has
// SignatureVerified=false, BundleApplied has Success=false.
func TestHandleBundle_TamperedPayload(t *testing.T) {
	pub, priv := genKey(t)
	stream := &mockAgentStream{}
	bundle := signedBundle(t, 10, priv)

	// Tamper: change generation after signing.
	bundle.Generation = 999

	applied := false
	err := HandleBundle(stream, bundle, pub, func(b *qfv1.PolicyBundle) (uint32, error) {
		applied = true
		return 0, nil
	})
	if err != nil {
		t.Fatalf("HandleBundle returned unexpected error: %v", err)
	}
	if applied {
		t.Error("applyFn must NOT be called for tampered bundle")
	}
	assertBundleAck(t, stream, false)
	assertBundleApplied(t, stream, false)
}

// TestHandleBundle_WrongKey verifies that a bundle signed with key A is rejected
// when verified with key B.
func TestHandleBundle_WrongKey(t *testing.T) {
	_, priv := genKey(t)
	pubB, _ := genKey(t) // different key
	stream := &mockAgentStream{}
	bundle := signedBundle(t, 10, priv)

	applied := false
	err := HandleBundle(stream, bundle, pubB, func(b *qfv1.PolicyBundle) (uint32, error) {
		applied = true
		return 0, nil
	})
	if err != nil {
		t.Fatalf("HandleBundle returned unexpected error: %v", err)
	}
	if applied {
		t.Error("applyFn must NOT be called when key mismatch")
	}
	assertBundleAck(t, stream, false)
}

// TestHandleBundle_NoSignature verifies that an unsigned bundle is rejected.
func TestHandleBundle_NoSignature(t *testing.T) {
	pub, _ := genKey(t)
	stream := &mockAgentStream{}
	bundle := &qfv1.PolicyBundle{Generation: 5} // no signature

	applied := false
	err := HandleBundle(stream, bundle, pub, func(b *qfv1.PolicyBundle) (uint32, error) {
		applied = true
		return 0, nil
	})
	if err != nil {
		t.Fatalf("HandleBundle returned unexpected error: %v", err)
	}
	if applied {
		t.Error("applyFn must NOT be called for unsigned bundle")
	}
	// ErrNoSignature does not set ErrorMessage in ack (see HandleBundle logic)
	assertBundleAck(t, stream, false)
	assertBundleApplied(t, stream, false)
}

// TestHandleBundle_ApplyError verifies that an apply failure sends BundleApplied
// with Success=false.
func TestHandleBundle_ApplyError(t *testing.T) {
	pub, priv := genKey(t)
	stream := &mockAgentStream{}
	bundle := signedBundle(t, 10, priv)

	err := HandleBundle(stream, bundle, pub, func(b *qfv1.PolicyBundle) (uint32, error) {
		return 0, errors.New("bpf load failed")
	})
	if err != nil {
		t.Fatalf("HandleBundle: %v", err)
	}
	assertBundleAck(t, stream, true)
	assertBundleApplied(t, stream, false)
}

func assertBundleAck(t *testing.T, stream *mockAgentStream, wantVerified bool) {
	t.Helper()
	if len(stream.sent) == 0 {
		t.Fatal("no messages sent on stream")
	}
	ack := stream.sent[0].GetBundleAck()
	if ack == nil {
		t.Fatal("first sent message is not BundleAck")
	}
	if ack.SignatureVerified != wantVerified {
		t.Errorf("BundleAck.SignatureVerified: want %v, got %v", wantVerified, ack.SignatureVerified)
	}
}

func assertBundleApplied(t *testing.T, stream *mockAgentStream, wantSuccess bool) {
	t.Helper()
	if len(stream.sent) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(stream.sent))
	}
	applied := stream.sent[1].GetBundleApplied()
	if applied == nil {
		t.Fatal("second sent message is not BundleApplied")
	}
	if applied.Success != wantSuccess {
		t.Errorf("BundleApplied.Success: want %v, got %v (err: %s)", wantSuccess, applied.Success, applied.ErrorMessage)
	}
}
