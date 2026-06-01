// Package forwarder routes log events to external systems (syslog, etc.).
// When QF_FORWARDER_DSN is set, the ingester calls Forward instead of writing
// log_events to PostgreSQL. Audit log is always written to PostgreSQL.
package forwarder

import "time"

// LogEvent is a flat, normalised log event ready for external forwarding.
type LogEvent struct {
	TenantID  string
	HostID    string
	RuleID    string
	PolicyID  string
	Direction string // "ingress" | "egress"
	Action    string // "allow" | "deny" | "log"
	Protocol  int16
	SrcIP     string
	SrcPort   int32
	DstIP     string
	DstPort   int32
	Ts        time.Time
	CtState   string
}

// Forwarder sends log events to an external sink.
// Implementations must be safe for concurrent use.
type Forwarder interface {
	Forward(events []LogEvent) error
	Close() error
}
