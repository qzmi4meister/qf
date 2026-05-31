package agentsrv

import (
	"log/slog"
	"sync"

	"github.com/qf/qf/cp/internal/policy"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

type disconnectSignal struct {
	reason           string
	reconnectAfterMs uint32
}

// activeStream wraps a gRPC stream with a send mutex and disconnect channel.
type activeStream struct {
	mu           sync.Mutex
	stream       qfv1.AgentService_StreamServer
	disconnectCh chan disconnectSignal
}

func (a *activeStream) send(msg *qfv1.ServerMessage) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stream.Send(msg)
}

// StreamRegistry tracks active agent streams keyed by hostID.
// Implements policy.Dispatcher.
type StreamRegistry struct {
	mu      sync.RWMutex
	streams map[string]*activeStream
}

// NewStreamRegistry creates a StreamRegistry.
func NewStreamRegistry() *StreamRegistry {
	return &StreamRegistry{streams: make(map[string]*activeStream)}
}

// register adds a stream and returns its disconnect channel.
func (r *StreamRegistry) register(hostID string, s qfv1.AgentService_StreamServer) chan disconnectSignal {
	ch := make(chan disconnectSignal, 1)
	r.mu.Lock()
	r.streams[hostID] = &activeStream{stream: s, disconnectCh: ch}
	r.mu.Unlock()
	return ch
}

func (r *StreamRegistry) deregister(hostID string) {
	r.mu.Lock()
	delete(r.streams, hostID)
	r.mu.Unlock()
}

// Dispatch implements policy.Dispatcher: pushes a bundle to the host's active stream.
func (r *StreamRegistry) Dispatch(update policy.BundleUpdate) {
	r.mu.RLock()
	as, ok := r.streams[update.HostID]
	r.mu.RUnlock()
	if !ok {
		slog.Debug("agentsrv: dispatch skip, no active stream", "host", update.HostID)
		return
	}
	if err := as.send(&qfv1.ServerMessage{
		Payload: &qfv1.ServerMessage_PolicyBundle{
			PolicyBundle: update.Bundle,
		},
	}); err != nil {
		slog.Warn("agentsrv: dispatch send failed", "host", update.HostID, "err", err)
	}
}

// Disconnect signals the host's stream to send DisconnectRequest and close.
func (r *StreamRegistry) Disconnect(hostID, reason string, reconnectAfterMs uint32) {
	r.mu.RLock()
	as, ok := r.streams[hostID]
	r.mu.RUnlock()
	if !ok {
		return
	}
	select {
	case as.disconnectCh <- disconnectSignal{reason: reason, reconnectAfterMs: reconnectAfterMs}:
	default: // already pending
	}
}

// DisconnectAll signals all active streams to disconnect. Used for graceful shutdown.
func (r *StreamRegistry) DisconnectAll(reason string, reconnectAfterMs uint32) {
	r.mu.RLock()
	ids := make([]string, 0, len(r.streams))
	for id := range r.streams {
		ids = append(ids, id)
	}
	r.mu.RUnlock()
	for _, id := range ids {
		r.Disconnect(id, reason, reconnectAfterMs)
	}
}
