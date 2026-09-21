package core

import (
	"context"
	"strings"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
)

// Reconciliation: making a stored route list match what Cloudflare actually
// has.
//
// The stored list is a cache. The real state lives in two places on
// Cloudflare - a DNS record per hostname, and the tunnel's ingress list - and
// either can change without this app running: a record deleted in the
// dashboard, a route added from another machine, a tunnel rebuilt. Trusting
// the local copy would show routes that no longer serve anything, and hide
// ones that do.
//
// So every stored route is re-checked, and anything the tunnel is serving
// that the cache has never heard of is adopted. That last part is what stops
// orphans accumulating: before it, a route outlived a restart on Cloudflare
// while vanishing from the UI, leaving DNS records and ingress rules nobody
// could see or clean up.

// State is what one hostname looks like on Cloudflare.
type State struct {
	InDNS     bool
	InIngress bool
	Target    string // local target, as the ingress rule has it
}

// FetchStates reads Cloudflare's view of every hostname the tunnel serves,
// plus the DNS status of each stored route.
//
// A DNS lookup that errors is recorded as present rather than absent. The
// alternative is deleting somebody's route because the network blipped.
func FetchStates(ctx context.Context, client *cloudflare.Client, tunnelID, defaultZone string, stored []Route) (map[string]*State, error) {
	doc, err := client.TunnelConfig(ctx, tunnelID)
	if err != nil {
		return nil, err
	}

	states := map[string]*State{}
	for _, rule := range doc.Ingress {
		if rule.Hostname == "" {
			continue // the catch-all
		}
		host := strings.ToLower(rule.Hostname)
		states[host] = &State{
			InIngress: true,
			Target:    strings.TrimPrefix(rule.Service, "http://"),
		}
	}

	// DNS is checked per stored route: a hostname with no record no longer
	// resolves, however healthy its ingress rule looks.
	want := cloudflare.TunnelCNAMETarget(tunnelID)
	for _, r := range stored {
		host := strings.ToLower(r.Hostname)
		if states[host] == nil {
			states[host] = &State{}
		}
		zone := r.ZoneID
		if zone == "" {
			zone = defaultZone
		}
		if zone == "" {
			continue
		}
		rec, err := client.FindDNSRecord(ctx, zone, "CNAME", r.Hostname)
		if err != nil {
			states[host].InDNS = true
			continue
		}
		states[host].InDNS = rec != nil && rec.Content == want
	}
	return states, nil
}

// ReconcileList is the decision half: given what is stored and what Cloudflare
// actually has, work out the new route list.
//
// Kept separate from acting on it so the rules below are testable on their
// own - the branch that deletes a user's routes should not require a live
// Cloudflare account to exercise.
func ReconcileList(stored []Route, states map[string]*State, defaultZone string) (kept []Route, dropped, adopted []string) {
	byHost := map[string]Route{}
	for _, r := range stored {
		byHost[strings.ToLower(r.Hostname)] = r
	}

	for host, st := range states {
		known, isKnown := byHost[host]

		// Neither half present: the route is gone from Cloudflare, so it goes
		// from the list too rather than sitting there pretending.
		if !st.InDNS && !st.InIngress {
			if isKnown {
				dropped = append(dropped, known.Hostname)
			}
			continue
		}

		r := known
		if !isKnown {
			// Serving on Cloudflare but absent here - adopt it, using the
			// port the ingress rule already names.
			r = Route{Hostname: host, Target: st.Target, ZoneID: defaultZone}
			adopted = append(adopted, host)
		}
		if r.Target == "" {
			r.Target = st.Target
		}
		if r.ZoneID == "" {
			r.ZoneID = defaultZone
		}

		if st.InDNS && st.InIngress {
			r.Status, r.Detail = StatusConnected, ""
		} else {
			// Half-published: one side survived. Republishing restores the
			// missing half instead of leaving a route that resolves nowhere
			// or a rule nothing points at.
			r.Status, r.Detail = StatusStarting, "restoring..."
		}
		kept = append(kept, r)
	}
	return kept, dropped, adopted
}
