package ui

import (
	"context"
	"strings"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/core"
	"github.com/San-Shiro/QuickFlare/internal/supervisor"
)

// Provisioning, as the panel sees it: which goroutine does the work, and how
// the result reaches the list.
//
// The rules themselves - what order DNS and ingress go in, and what is safe
// to leave behind - live in internal/core, so the CLI applies exactly the
// same ones. What is left here is the threading: every network call runs off
// the Gio loop and comes back through dispatch.

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

		tun, err := core.FindOrCreateTunnel(ctx, client)
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

// provisionRoute publishes one route, off the Gio loop.
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

		err := core.Publish(ctx, client, zoneID, tunnelID, hostname, target)
		p.dispatch(func() {
			if err != nil {
				p.setRouteStatus(hostname, statusError, err.Error())
				return
			}
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

// deprovisionRoute tears a route down, off the Gio loop, and reports whatever
// core could not clean up.
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

		problems := core.Unpublish(ctx, client, zoneID, tunnelID, hostname)

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
