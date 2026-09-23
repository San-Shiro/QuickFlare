// Package config persists QuickFlare's settings between runs.
//
// The API token is a live credential: it can create tunnels and rewrite DNS
// on the account. It is therefore never written in plaintext - see
// protect/unprotect for what that means on each platform.
package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// StoredRoute is a published route remembered between runs.
//
// This is a cache, not the source of truth. Cloudflare holds the real state -
// DNS records and the tunnel's ingress list - and a route can be removed from
// the dashboard, or from another machine, without this file hearing about it.
// Everything here is therefore re-checked on launch rather than trusted.
type StoredRoute struct {
	Hostname string `json:"hostname"`
	Target   string `json:"target"`
	ZoneID   string `json:"zone_id"`
}

// Config is what survives a restart.
type Config struct {
	// APIToken is the decrypted token, in memory only. It is deliberately
	// not serialized: json:"-" is what keeps a plaintext credential from
	// reaching disk if someone adds a Marshal call somewhere else later.
	APIToken string `json:"-"`

	// TokenEnc is the protected form actually written to disk.
	TokenEnc string `json:"api_token_enc,omitempty"`

	// Domain is the zone that was selected last, so the picker comes back
	// where it was left rather than defaulting to whichever zone Cloudflare
	// happens to list first.
	Domain string `json:"domain,omitempty"`

	// Routes is the last known route list, reconciled against Cloudflare on
	// the next launch.
	Routes []StoredRoute `json:"routes,omitempty"`

	// RoutesDisabled is true if routes were paused/disabled when last running.
	RoutesDisabled bool `json:"routes_disabled,omitempty"`

	// RestoreLastState restores the last route state on launch when true.
	// When false (default), routes always default to OFF on launch.
	RestoreLastState bool `json:"restore_last_state,omitempty"`
}

// SecretsAreEncrypted reports whether the stored token gets real OS-backed
// protection on this platform.
func SecretsAreEncrypted() bool { return secretsAreEncrypted }

// Dir is the per-user directory holding the config file.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(base, "QuickFlare"), nil
}

// Path is the config file itself.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the stored config. A missing file is not an error - it is just a
// first run.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if c.TokenEnc != "" {
		enc, err := base64.StdEncoding.DecodeString(c.TokenEnc)
		if err != nil {
			return nil, fmt.Errorf("decode stored token: %w", err)
		}
		plain, err := unprotect(enc)
		if err != nil {
			// A blob written by a different Windows account, or on another
			// machine, cannot be decrypted here. That is the protection
			// working, not corruption - drop the token and let the user
			// paste a fresh one rather than failing to start.
			return &Config{Domain: c.Domain, Routes: c.Routes, RoutesDisabled: c.RoutesDisabled, RestoreLastState: c.RestoreLastState}, nil
		}
		c.APIToken = string(plain)
	}
	return &c, nil
}

// Save writes the config, protecting the token on the way out.
func (c *Config) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := guardTestWrites(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	out := *c
	out.TokenEnc = ""
	if c.APIToken != "" {
		enc, err := protect([]byte(c.APIToken))
		if err != nil {
			return fmt.Errorf("protect token: %w", err)
		}
		out.TokenEnc = base64.StdEncoding.EncodeToString(enc)
	}

	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	path := filepath.Join(dir, "config.json")

	// Write to a temp file and rename, so an interrupted write cannot leave
	// a half-written config that fails to parse on next launch.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
