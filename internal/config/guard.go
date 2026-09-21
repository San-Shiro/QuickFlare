package config

import (
	"errors"
	"os"
	"strings"
)

// testBinarySuffix is how a Go test binary names itself.
const testBinarySuffix = ".test"

// errTestWrite is returned when a test tries to write the real config.
var errTestWrite = errors.New(
	"config: refusing to write the user's real config from a test - " +
		"set APPDATA (Windows) or XDG_CONFIG_HOME to a t.TempDir() first")

// guardTestWrites stops a test run from overwriting the user's actual
// settings.
//
// This is not hypothetical: a UI test calling a method that happened to call
// Save wrote a fake route into the real config and, because it used a
// zero-valued struct, replaced the stored API token with an empty one. The
// test passed; the user's saved credential was gone. A test that has not
// redirected its config directory should fail loudly instead.
func guardTestWrites(dir string) error {
	if !strings.HasSuffix(os.Args[0], testBinarySuffix) &&
		!strings.HasSuffix(os.Args[0], testBinarySuffix+".exe") {
		return nil
	}
	// Under a test, the directory must live somewhere temporary.
	tmp := os.TempDir()
	if tmp != "" && strings.HasPrefix(strings.ToLower(dir), strings.ToLower(tmp)) {
		return nil
	}
	return errTestWrite
}
