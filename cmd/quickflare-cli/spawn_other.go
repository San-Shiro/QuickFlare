//go:build !windows

package main

import (
	"errors"
)

func startTrayProcess() error {
	return errors.New("tray application is only available on Windows")
}
