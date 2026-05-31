package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"github.com/jackc/pgx/v5/pgtype"
)

// ResolvedGroup holds the fully-resolved form of an ObjectGroup.
type ResolvedGroup struct {
	ID   string
	Type string
	// CIDRs is populated for ipset and hostset types.
	CIDRs []*qfv1.CIDR
	// Ports is populated for portset type.
	Ports []*qfv1.PortRange
}

// Resolver resolves ObjectGroup references into concrete CIDRs and PortRanges.
type Resolver struct {
	queries  *storegen.Queries
	selector *SelectorMatcher
}

// NewResolver creates a Resolver.
func NewResolver(queries *storegen.Queries) *Resolver {
	return &Resolver{
		queries:  queries,
		selector: NewSelectorMatcher(queries),
	}
}

// Resolve resolves an object group by ID into its concrete form.
func (r *Resolver) Resolve(ctx context.Context, tenantID, ogID string) (*ResolvedGroup, error) {
	var tenantUUID, ogUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, fmt.Errorf("resolver: invalid tenant_id: %w", err)
	}
	if err := ogUUID.Scan(ogID); err != nil {
		return nil, fmt.Errorf("resolver: invalid og_id: %w", err)
	}

	og, err := r.queries.GetObjectGroup(ctx, storegen.GetObjectGroupParams{
		ID:       ogUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolver: get object_group %s: %w", ogID, err)
	}

	switch og.Type {
	case "ipset":
		return r.resolveIPSet(og)
	case "portset":
		return r.resolvePortSet(og)
	case "hostset":
		return r.resolveHostSet(ctx, tenantID, og)
	default:
		return nil, fmt.Errorf("resolver: unknown object_group type %q", og.Type)
	}
}

// resolveIPSet parses spec.cidrs → []*qfv1.CIDR.
func (r *Resolver) resolveIPSet(og storegen.ObjectGroup) (*ResolvedGroup, error) {
	var spec struct {
		CIDRs []string `json:"cidrs"`
	}
	if err := json.Unmarshal(og.Spec, &spec); err != nil {
		return nil, fmt.Errorf("resolver: ipset spec: %w", err)
	}

	cidrs := make([]*qfv1.CIDR, 0, len(spec.CIDRs))
	for _, cidrStr := range spec.CIDRs {
		c, err := parseCIDR(cidrStr)
		if err != nil {
			return nil, fmt.Errorf("resolver: ipset cidr %q: %w", cidrStr, err)
		}
		cidrs = append(cidrs, c)
	}
	return &ResolvedGroup{
		ID:    pgUUIDToStr(og.ID),
		Type:  "ipset",
		CIDRs: cidrs,
	}, nil
}

// resolvePortSet parses spec.ports → []*qfv1.PortRange.
// Each entry is either "80" (single port) or "8080-8090" (range).
func (r *Resolver) resolvePortSet(og storegen.ObjectGroup) (*ResolvedGroup, error) {
	var spec struct {
		Ports []string `json:"ports"`
	}
	if err := json.Unmarshal(og.Spec, &spec); err != nil {
		return nil, fmt.Errorf("resolver: portset spec: %w", err)
	}

	ports := make([]*qfv1.PortRange, 0, len(spec.Ports))
	for _, p := range spec.Ports {
		pr, err := parsePortRange(p)
		if err != nil {
			return nil, fmt.Errorf("resolver: portset entry %q: %w", p, err)
		}
		ports = append(ports, pr)
	}
	return &ResolvedGroup{
		ID:    pgUUIDToStr(og.ID),
		Type:  "portset",
		Ports: ports,
	}, nil
}

// resolveHostSet resolves spec.selector → matching hosts → /32 (or /128) CIDRs.
// Host IPs come from the "ip" label (e.g. {"ip": "10.0.0.5"}).
func (r *Resolver) resolveHostSet(ctx context.Context, tenantID string, og storegen.ObjectGroup) (*ResolvedGroup, error) {
	var spec struct {
		Selector Selector `json:"selector"`
	}
	if err := json.Unmarshal(og.Spec, &spec); err != nil {
		return nil, fmt.Errorf("resolver: hostset spec: %w", err)
	}

	hosts, err := r.selector.ResolveHosts(ctx, tenantID, spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("resolver: hostset selector: %w", err)
	}

	var cidrs []*qfv1.CIDR
	for _, h := range hosts {
		labels, _ := parseLabels(h.Labels)
		ipStr, ok := labels["ip"]
		if !ok || ipStr == "" {
			continue
		}
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}
		if ip4 := ip.To4(); ip4 != nil {
			cidrs = append(cidrs, &qfv1.CIDR{Ip: ip4, PrefixLen: 32})
		} else {
			cidrs = append(cidrs, &qfv1.CIDR{Ip: ip.To16(), PrefixLen: 128})
		}
	}
	return &ResolvedGroup{
		ID:    pgUUIDToStr(og.ID),
		Type:  "hostset",
		CIDRs: cidrs,
	}, nil
}

func parseCIDR(s string) (*qfv1.CIDR, error) {
	ip, network, err := net.ParseCIDR(s)
	if err != nil {
		return nil, err
	}
	ones, _ := network.Mask.Size()
	if ip4 := ip.To4(); ip4 != nil {
		return &qfv1.CIDR{Ip: ip4, PrefixLen: uint32(ones)}, nil
	}
	return &qfv1.CIDR{Ip: ip.To16(), PrefixLen: uint32(ones)}, nil
}

func parsePortRange(s string) (*qfv1.PortRange, error) {
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		lo, err := strconv.ParseUint(s[:idx], 10, 32)
		if err != nil {
			return nil, err
		}
		hi, err := strconv.ParseUint(s[idx+1:], 10, 32)
		if err != nil {
			return nil, err
		}
		if lo > hi {
			return nil, fmt.Errorf("invalid range: %s", s)
		}
		return &qfv1.PortRange{Start: uint32(lo), End: uint32(hi)}, nil
	}
	p, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return nil, err
	}
	return &qfv1.PortRange{Start: uint32(p), End: uint32(p)}, nil
}
