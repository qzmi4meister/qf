package ingest

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/forwarder"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func protoLogEventToParams(tenantID, hostID pgtype.UUID, ev *qfv1.LogEvent) storegen.InsertLogEventsBatchParams {
	var ts pgtype.Timestamptz
	if ev.Ts != nil {
		ts = pgtype.Timestamptz{Time: ev.Ts.AsTime(), Valid: true}
	} else {
		ts = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	p := storegen.InsertLogEventsBatchParams{
		TenantID:   tenantID,
		HostID:     hostID,
		RuleID:     parseUUID(ev.RuleId),
		PolicyID:   parseUUID(ev.PolicyId),
		Direction:  protoDirectionStr(ev.Direction),
		Action:     protoActionStr(ev.Action),
		Protocol:   int16(ev.Protocol),
		SrcIp:      bytesToAddr(ev.SrcIp),
		DstIp:      bytesToAddr(ev.DstIp),
		CreatedAt:  ts,
	}
	if ev.SrcPort > 0 {
		v := int32(ev.SrcPort)
		p.SrcPort = &v
	}
	if ev.DstPort > 0 {
		v := int32(ev.DstPort)
		p.DstPort = &v
	}
	if ev.PacketSize > 0 {
		v := int32(ev.PacketSize)
		p.PacketSize = &v
	}
	if ev.TcpFlags > 0 {
		v := int32(ev.TcpFlags)
		p.TcpFlags = &v
	}
	if s := protoConntrackStateStr(ev.ConntrackState); s != "" {
		p.CtState = &s
	}
	return p
}

func protoFlowEventToParams(tenantID, hostID pgtype.UUID, fv *qfv1.FlowEvent) storegen.InsertFlowEventsBatchParams {
	p := storegen.InsertFlowEventsBatchParams{
		TenantID:     tenantID,
		HostID:       hostID,
		Protocol:     int16(fv.Protocol),
		SrcIp:        bytesToAddr(fv.SrcIp),
		DstIp:        bytesToAddr(fv.DstIp),
		BytesOrig:    int64(fv.BytesOrig),
		BytesReply:   int64(fv.BytesReply),
		PacketsOrig:  int64(fv.PacketsOrig),
		PacketsReply: int64(fv.PacketsReply),
		CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	if fv.SrcPort > 0 {
		v := int32(fv.SrcPort)
		p.SrcPort = &v
	}
	if fv.DstPort > 0 {
		v := int32(fv.DstPort)
		p.DstPort = &v
	}
	if fv.FinalState != "" {
		p.FinalState = &fv.FinalState
	}
	if fv.TsStart != nil {
		p.StartedAt = pgtype.Timestamptz{Time: fv.TsStart.AsTime(), Valid: true}
	}
	if fv.TsEnd != nil {
		p.EndedAt = pgtype.Timestamptz{Time: fv.TsEnd.AsTime(), Valid: true}
	}
	return p
}

func protoSystemEventToParams(tenantID, hostID pgtype.UUID, ev *qfv1.SystemEvent) storegen.InsertSystemEventParams {
	attrs := []byte("{}")
	if len(ev.Attributes) > 0 {
		// encode map[string]string as JSON manually to avoid import cycle
		var sb strings.Builder
		sb.WriteByte('{')
		first := true
		for k, v := range ev.Attributes {
			if !first {
				sb.WriteByte(',')
			}
			first = false
			sb.WriteString(`"` + jsonEscapeSimple(k) + `":"` + jsonEscapeSimple(v) + `"`)
		}
		sb.WriteByte('}')
		attrs = []byte(sb.String())
	}
	return storegen.InsertSystemEventParams{
		TenantID:   tenantID,
		HostID:     hostID,
		Type:       ev.Type,
		Severity:   protoSeverityStr(ev.Severity),
		Detail:     ev.Detail,
		Attributes: attrs,
	}
}

// protoToForwarderEvent converts a proto LogEvent to a forwarder.LogEvent.
func protoToForwarderEvent(tenantID, hostID pgtype.UUID, ev *qfv1.LogEvent) forwarder.LogEvent {
	ts := time.Now()
	if ev.Ts != nil {
		ts = ev.Ts.AsTime()
	}
	fe := forwarder.LogEvent{
		TenantID:  uuidToString(tenantID),
		HostID:    uuidToString(hostID),
		RuleID:    ev.RuleId,
		PolicyID:  ev.PolicyId,
		Direction: protoDirectionStr(ev.Direction),
		Action:    protoActionStr(ev.Action),
		Protocol:  int16(ev.Protocol),
		SrcPort:   int32(ev.SrcPort),
		DstPort:   int32(ev.DstPort),
		Ts:        ts,
		CtState:   protoConntrackStateStr(ev.ConntrackState),
	}
	if addr := bytesToAddr(ev.SrcIp); addr != nil {
		fe.SrcIP = addr.String()
	}
	if addr := bytesToAddr(ev.DstIp); addr != nil {
		fe.DstIP = addr.String()
	}
	return fe
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ── conversion helpers ────────────────────────────────────────────────────

func bytesToAddr(b []byte) *netip.Addr {
	switch len(b) {
	case 4:
		a := netip.AddrFrom4([4]byte(b))
		return &a
	case 16:
		a := netip.AddrFrom16([16]byte(b))
		return &a
	}
	return nil
}

func uuidBytesToStr(b [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func protoTsToTimestamptz(ts *timestamppb.Timestamp) pgtype.Timestamptz {
	if ts == nil {
		return pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	return pgtype.Timestamptz{Time: ts.AsTime(), Valid: true}
}

func protoDirectionStr(d qfv1.Direction) string {
	switch d {
	case qfv1.Direction_DIRECTION_INGRESS:
		return "ingress"
	case qfv1.Direction_DIRECTION_EGRESS:
		return "egress"
	default:
		return "ingress"
	}
}

func protoActionStr(a qfv1.Action) string {
	switch a {
	case qfv1.Action_ACTION_ALLOW:
		return "allow"
	case qfv1.Action_ACTION_DENY:
		return "deny"
	case qfv1.Action_ACTION_LOG:
		return "log"
	default:
		return "allow"
	}
}

func protoConntrackStateStr(s qfv1.ConntrackState) string {
	switch s {
	case qfv1.ConntrackState_CONNTRACK_STATE_NEW:
		return "new"
	case qfv1.ConntrackState_CONNTRACK_STATE_ESTABLISHED:
		return "established"
	case qfv1.ConntrackState_CONNTRACK_STATE_RELATED:
		return "related"
	case qfv1.ConntrackState_CONNTRACK_STATE_INVALID:
		return "invalid"
	default:
		return ""
	}
}

func protoSeverityStr(s qfv1.Severity) string {
	switch s {
	case qfv1.Severity_SEVERITY_WARNING:
		return "warning"
	case qfv1.Severity_SEVERITY_ERROR:
		return "error"
	default:
		return "info"
	}
}

// jsonEscapeSimple escapes only the characters required by JSON strings
// for use in simple map key/value encoding where values come from
// internal agent attributes (no arbitrary user input).
func jsonEscapeSimple(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
