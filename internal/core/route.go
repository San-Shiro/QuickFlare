// Package core is QuickFlare's domain logic, with no user interface attached.
//
// It exists because the provisioning and reconciliation rules used to live in
// internal/ui as methods on the Gio panel, which meant a command-line tool
// could not reach them without importing a GUI toolkit. Everything here is
// callable from a tray app, a terminal, or a test.
//
// Nothing in this package may import gioui.org. core_deps_test.go enforces
// that rather than trusting it.
package core

import (
	"fmt"
	"strconv"
	"strings"
)

// TunnelName is the tunnel QuickFlare creates and reuses. Keeping a stable
// name means a reinstall adopts the existing tunnel rather than littering the
// account with duplicates.
const TunnelName = "quickflare"

// Status is where a route has got to.
type Status int

const (
	StatusIdle Status = iota
	StatusStarting
	StatusConnected
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusStarting:
		return "starting"
	case StatusConnected:
		return "connected"
	case StatusError:
		return "error"
	}
	return "idle"
}

// Route is one published hostname on the user's own domain.
//
// This carries no presentation state. A frontend that needs per-row widgets
// or timers keeps them alongside, keyed by hostname - routes are addressed by
// name throughout, because a row's position moves.
type Route struct {
	Hostname string
	Target   string // "localhost:3000"
	Status   Status
	Detail   string // overrides Target when set, e.g. "port 4000 refused"

	// ZoneID is the zone this route was published into. Carried on the route
	// rather than looked up at deletion time: a user with several domains
	// would otherwise have a delete aimed at whichever zone is selected now.
	ZoneID string
}

// ParsePort validates a port number.
func ParsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 65535 {
		return 0, ErrPort
	}
	return n, nil
}

type portError struct{}

func (portError) Error() string { return "Port must be a number between 1 and 65535" }

// ErrPort is returned by ParsePort for anything that is not a usable port.
var ErrPort = portError{}

// ValidateLabel checks a subdomain is a single, legal DNS label.
//
// The dot rule is about TLS: the free Universal certificate covers *.domain
// but not *.a.domain, so a two-label name fails in the browser. The rest is
// about not sending junk to the API - and specifically about "*", which would
// publish a wildcard record and hand the entire zone to this tunnel. That is
// the behaviour QuickFlare deliberately moved away from, so it must not be
// reachable by typing one character into a form or a flag.
func ValidateLabel(label string) error {
	switch {
	case label == "*":
		return fmt.Errorf("a wildcard would take over the whole domain - use a name")
	case label == "":
		return fmt.Errorf("subdomain is required")
	case strings.Contains(label, "."):
		return fmt.Errorf("subdomain must be a single label")
	case len(label) > 63:
		return fmt.Errorf("subdomain is too long (max 63 characters)")
	case strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-"):
		return fmt.Errorf("subdomain cannot start or end with a hyphen")
	}
	for _, r := range label {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isUpper && !isDigit && r != '-' {
			return fmt.Errorf("subdomain can only use letters, numbers and hyphens")
		}
	}
	return nil
}
