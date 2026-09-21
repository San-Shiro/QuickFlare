package ui

import (
	"context"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
)

// Reconciliation: making the list match what Cloudflare actually has.
//
// The stored route list is a cache. The real state lives in two places on
// Cloudflare - a DNS record per hostname, and the tunnel's ingress list - and
// either can change without this app running: a record deleted in the
// dashboard, a route added from another machine, a tunnel rebuilt. Trusting
// the local file would show routes that no longer serve anything, and hide
// ones that do.
//
// So on launch every stored route is re-checked, and anything the tunnel is
// serving that the file has never heard of is adopted. That last part is what
// stops orphans accumulating: before this, a route outlived a restart on
// Cloudflare while vanishing from the UI, leaving DNS records and ingress
// rules nobody could see or clean up.

// reconcileState is what one hostname looks like on Cloudflare.
type reconcileState struct {
	inDNS     bool
	inIngress bool
	target    string // local target, as the ingress rule has it
}

// reconcileRoutes rebuilds the route list from Cloudflare and heals anything
// half-published.
func (p *Panel) reconcileRoutes() {
	client := p.cf
	if client == nil || p.tunnelID == "" {
		return
	}

	stored := append([]Route(nil), p.routes...)
	tunnelID := p.tunnelID
	zoneID := p.currentZoneID()

	p.setStatus("Checking routes against Cloudflare...", colWarning)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		// One call gives every hostname the tunnel serves, and the local
		// port each points at - enough to adopt routes this machine has
		// never seen.
		doc, err := client.TunnelConfig(ctx, tunnelID)
		if err != nil {
			p.dispatch(func() {
				p.setStatus("Could not read tunnel config: "+err.Error(), colError)
			})
			return
		}

		states := map[string]*reconcileState{}
		for _, rule := range doc.Ingress {
			if rule.Hostname == "" {
				continue // the catch-all
			}
			host := strings.ToLower(rule.Hostname)
			states[host] = &reconcileState{
				inIngress: true,
				target:    strings.TrimPrefix(rule.Service, "http://"),
			}
		}

		// DNS is checked per stored route: a hostname with no record no
		// longer resolves, however healthy its ingress rule looks.
		target := cloudflare.TunnelCNAMETarget(tunnelID)
		for _, r := range stored {
			host := strings.ToLower(r.Hostname)
			if states[host] == nil {
				states[host] = &reconcileState{}
			}
			zone := r.ZoneID
			if zone == "" {
				zone = zoneID
			}
			if zone == "" {
				continue
			}
			rec, err := client.FindDNSRecord(ctx, zone, "CNAME", r.Hostname)
			if err != nil {
				// Treat a lookup failure as "unknown, leave alone" rather
				// than deleting a route because the network blipped.
				states[host].inDNS = true
				continue
			}
			states[host].inDNS = rec != nil && rec.Content == target
		}

		p.dispatch(func() {
			p.applyReconcile(stored, states, zoneID)
		})
	}()
}

// reconcileList is the decision half: given what is stored and what
// Cloudflare actually has, work out the new route list.
//
// Kept separate from acting on it so the rules below are testable on their
// own - the branch that deletes a user's routes should not require a live
// Cloudflare account to exercise.
func reconcileList(stored []Route, states map[string]*reconcileState, defaultZone string) (kept []Route, dropped, adopted []string) {
	byHost := map[string]Route{}
	for _, r := range stored {
		byHost[strings.ToLower(r.Hostname)] = r
	}

	for host, st := range states {
		known, isKnown := byHost[host]

		// Neither half present: the route is gone from Cloudflare, so it
		// goes from the list too rather than sitting there pretending.
		if !st.inDNS && !st.inIngress {
			if isKnown {
				dropped = append(dropped, known.Hostname)
			}
			continue
		}

		r := known
		if !isKnown {
			// Serving on Cloudflare but absent here - adopt it, using the
			// port the ingress rule already names.
			r = Route{Hostname: host, Target: st.target, ZoneID: defaultZone}
			adopted = append(adopted, host)
		}
		if r.Target == "" {
			r.Target = st.target
		}
		if r.ZoneID == "" {
			r.ZoneID = defaultZone
		}

		if st.inDNS && st.inIngress {
			r.Status, r.Detail = statusConnected, ""
		} else {
			// Half-published: one side survived. Republishing restores the
			// missing half instead of leaving a route that resolves nowhere
			// or a rule nothing points at.
			r.Status, r.Detail = statusStarting, "restoring..."
		}
		kept = append(kept, r)
	}
	return kept, dropped, adopted
}

// applyReconcile installs the reconciled list and republishes what is only
// half there.
func (p *Panel) applyReconcile(stored []Route, states map[string]*reconcileState, defaultZone string) {
	kept, dropped, adopted := reconcileList(stored, states, defaultZone)
	p.routes = kept

	// Collected before provisioning: provisionRoute rewrites Status, so
	// deciding from it mid-loop would act on what the loop just changed.
	var repair []Route
	for _, r := range kept {
		if r.Status == statusStarting {
			repair = append(repair, r)
		}
	}

	p.saveConfig()
	if msg, tone := reconcileSummary(len(kept), dropped, adopted); msg != "" {
		p.setStatus(msg, tone)
	}

	// Every step adopts what is already there, so this is safe to run over
	// a route that turns out to be healthy after all.
	for _, r := range repair {
		p.provisionRoute(r.Hostname, r.Target)
	}
}

// reconcileSummary is one short line for a strip one line tall.
//
// It used to spell out every hostname it had removed, which produced a
// three-line paragraph overflowing the footer - and marked a route
// disappearing as success. The list itself already shows what survived, so
// this only needs to flag the part the user did not do: routes that went
// away on their own.
func reconcileSummary(kept int, dropped, adopted []string) (string, color.NRGBA) {
	if n := len(dropped); n > 0 {
		return plural(n, "route") + " removed", colWarning
	}
	if n := len(adopted); n > 0 {
		return plural(n, "route") + " found", colSuccess
	}
	if kept > 0 {
		return plural(kept, "route") + " ready", colSuccess
	}
	return "", colTextDisabled
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

func itoa(n int) string { return strconv.Itoa(n) }
