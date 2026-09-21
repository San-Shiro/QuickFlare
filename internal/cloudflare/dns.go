package cloudflare

import (
	"context"
	"fmt"
	"net/url"
)

// DNSRecord is a DNS record in a zone.
type DNSRecord struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
	Comment string `json:"comment,omitempty"`
}

// cfargotunnelSuffix marks a CNAME as pointing into some Cloudflare tunnel.
const cfargotunnelSuffix = ".cfargotunnel.com"

// RecordComment is written into every DNS record QuickFlare creates.
//
// It is the only thing that makes ownership provable. Pointing at a tunnel is
// not evidence of ownership: a hand-written cloudflared config, another tool,
// or the same domain used from a second machine all produce tunnel records
// that are not ours. Repointing or deleting those would break someone's
// working route, so the marker - not the target - decides.
const RecordComment = "Managed by QuickFlare"

// legacyRecordComments were written by earlier versions under the previous
// project name. Recognised so an upgrade does not orphan its own records.
var legacyRecordComments = []string{"Managed by Keycard"}

// OwnedByUs reports whether a record carries our marker.
func OwnedByUs(rec *DNSRecord) bool {
	if rec == nil {
		return false
	}
	if rec.Comment == RecordComment {
		return true
	}
	for _, legacy := range legacyRecordComments {
		if rec.Comment == legacy {
			return true
		}
	}
	return false
}

// TunnelCNAMETarget is the CNAME content that routes a hostname into a tunnel.
func TunnelCNAMETarget(tunnelID string) string {
	return tunnelID + cfargotunnelSuffix
}

// conflictTypes are the record types that would stop a hostname from
// reaching our tunnel. A CNAME cannot coexist with anything else at the same
// name, and an existing A/AAAA would answer before the tunnel ever did.
var conflictTypes = []string{"CNAME", "A", "AAAA"}

// HostnameConflict reports the record already occupying a hostname, or nil if
// the name is free.
//
// This replaces the old wildcard-availability check. QuickFlare used to claim
// *.<zone> for one tunnel, which came from an earlier design where a local
// proxy did the routing and a single DNS record therefore had to cover every
// future route. Routing moved into the tunnel's own ingress config, so the
// wildcard stopped buying anything - while still taking over every subdomain
// on the zone and colliding with records that were already there. One record
// per route is narrower, survives a zone that is already in use for other
// things, and makes the only question that matters at route-creation time -
// "is this specific name free?" - directly answerable.
func (c *Client) HostnameConflict(ctx context.Context, zoneID, hostname string) (*DNSRecord, error) {
	for _, typ := range conflictTypes {
		rec, err := c.FindDNSRecord(ctx, zoneID, typ, hostname)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			return rec, nil
		}
	}
	return nil, nil
}

// EnsureRouteCNAME points a single hostname at the tunnel, proxied.
//
// It refuses to overwrite a record it did not create: a hostname already
// answering for something else is a collision to report, not something to
// quietly repoint. Re-running it for a hostname already aimed at this tunnel
// is a no-op, so retrying a half-finished route is safe.
func (c *Client) EnsureRouteCNAME(ctx context.Context, zoneID, hostname, tunnelID string) (*DNSRecord, error) {
	target := TunnelCNAMETarget(tunnelID)

	existing, err := c.HostnameConflict(ctx, zoneID, hostname)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Type == "CNAME" && existing.Content == target {
			return existing, nil // already ours, nothing to do
		}

		// Only a record we created gets repointed. The tunnel is found by
		// name, so a rename, a deleted tunnel or a fresh account mints a new
		// id while our own DNS still names the old one - that is worth
		// healing. A tunnel record we did not write is somebody else's
		// working route, and moving it would break them.
		if OwnedByUs(existing) && existing.Type == "CNAME" {
			existing.Content = target
			existing.Proxied = true
			return c.UpdateDNSRecord(ctx, zoneID, *existing)
		}

		return nil, fmt.Errorf("name already used by an existing %s record", existing.Type)
	}

	rec := DNSRecord{
		Type:    "CNAME",
		Name:    hostname,
		Content: target,
		Proxied: true,
		TTL:     1, // 1 means automatic; required when proxied
		Comment: RecordComment,
	}
	created, err := c.CreateDNSRecord(ctx, zoneID, rec)
	if IsAlreadyExists(err) {
		return c.FindDNSRecord(ctx, zoneID, "CNAME", hostname)
	}
	return created, err
}

// CreateDNSRecord adds a record to a zone.
func (c *Client) CreateDNSRecord(ctx context.Context, zoneID string, rec DNSRecord) (*DNSRecord, error) {
	var out DNSRecord
	path := fmt.Sprintf("/zones/%s/dns_records", zoneID)
	if err := c.do(ctx, "POST", path, rec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateDNSRecord overwrites a record.
func (c *Client) UpdateDNSRecord(ctx context.Context, zoneID string, rec DNSRecord) (*DNSRecord, error) {
	if rec.ID == "" {
		return nil, fmt.Errorf("cloudflare: dns record id required for update")
	}
	var out DNSRecord
	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, rec.ID)
	if err := c.do(ctx, "PUT", path, rec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteDNSRecord removes a record.
func (c *Client) DeleteDNSRecord(ctx context.Context, zoneID, recordID string) error {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, recordID)
	return c.do(ctx, "DELETE", path, nil, nil)
}

// FindDNSRecord returns the first record of a type and name, or nil.
func (c *Client) FindDNSRecord(ctx context.Context, zoneID, typ, name string) (*DNSRecord, error) {
	path := fmt.Sprintf("/zones/%s/dns_records?type=%s&name=%s",
		zoneID, url.QueryEscape(typ), url.QueryEscape(name))
	var out []DNSRecord
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &out[0], nil
}
