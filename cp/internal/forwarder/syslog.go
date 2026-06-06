package forwarder

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	syslogAppName    = "qf"
	syslogEnterprise = "32473" // IANA example PEN for private structured data
)

// Open creates a Forwarder from a DSN string.
//
// Supported schemes:
//
//	syslog://host:port         TCP (default)
//	syslog+udp://host:port     UDP (no framing, max ~1 KB message)
//
// Query parameters:
//
//	facility=local0   syslog facility (kern/user/daemon/local0..local7); default local0
func Open(dsn string) (Forwarder, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, fmt.Errorf("forwarder: parse DSN: %w", err)
	}

	var proto string
	switch u.Scheme {
	case "syslog":
		proto = "tcp"
	case "syslog+udp":
		proto = "udp"
	default:
		return nil, fmt.Errorf("forwarder: unknown scheme %q (use syslog:// or syslog+udp://)", u.Scheme)
	}

	addr := u.Host
	if addr == "" {
		return nil, fmt.Errorf("forwarder: missing host in DSN")
	}

	facility := parseFacility(u.Query().Get("facility"))

	sf := &syslogForwarder{addr: addr, proto: proto, facility: facility}

	// Pre-connect for TCP to surface config errors at startup.
	if proto == "tcp" {
		if err := sf.dial(); err != nil {
			return nil, fmt.Errorf("forwarder: initial connect to %s: %w", addr, err)
		}
	}
	return sf, nil
}

type syslogForwarder struct {
	addr     string
	proto    string
	facility int

	mu   sync.Mutex
	conn net.Conn
}

func (s *syslogForwarder) Forward(events []LogEvent) error {
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ev := range events {
		line := formatRFC5424(ev, s.facility) + "\n"
		if err := s.write([]byte(line)); err != nil {
			return err
		}
	}
	return nil
}

func (s *syslogForwarder) write(data []byte) error {
	if s.proto == "udp" {
		conn, err := net.DialTimeout("udp", s.addr, 5*time.Second)
		if err != nil {
			return err
		}
		defer conn.Close()
		_, err = conn.Write(data)
		return err
	}

	// TCP: try once, reconnect on error, retry once.
	if s.conn == nil {
		if err := s.dial(); err != nil {
			return err
		}
	}
	if _, err := s.conn.Write(data); err != nil {
		s.conn.Close()
		s.conn = nil
		if err2 := s.dial(); err2 != nil {
			return err2
		}
		_, err = s.conn.Write(data)
		return err
	}
	return nil
}

func (s *syslogForwarder) dial() error {
	conn, err := net.DialTimeout("tcp", s.addr, 5*time.Second)
	if err != nil {
		return err
	}
	s.conn = conn
	return nil
}

func (s *syslogForwarder) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}

// ── RFC5424 formatting ─────────────────────────────────────────────────────

func formatRFC5424(ev LogEvent, facility int) string {
	severity := 6 // informational
	if ev.Action == "deny" {
		severity = 3 // error
	}
	pri := (facility * 8) + severity

	ts := ev.Ts
	if ts.IsZero() {
		ts = time.Now()
	}

	hostname := nilval(ev.HostID)
	procID := nilval(ev.RuleID)
	msgID := nilval(ev.Action)

	sd := buildSD(ev)
	msg := fmt.Sprintf("%s src=%s:%d dst=%s:%d proto=%d",
		ev.Action, ev.SrcIP, ev.SrcPort, ev.DstIP, ev.DstPort, ev.Protocol)

	return fmt.Sprintf("<%d>1 %s %s %s %s %s %s %s",
		pri, ts.UTC().Format(time.RFC3339), hostname,
		syslogAppName, procID, msgID, sd, msg)
}

func buildSD(ev LogEvent) string {
	var sb strings.Builder
	sb.WriteString("[qf@")
	sb.WriteString(syslogEnterprise)
	writeParam(&sb, "direction", ev.Direction)
	writeParam(&sb, "src", ev.SrcIP)
	writeParam(&sb, "src_port", fmt.Sprint(ev.SrcPort))
	writeParam(&sb, "dst", ev.DstIP)
	writeParam(&sb, "dst_port", fmt.Sprint(ev.DstPort))
	writeParam(&sb, "proto", fmt.Sprint(ev.Protocol))
	if ev.RuleID != "" {
		writeParam(&sb, "rule_id", ev.RuleID)
	}
	if ev.TenantID != "" {
		writeParam(&sb, "tenant_id", ev.TenantID)
	}
	if ev.CtState != "" {
		writeParam(&sb, "ct_state", ev.CtState)
	}
	sb.WriteByte(']')
	return sb.String()
}

func writeParam(sb *strings.Builder, k, v string) {
	sb.WriteByte(' ')
	sb.WriteString(k)
	sb.WriteString(`="`)
	sb.WriteString(sdEscape(v))
	sb.WriteByte('"')
}

// sdEscape escapes the three special characters in RFC5424 PARAM-VALUE.
func sdEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `]`, `\]`)
	return s
}

func nilval(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// parseFacility maps syslog facility names to their numeric values.
// Unknown names default to local0 (16).
func parseFacility(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "kern":
		return 0
	case "user":
		return 1
	case "mail":
		return 2
	case "daemon":
		return 3
	case "auth":
		return 4
	case "syslog":
		return 5
	case "lpr":
		return 6
	case "news":
		return 7
	case "uucp":
		return 8
	case "cron":
		return 9
	case "authpriv":
		return 10
	case "ftp":
		return 11
	case "local1":
		return 17
	case "local2":
		return 18
	case "local3":
		return 19
	case "local4":
		return 20
	case "local5":
		return 21
	case "local6":
		return 22
	case "local7":
		return 23
	default:
		return 16 // local0
	}
}
