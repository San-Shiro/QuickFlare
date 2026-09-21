//go:build !windows

package ui

import (
	"os/exec"
	"runtime"
)

func openURL(url string) error {
	cmd := "xdg-open"
	if runtime.GOOS == "darwin" {
		cmd = "open"
	}
	return exec.Command(cmd, url).Start()
}
