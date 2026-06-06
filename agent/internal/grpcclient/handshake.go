package grpcclient

import (
	"fmt"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// HandshakeResult holds the Welcome and an optional initial PolicyBundle
// that CP pushed because the agent was behind.
type HandshakeResult struct {
	Welcome *qfv1.Welcome
	// Bundle is non-nil when CP pushed an initial bundle after Welcome
	// (hello.current_generation < welcome.server_generation).
	Bundle *qfv1.PolicyBundle
}

// Handshake sends Hello, receives Welcome, and — if the agent is behind —
// receives the immediately-following PolicyBundle from CP.
// Caller is responsible for applying Bundle if non-nil.
func Handshake(stream qfv1.AgentService_StreamClient, hello *qfv1.Hello) (*HandshakeResult, error) {
	if err := stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_Hello{Hello: hello},
	}); err != nil {
		return nil, fmt.Errorf("handshake: send hello: %w", err)
	}

	msg, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("handshake: recv welcome: %w", err)
	}
	welcome := msg.GetWelcome()
	if welcome == nil {
		return nil, fmt.Errorf("handshake: expected Welcome, got %T", msg.Payload)
	}
	if !welcome.Accepted {
		return nil, fmt.Errorf("handshake: rejected by CP: %s", welcome.RejectReason)
	}

	res := &HandshakeResult{Welcome: welcome}

	// CP sends PolicyBundle immediately after Welcome when agent is behind.
	if hello.CurrentGeneration < welcome.ServerGeneration {
		bundleMsg, err := stream.Recv()
		if err != nil {
			return nil, fmt.Errorf("handshake: recv initial bundle: %w", err)
		}
		switch p := bundleMsg.Payload.(type) {
		case *qfv1.ServerMessage_PolicyBundle:
			res.Bundle = p.PolicyBundle
		case *qfv1.ServerMessage_DisconnectRequest:
			return nil, status.Errorf(codes.Unavailable, "CP disconnected during handshake: %s", p.DisconnectRequest.Reason)
		default:
			return nil, fmt.Errorf("handshake: expected PolicyBundle, got %T", bundleMsg.Payload)
		}
	}

	return res, nil
}
