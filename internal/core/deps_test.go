package core_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestCoreHasNoUIDependency is the whole reason this package exists.
//
// The domain logic used to be methods on the Gio panel, so a command-line
// tool could not reach it without linking a GUI toolkit - and on Linux, a GUI
// toolkit means cgo and X11 headers, which makes cross-compiling impossible.
// A comment saying "do not import Gio here" would not survive a busy
// afternoon. This does.
func TestCoreHasNoUIDependency(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps",
		"github.com/San-Shiro/QuickFlare/internal/core").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}

	banned := []string{
		"gioui.org",                // the GUI toolkit, and cgo with it
		"fyne.io",                  // the tray
		"golang.org/x/sys/windows", // Windows-only syscalls
	}

	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		dep = strings.TrimSpace(dep)
		for _, b := range banned {
			if dep == b || strings.HasPrefix(dep, b+"/") {
				t.Errorf("internal/core depends on %q via %q.\n"+
					"core must stay importable from a CLI on any platform - "+
					"move whatever needs this into the frontend.", b, dep)
			}
		}
	}
}
