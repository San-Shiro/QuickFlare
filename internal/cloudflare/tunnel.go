package cloudflare

import (
	"context"
	"fmt"
	"strings"
)

// Tunnel is a cloudflared tunnel.
type Tunnel struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Status      string       `json:"status"`
	CreatedAt   string       `json:"created_at"`
	DeletedAt   *string      `json:"deleted_at"`
	Connections []Connection `json:"connections"`
}

// Connection is one cloudflared connector attached to a tunnel.
type Connection struct {
	ID           string `json:"id"`
	ColoName     string `json:"colo_name"`
	IsPendingRec bool   `json:"is_pending_reconnect"`
	OpenedAt     string `json:"opened_at"`
	OriginIP     string `json:"origin_ip"`
}

// IngressRule maps a public hostname to a local service. A rule with an empty
// Hostname is the catch-all and must be last.
type IngressRule struct {
	Hostname      string         `json:"hostname,omitempty"`
	Path          string         `json:"path,omitempty"`
	Service       string         `json:"service"`
	OriginRequest *OriginRequest `json:"originRequest,omitempty"`
}

// OriginRequest holds per-rule origin settings. Only the fields this app sets
// are modelled; the rest round-trip untouched because we always read before
// we write.
type OriginRequest struct {
	ConnectTimeout int    `json:"connectTimeout,omitempty"`
	NoTLSVerify    bool   `json:"noTLSVerify,omitempty"`
	HTTPHostHeader string `json:"httpHostHeader,omitempty"`
}

// TunnelConfigDoc is the routing document for a remotely-managed tunnel.
type TunnelConfigDoc struct {
	Ingress       []IngressRule  `json:"ingress"`
	OriginRequest *OriginRequest `json:"originRequest,omitempty"`
	WARPRouting   map[string]any `json:"warp-routing,omitempty"`
}

// tunnelConfigEnvelope is what the configurations endpoint returns.
type tunnelConfigEnvelope struct {
	TunnelID string          `json:"tunnel_id"`
	Version  int             `json:"version"`
	Config   TunnelConfigDoc `json:"config"`
}

// CatchAllService is the service the final ingress rule points at. cloudflared
// refuses a configuration whose last rule is hostname-scoped, so one of these
// is always appended.
const CatchAllService = "http_status:404"

// CreateTunnel creates a remotely-managed tunnel. config_src "cloudflare" is
// what makes the ingress document live in the API rather than in a local
// config file - without it, none of the routing calls below apply.
func (c *Client) CreateTunnel(ctx context.Context, name string) (*Tunnel, error) {
	if err := c.requireAccount(); err != nil {
		return nil, err
	}
	body := map[string]string{
		"name":       name,
		"config_src": "cloudflare",
	}
	var out Tunnel
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel", c.accountID)
	if err := c.do(ctx, "POST", path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Tunnels lists tunnels that have not been deleted.
func (c *Client) Tunnels(ctx context.Context) ([]Tunnel, error) {
	if err := c.requireAccount(); err != nil {
		return nil, err
	}
	var all []Tunnel
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel?is_deleted=false", c.accountID)
	if err := c.do(ctx, "GET", path, nil, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// DeleteTunnel removes a tunnel.
func (c *Client) DeleteTunnel(ctx context.Context, tunnelID string) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s", c.accountID, tunnelID)
	return c.do(ctx, "DELETE", path, nil, nil)
}

// TunnelToken returns the connector token, which is what the cloudflared child
// process is launched with.
func (c *Client) TunnelToken(ctx context.Context, tunnelID string) (string, error) {
	if err := c.requireAccount(); err != nil {
		return "", err
	}
	var token string
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/token", c.accountID, tunnelID)
	if err := c.do(ctx, "GET", path, nil, &token); err != nil {
		return "", err
	}
	return token, nil
}

// TunnelConfig reads the current routing document.
func (c *Client) TunnelConfig(ctx context.Context, tunnelID string) (*TunnelConfigDoc, error) {
	if err := c.requireAccount(); err != nil {
		return nil, err
	}
	var out tunnelConfigEnvelope
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", c.accountID, tunnelID)
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return &out.Config, nil
}

// PutTunnelConfig writes the routing document.
//
// This endpoint REPLACES the whole document - there is no per-rule update
// (cloudflared#1437). Callers should almost always use UpdateIngress, which
// reads first, so a write cannot silently drop rules it never knew about.
func (c *Client) PutTunnelConfig(ctx context.Context, tunnelID string, doc TunnelConfigDoc) error {
	if err := c.requireAccount(); err != nil {
		return err
	}
	doc.Ingress = normalizeIngress(doc.Ingress)
	body := map[string]any{"config": doc}
	path := fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", c.accountID, tunnelID)
	return c.do(ctx, "PUT", path, body, nil)
}

// UpdateIngress applies a read-modify-write to the ingress list.
//
// mutate receives the current rules with the catch-all already stripped, and
// returns the rules it wants; the catch-all is re-appended on the way out.
//
// Caveat: the API offers no compare-and-swap, so a concurrent editor (a second
// instance, or somebody in the dashboard) can lose writes. For a single
// desktop owner that is acceptable; it would not be for a shared deployment.
func (c *Client) UpdateIngress(ctx context.Context, tunnelID string, mutate func([]IngressRule) []IngressRule) error {
	doc, err := c.TunnelConfig(ctx, tunnelID)
	if err != nil {
		return fmt.Errorf("read tunnel config: %w", err)
	}
	doc.Ingress = mutate(stripCatchAll(doc.Ingress))
	return c.PutTunnelConfig(ctx, tunnelID, *doc)
}

// AddRoute points a hostname at a local service, replacing any existing rule
// for the same hostname so repeated adds are idempotent.
func (c *Client) AddRoute(ctx context.Context, tunnelID, hostname, service string) error {
	return c.UpdateIngress(ctx, tunnelID, func(rules []IngressRule) []IngressRule {
		out := make([]IngressRule, 0, len(rules)+1)
		for _, r := range rules {
			if !strings.EqualFold(r.Hostname, hostname) {
				out = append(out, r)
			}
		}
		return append(out, IngressRule{Hostname: hostname, Service: service})
	})
}

// RemoveRoute drops every rule for a hostname.
func (c *Client) RemoveRoute(ctx context.Context, tunnelID, hostname string) error {
	return c.UpdateIngress(ctx, tunnelID, func(rules []IngressRule) []IngressRule {
		out := make([]IngressRule, 0, len(rules))
		for _, r := range rules {
			if !strings.EqualFold(r.Hostname, hostname) {
				out = append(out, r)
			}
		}
		return out
	})
}

// stripCatchAll removes trailing hostname-less rules so callers only ever see
// real routes.
func stripCatchAll(rules []IngressRule) []IngressRule {
	out := make([]IngressRule, 0, len(rules))
	for _, r := range rules {
		if r.Hostname == "" && r.Path == "" {
			continue
		}
		out = append(out, r)
	}
	return out
}

// normalizeIngress guarantees exactly one catch-all, last. A document without
// it is rejected by the API.
func normalizeIngress(rules []IngressRule) []IngressRule {
	out := stripCatchAll(rules)
	return append(out, IngressRule{Service: CatchAllService})
}
