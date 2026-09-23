package ui

import (
	"context"
	"image/color"
	"strconv"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/core"
)

// Reconciliation, as the panel sees it.
//
// The rules - what counts as present, what gets dropped, what gets adopted -
// are core.ReconcileList, shared with the CLI and tested there. What is left
// here is fetching off the Gio loop, turning core routes back into list rows,
// and saying what happened in one line of footer.

// reconcileRoutes rebuilds the route list from Cloudflare and heals anything
// half-published.
func (p *Panel) reconcileRoutes() {
	client := p.cf
	if client == nil || p.tunnelID == "" {
		return
	}

	stored := make([]core.Route, 0, len(p.routes))
	for _, r := range p.routes {
		stored = append(stored, r.Route)
	}
	tunnelID := p.tunnelID
	zoneID := p.currentZoneID()

	p.setStatus("Checking routes against Cloudflare...", colWarning)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()

		states, err := core.FetchStates(ctx, client, tunnelID, zoneID, stored)
		if err != nil {
			p.dispatch(func() {
				p.setStatus("Could not read tunnel config: "+err.Error(), colError)
			})
			return
		}

		p.dispatch(func() {
			p.applyReconcile(stored, states, zoneID)
		})
	}()
}

// applyReconcile installs the reconciled list and republishes what is only
// half there.
func (p *Panel) applyReconcile(stored []core.Route, states map[string]*core.State, defaultZone string) {
	kept, dropped, adopted := core.ReconcileList(stored, states, defaultZone)

	rows := make([]Route, 0, len(kept))
	for _, r := range kept {
		rows = append(rows, Route{Route: r})
	}
	p.routes = rows

	// Collected before provisioning: provisionRoute rewrites Status, so
	// deciding from it mid-loop would act on what the loop just changed.
	var repair []core.Route
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
	p.triggerPortProbe()
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
