package core

import (
	"context"
	"fmt"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
)

// Provisioning: turning an intended route into something that actually
// answers on the internet.
//
// The order is not interchangeable, and getting it wrong is why a route can
// look configured while being unreachable:
//
//  1. A tunnel must exist. It is the thing DNS points at, so nothing before
//     this step has an address to use.
//  2. DNS must name the tunnel. <host> CNAME <tunnel>.cfargotunnel.com.
//  3. The tunnel's ingress must map that hostname to a local port, or
//     requests arrive and get the catch-all 404.

// FindOrCreateTunnel adopts an existing QuickFlare tunnel if there is one.
//
// Creating unconditionally would leave a fresh tunnel on every launch, each
// with its own id - and every DNS record written by a previous run would then
// point at a tunnel nothing is running.
func FindOrCreateTunnel(ctx context.Context, client *cloudflare.Client) (*cloudflare.Tunnel, error) {
	existing, err := client.Tunnels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	for i := range existing {
		if existing[i].Name == TunnelName && existing[i].DeletedAt == nil {
			return &existing[i], nil
		}
	}
	return client.CreateTunnel(ctx, TunnelName)
}

// Publish points a hostname at the tunnel and maps it to a local port.
//
// DNS goes first: the ingress rule is meaningless until the hostname resolves
// to this tunnel, and a conflict there is the one failure worth stopping on
// rather than working around.
//
// Safe to run again over a route that already exists - each step adopts what
// is already there - which is what makes it usable as a repair.
func Publish(ctx context.Context, client *cloudflare.Client, zoneID, tunnelID, hostname, target string) error {
	if client == nil {
		return fmt.Errorf("not connected to Cloudflare")
	}
	if tunnelID == "" {
		return fmt.Errorf("no tunnel yet")
	}
	if zoneID == "" {
		return fmt.Errorf("no zone selected")
	}

	if _, err := client.EnsureRouteCNAME(ctx, zoneID, hostname, tunnelID); err != nil {
		return err
	}
	if err := client.AddRoute(ctx, tunnelID, hostname, "http://"+target); err != nil {
		return fmt.Errorf("ingress: %w", err)
	}
	return nil
}

// Unpublish removes everything publishing a hostname, reporting whatever it
// could not clean up rather than stopping at the first problem.
//
// DNS goes first, and deliberately not in the reverse order of Publish.
// Teardown can fail partway through, and the moment the record is gone the
// hostname stops resolving - so anything left behind after that is tidying
// rather than exposure.
//
// The returned strings are leftovers the user may need to clear by hand. An
// empty slice means everything went.
func Unpublish(ctx context.Context, client *cloudflare.Client, zoneID, tunnelID, hostname string) []string {
	if client == nil {
		return []string{"not connected - removed locally only"}
	}

	var problems []string

	// 1. DNS - stops the hostname resolving.
	if zoneID != "" {
		rec, err := client.FindDNSRecord(ctx, zoneID, "CNAME", hostname)
		switch {
		case err != nil:
			problems = append(problems, "DNS lookup: "+err.Error())
		case rec == nil:
			// Already gone; nothing to do.
		case !cloudflare.OwnedByUs(rec):
			// Adopted into the list rather than created by us - most likely a
			// record that was already serving this name before QuickFlare saw
			// it. Removing the route means we stop managing it, not that we
			// get to delete somebody else's DNS.
			problems = append(problems, "left DNS record alone (not created by QuickFlare)")
		default:
			if err := client.DeleteDNSRecord(ctx, zoneID, rec.ID); err != nil {
				problems = append(problems, "DNS: "+err.Error())
			}
		}
	}

	// 2. Ingress - the tunnel stops claiming the hostname.
	if tunnelID != "" {
		if err := client.RemoveRoute(ctx, tunnelID, hostname); err != nil {
			problems = append(problems, "ingress: "+err.Error())
		}
	}

	return problems
}
