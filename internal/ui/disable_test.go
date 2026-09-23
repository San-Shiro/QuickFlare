package ui

import (
	"testing"

	"github.com/San-Shiro/QuickFlare/internal/config"
)

func TestPanelDisableCallbackAndState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	p := &Panel{}
	if p.IsDisabled() {
		t.Errorf("default panel should not be disabled before init")
	}

	var callbackCalled bool
	var callbackVal bool
	p.OnDisableChanged(func(disabled bool) {
		callbackCalled = true
		callbackVal = disabled
	})

	// Manually invoke setDisabled
	p.setDisabled(true)
	if !p.IsDisabled() {
		t.Errorf("expected panel to be disabled")
	}
	if !callbackCalled || !callbackVal {
		t.Errorf("callback not triggered with true: called=%v val=%v", callbackCalled, callbackVal)
	}

	// Flip restore state
	p.toggleRestoreLastState()
	if !p.restoreLastState {
		t.Errorf("expected restoreLastState to be true after toggle")
	}

	// Verify config saved
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	if !cfg.RoutesDisabled {
		t.Errorf("saved config RoutesDisabled = false, want true")
	}
	if !cfg.RestoreLastState {
		t.Errorf("saved config RestoreLastState = false, want true")
	}
}

func TestPanelRestoreDefaultsToDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Save a config where routes were NOT disabled, but RestoreLastState is false
	cfg := &config.Config{
		Domain:           "example.com",
		RoutesDisabled:   false,
		RestoreLastState: false,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &Panel{}
	p.restore()

	// Since RestoreLastState is false, launch must default to OFF (disabled = true)
	if !p.disabled {
		t.Errorf("expected p.disabled=true when RestoreLastState is false, got %v", p.disabled)
	}
}

func TestPanelRestoreHonorsLastStateWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Save a config where routes were NOT disabled, and RestoreLastState is true
	cfg := &config.Config{
		Domain:           "example.com",
		RoutesDisabled:   false,
		RestoreLastState: true,
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	p := &Panel{}
	p.restore()

	if p.disabled {
		t.Errorf("expected p.disabled=false when RestoreLastState is true and RoutesDisabled is false, got %v", p.disabled)
	}
}
