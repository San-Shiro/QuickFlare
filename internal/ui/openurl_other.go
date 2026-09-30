//go:build !windows

package ui

import (
	"os"
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

func openFolder(path string) error {
	_ = os.MkdirAll(path, 0755)
	return openURL(path)
}
