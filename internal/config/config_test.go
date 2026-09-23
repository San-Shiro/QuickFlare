package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtectRoundTrip(t *testing.T) {
	const secret = "cf-token-abc123-DEF456_xyz"

	enc, err := protect([]byte(secret))
	if err != nil {
		t.Fatalf("protect: %v", err)
	}
	if secretsAreEncrypted && strings.Contains(string(enc), secret) {
		t.Fatal("ciphertext still contains the plaintext token")
	}

	plain, err := unprotect(enc)
	if err != nil {
		t.Fatalf("unprotect: %v", err)
	}
	if string(plain) != secret {
		t.Errorf("round trip mismatch: got %q want %q", plain, secret)
	}
}

// The whole point of the package: a token must never reach disk readable.
func TestSaveDoesNotWritePlaintextToken(t *testing.T) {
	if !secretsAreEncrypted {
		t.Skip("platform has no secret protection wired up yet")
	}
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	const secret = "super-secret-token-value"
	c := &Config{APIToken: secret, Domain: "example.com"}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("config file contains the plaintext token:\n%s", raw)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.APIToken != secret {
		t.Errorf("token did not survive the round trip: got %q", got.APIToken)
	}
	if got.Domain != "example.com" {
		t.Errorf("domain did not survive: got %q", got.Domain)
	}
}

func TestLoadMissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	c, err := Load()
	if err != nil {
		t.Fatalf("first run should not error: %v", err)
	}
	if c.APIToken != "" || c.Domain != "" {
		t.Errorf("expected empty config, got %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, "QuickFlare", "config.json")); err == nil {
		t.Error("Load should not create a file")
	}
}

func TestRoutesDisabledAndRestoreStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	c := &Config{
		Domain:           "example.com",
		RoutesDisabled:   true,
		RestoreLastState: true,
	}
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !got.RoutesDisabled {
		t.Errorf("RoutesDisabled not persisted: got false, want true")
	}
	if !got.RestoreLastState {
		t.Errorf("RestoreLastState not persisted: got false, want true")
	}
}
