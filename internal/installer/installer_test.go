package installer_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/San-Shiro/QuickFlare/internal/installer"
)

func TestDefaultInstallDir(t *testing.T) {
	dir := installer.DefaultInstallDir()
	if dir == "" {
		t.Fatal("DefaultInstallDir returned empty string")
	}
	base := filepath.Base(dir)
	if !strings.EqualFold(base, "QuickFlare") {
		t.Fatalf("expected install dir base to be 'QuickFlare', got: %s", base)
	}
}

func TestOptionsDefaults(t *testing.T) {
	opt := installer.Options{
		InstallTray:  true,
		AddToPath:    true,
		RemoveRoutes: true,
		RemoveConfig: true,
	}
	if !opt.InstallTray {
		t.Fatal("expected InstallTray to be true")
	}
	if !opt.AddToPath {
		t.Fatal("expected AddToPath to be true")
	}
	if !opt.RemoveRoutes {
		t.Fatal("expected RemoveRoutes to be true")
	}
}
