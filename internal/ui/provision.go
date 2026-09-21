package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
	"github.com/San-Shiro/QuickFlare/internal/supervisor"
)

// Provisioning: turning a row in the UI into something that actually answers
// on the internet.
//
// The order is not interchangeable, and getting it wrong is why a route can
// look configured while being unreachable:
//
//  1. A tunnel must exist. It is the thing DNS points at, so nothing before
//     this step has an address to use.
//  2. DNS must name the tunnel. <host> CNAME <tunnel>.cfargotunnel.com.
//  3. The tunnel's ingress must map that hostname to a local port, or
//     requests arrive and get the catch-all 404.

// tunnelName is the tunnel QuickFlare creates and reuses. Keeping a stable name
// means a reinstall adopts the existing tunnel rather than littering the
// account with duplicates.
const tunnelName = "quickflare"

// ensureTunnel finds or creates QuickFlare's tunnel and starts the connector.
func (p *Panel) ensureTunnel() {
	client := p.cf
	if client == nil {
		return
	}
	if p.binErr != nil {
		p.setStatus("cloudflared not installed - routes cannot connect", colError)
		return
	}

	// Re-verifying (Settings -> Save) runs this again. Without stopping the
	// previous connector first, every re-verify left another cloudflared
	// running against the same tunnel, accumulating processes that only a
	// reboot would clear.
	if p.tunnel != nil {
		p.tunnel.Stop()
		p.tunnel = nil
	}

	p.setStatus("Setting up tunnel...", colWarning)

	binPath := p.binPath
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tun, err := findOrCreateTunnel(ctx, client)
		if err != nil {
			p.dispatch(func() {
				p.setStatus("Tunnel setup failed: "+err.Error(), colError)
			})
			return
		}

		token, err := client.TunnelToken(ctx, tun.ID)
		if err != nil {
			p.dispatch(func() {
				p.setStatus("Could not fetch tunnel token: "+err.Error(), colError)
			})
			return
		}

		sup := supervisorFor(binPath)
		if err := sup.Start(context.Background(), token); err != nil {
			p.dispatch(func() {
				p.setStatus("Could not start cloudflared: "+err.Error(), colError)
			})
			return
		}

		// Registered before the dispatch lands, so a quit racing tunnel
		// startup still tears the connector down.
		p.addCloser(sup.Stop)

		p.dispatch(func() {
			p.tunnelID = tun.ID
			p.tunnel = sup
			p.setStatus("Tunnel ready", colSuccess)
			// The tunnel is the first point at which Cloudflare's real route
			// state can be read, so this is where the cached list gets
			// checked and anything half-published gets healed.
			p.reconcileRoutes()
		})
	}()
}

// findOrCreateTunnel adopts an existing QuickFlare tunnel if there is one.
//
// Creating unconditionally would leave a fresh tunnel on every launch, each
// with its own id - and every DNS record written by a previous run would then
// point at a tunnel nothing is running.
func findOrCreateTunnel(ctx context.Context, client *cloudflare.Client) (*cloudflare.Tunnel, error) {
	existing, err := client.Tunnels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	for i := range existing {
		if existing[i].Name == tunnelName && existing[i].DeletedAt == nil {
			return &existing[i], nil
		}
	}
	return client.CreateTunnel(ctx, tunnelName)
}

// provisionRoute publishes one route end to end.
func (p *Panel) provisionRoute(hostname, target string) {
	client := p.cf
	if client == nil {
		p.setRouteStatus(hostname, statusError, "not connected to Cloudflare")
		return
	}
	if p.tunnelID == "" {
		// The tunnel is still being set up; ensureTunnel will come back for
		// this route rather than failing it outright.
		p.setRouteStatus(hostname, statusStarting, "waiting for tunnel...")
		return
	}

	zoneID := p.currentZoneID()
	if zoneID == "" {
		p.setRouteStatus(hostname, statusError, "no zone selected")
		return
	}

	tunnelID := p.tunnelID
	p.setRouteStatus(hostname, statusStarting, "publishing...")

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// DNS first: the ingress rule is meaningless until the hostname
		// resolves to this tunnel, and a conflict here is the one failure
		// worth stopping on rather than working around.
		if _, err := client.EnsureRouteCNAME(ctx, zoneID, hostname, tunnelID); err != nil {
			p.dispatch(func() {
				p.setRouteStatus(hostname, statusError, err.Error())
			})
			return
		}

		if err := client.AddRoute(ctx, tunnelID, hostname, "http://"+target); err != nil {
			p.dispatch(func() {
				p.setRouteStatus(hostname, statusError, "ingress: "+err.Error())
			})
			return
		}

		p.dispatch(func() {
			p.setRouteStatus(hostname, statusConnected, "")
		})
	}()
}

// currentZoneID is the Cloudflare zone id for the selected domain.
func (p *Panel) currentZoneID() string {
	if p.domainIx >= 0 && p.domainIx < len(p.zones) {
		return p.zones[p.domainIx].ID
	}
	return ""
}

// supervisorFor builds the cloudflared supervisor. Split out so the
// provisioning flow above stays readable and the supervisor's wiring - log
// pump, status callback - lives in one place.
func supervisorFor(binPath string) *supervisor.Tunnel {
	return supervisor.NewTunnel(binPath)
}

// deprovisionRoute removes everything publishing a hostname.
//
// DNS goes first, because teardown can fail partway through and the failure
// modes are not equally bad. The moment the record is gone the hostname stops
// resolving, so anything still left behind is tidying rather than exposure.
//
// Every step is best-effort past that point: a failure to tidy up is worth
// reporting, but should not stop the remaining cleanup or leave the row
// stuck in the list.
func (p *Panel) deprovisionRoute(r Route) {
	client := p.cf
	if client == nil {
		p.setStatus("Not connected - removed locally only", colWarning)
		return
	}

	hostname, zoneID, tunnelID := r.Hostname, r.ZoneID, p.tunnelID

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

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
				// Adopted into the list rather than created by us - most
				// likely a record that was already serving this name before
				// QuickFlare saw it. Removing the route here means we stop
				// managing it, not that we get to delete somebody else's
				// DNS.
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

		p.dispatch(func() {
			if len(problems) == 0 {
				p.setStatus("Removed "+hostname, colSuccess)
				return
			}
			// Naming what survived matters: these are account-level leftovers
			// the user may need to clear by hand.
			p.setStatus("Removed "+hostname+" - leftovers: "+strings.Join(problems, "; "), colWarning)
		})
	}()
}
