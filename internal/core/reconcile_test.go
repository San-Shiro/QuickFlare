package core

import (
	"testing"
)

// The decision table that governs whether a route survives a restart.
func TestApplyReconcile(t *testing.T) {
	stored := []Route{
		{Hostname: "healthy.example.com", Target: "localhost:3000", ZoneID: "z1"},
		{Hostname: "gone.example.com", Target: "localhost:4000", ZoneID: "z1"},
		{Hostname: "dnsonly.example.com", Target: "localhost:5000", ZoneID: "z1"},
		{Hostname: "ingressonly.example.com", Target: "localhost:6000", ZoneID: "z1"},
	}
	states := map[string]*State{
		"healthy.example.com":     {InDNS: true, InIngress: true, Target: "localhost:3000"},
		"gone.example.com":        {InDNS: false, InIngress: false},
		"dnsonly.example.com":     {InDNS: true, InIngress: false},
		"ingressonly.example.com": {InDNS: false, InIngress: true, Target: "localhost:6000"},
		// Serving on Cloudflare but never seen by this machine.
		"adopted.example.com": {InDNS: true, InIngress: true, Target: "localhost:7000"},
	}

	kept, dropped, adoptedHosts := ReconcileList(stored, states, "z1")
	_, _ = dropped, adoptedHosts

	got := map[string]Route{}
	for _, r := range kept {
		got[r.Hostname] = r
	}

	if _, ok := got["gone.example.com"]; ok {
		t.Error("a route absent from both DNS and ingress must be dropped")
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 surviving routes, got %d: %v", len(got), keys(got))
	}

	if r := got["healthy.example.com"]; r.Status != StatusConnected {
		t.Errorf("fully published route should be connected, got %v %q", r.Status, r.Detail)
	}
	for _, half := range []string{"dnsonly.example.com", "ingressonly.example.com"} {
		if r := got[half]; r.Status != StatusStarting {
			t.Errorf("%s is half-published and should be restoring, got %v", half, r.Status)
		}
	}

	adopted, ok := got["adopted.example.com"]
	if !ok {
		t.Fatal("a hostname the tunnel serves must be adopted, not ignored - that is how orphans accumulate")
	}
	if adopted.Target != "localhost:7000" {
		t.Errorf("adopted route should take its port from the ingress rule, got %q", adopted.Target)
	}
	if adopted.ZoneID != "z1" {
		t.Errorf("adopted route should get the current zone, got %q", adopted.ZoneID)
	}
}

// A DNS lookup failure must not be read as "the route is gone" - a network
// blip would otherwise delete live routes.
func TestApplyReconcileKeepsRouteWhenOnlyIngressKnown(t *testing.T) {
	stored := []Route{{Hostname: "keep.example.com", Target: "localhost:1", ZoneID: "z1"}}
	states := map[string]*State{
		"keep.example.com": {InDNS: true, InIngress: false},
	}
	kept, _, _ := ReconcileList(stored, states, "z1")

	if len(kept) != 1 {
		t.Fatalf("route should survive, got %d", len(kept))
	}
}

func keys(m map[string]Route) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
