//go:build windows

package ui

import "os/exec"

// openURL opens a link in the user's default browser.
//
// rundll32 url.dll,FileProtocolHandler rather than "cmd /c start": start is a
// shell builtin that would need a console, and this app deliberately has none.
func openURL(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
