package cloudflare

import (
	"context"
	"fmt"
	"net/url"
)

// Account is a Cloudflare account the token can act on.
type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Zone is a domain in an account.
type Zone struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Account struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"account"`
}

// TokenStatus is the result of verifying an API token.
type TokenStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// VerifyToken checks the API token is live before we rely on it. Setup calls
// this first so a bad token fails with a clear message rather than as a
// confusing error three steps later.
func (c *Client) VerifyToken(ctx context.Context) (*TokenStatus, error) {
	var out TokenStatus
	if err := c.do(ctx, "GET", "/user/tokens/verify", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Accounts lists the accounts this token can reach.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var out []Account
	if err := c.do(ctx, "GET", "/accounts", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Zones lists the zones this token can reach, optionally filtered by name.
func (c *Client) Zones(ctx context.Context, name string) ([]Zone, error) {
	path := "/zones?per_page=50"
	if name != "" {
		path += "&name=" + url.QueryEscape(name)
	}
	var out []Zone
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ZoneByName resolves exactly one zone by domain name.
func (c *Client) ZoneByName(ctx context.Context, name string) (*Zone, error) {
	zones, err := c.Zones(ctx, name)
	if err != nil {
		return nil, err
	}
	for i := range zones {
		if zones[i].Name == name {
			return &zones[i], nil
		}
	}
	return nil, fmt.Errorf("zone %q not found for this token", name)
}

// requireAccount guards calls that need an account id.
func (c *Client) requireAccount() error {
	if c.accountID == "" {
		return fmt.Errorf("cloudflare: account id not set")
	}
	return nil
}
