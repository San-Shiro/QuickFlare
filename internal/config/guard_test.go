package config

import (
	"errors"
	"os"
	"testing"
)

// The guard must actually fire - it exists because a test silently destroyed
// a real saved token once already.
func TestSaveRefusesToWriteRealConfigFromTests(t *testing.T) {
	// No t.Setenv here on purpose: this is the unsandboxed case.
	dir, err := Dir()
	if err != nil {
		t.Skipf("no config dir on this platform: %v", err)
	}
	if err := guardTestWrites(dir); !errors.Is(err, errTestWrite) {
		t.Fatalf("guard should refuse the real config dir %q, got %v", dir, err)
	}

	c := &Config{APIToken: "would-have-clobbered-the-real-one"}
	if err := c.Save(); !errors.Is(err, errTestWrite) {
		t.Fatalf("Save should refuse, got %v", err)
	}
}

// A properly sandboxed test must still be able to save.
func TestSaveAllowedWhenSandboxed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}
	c := &Config{APIToken: "sandboxed", Domain: "example.com"}
	if err := c.Save(); err != nil {
		t.Fatalf("sandboxed save should succeed: %v", err)
	}
}
