package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
)

// session is the state every command needs: the stored config, a live API
// client, and the tunnel the account uses.
//
// Loaded per invocation rather than held open. Nothing here is expensive
// enough to justify a daemon, and Cloudflare is the source of truth anyway -
// see the package comment.
type session struct {
	cfg      *config.Config
	client   *cloudflare.Client
	tunnelID string
}

// open loads the config and builds a client. It does not touch the network.
func open() (*session, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("no API token stored - run 'quickflare login' first")
	}
	return &session{cfg: cfg, client: cloudflare.New(cfg.APIToken, "")}, nil
}

// withAccount resolves the account id, which most endpoints need and which
// the token alone does not carry.
func (s *session) withAccount(ctx context.Context) error {
	if s.client.AccountID() != "" {
		return nil
	}
	accounts, err := s.client.Accounts(ctx)
	if err != nil {
		return fmt.Errorf("list accounts: %w", err)
	}
	if len(accounts) == 0 {
		return fmt.Errorf("token can see no accounts - check its permissions")
	}
	s.client.SetAccountID(accounts[0].ID)
	return nil
}

// withTunnel finds or creates the QuickFlare tunnel.
func (s *session) withTunnel(ctx context.Context) error {
	if s.tunnelID != "" {
		return nil
	}
	if err := s.withAccount(ctx); err != nil {
		return err
	}
	tun, err := core.FindOrCreateTunnel(ctx, s.client)
	if err != nil {
		return err
	}
	s.tunnelID = tun.ID
	return nil
}

// zone resolves a domain name to its zone, defaulting to the stored one.
//
// The default matters: a user with one domain should never have to name it,
// and a user with several should not have a route land on whichever zone the
// API happened to list first.
func (s *session) zone(ctx context.Context, name string) (*cloudflare.Zone, error) {
	if name == "" {
		name = s.cfg.Domain
	}
	if name == "" {
		return nil, fmt.Errorf("no domain set - pass --domain, or run 'quickflare login' to pick one")
	}
	z, err := s.client.ZoneByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("look up %s: %w", name, err)
	}
	if z == nil {
		return nil, fmt.Errorf("this token cannot reach the domain %q", name)
	}
	return z, nil
}

// routes returns the stored route cache as core routes.
func (s *session) routes() []core.Route {
	out := make([]core.Route, 0, len(s.cfg.Routes))
	for _, r := range s.cfg.Routes {
		out = append(out, core.Route{
			Hostname: r.Hostname,
			Target:   r.Target,
			ZoneID:   r.ZoneID,
		})
	}
	return out
}

// saveRoutes writes the route cache back.
func (s *session) saveRoutes(routes []core.Route) error {
	stored := make([]config.StoredRoute, 0, len(routes))
	for _, r := range routes {
		stored = append(stored, config.StoredRoute{
			Hostname: r.Hostname,
			Target:   r.Target,
			ZoneID:   r.ZoneID,
		})
	}
	s.cfg.Routes = stored
	if err := s.cfg.Save(); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

// hostname builds the full name for a label in a zone.
func hostname(label, domain string) string {
	return strings.ToLower(label) + "." + strings.ToLower(domain)
}
