package grpcclient

import (
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// System-event type constants.
const (
	EventAgentStarted      = "agent_started"
	EventCPConnected       = "cp_connected"
	EventCPDisconnected    = "cp_disconnected"
	EventBundleApplied     = "bundle_applied"
	EventBundleCacheLoaded = "bundle_cache_loaded"
	EventAttachFailed      = "attach_failed"
	EventCertRotated       = "cert_rotated"
)

// SendSystemEvent sends a SystemEvent message on the stream.
// attrs may be nil.
func SendSystemEvent(
	stream qfv1.AgentService_StreamClient,
	evType string,
	severity qfv1.Severity,
	detail string,
	attrs map[string]string,
) error {
	return stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_SystemEvent{
			SystemEvent: &qfv1.SystemEvent{
				Ts:         timestamppb.Now(),
				Type:       evType,
				Severity:   severity,
				Detail:     detail,
				Attributes: attrs,
			},
		},
	})
}
