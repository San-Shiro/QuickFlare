//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func findTrayExecutable() (string, error) {
	selfPath, err := os.Executable()
	if err == nil {
		selfDir := filepath.Dir(selfPath)
		candidates := []string{
			filepath.Join(selfDir, "quickflare-tray.exe"),
			filepath.Join(selfDir, "QuickFlare.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}

	localApp := os.Getenv("LOCALAPPDATA")
	if localApp != "" {
		candidates := []string{
			filepath.Join(localApp, "QuickFlare", "quickflare-tray.exe"),
			filepath.Join(localApp, "QuickFlare", "QuickFlare.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}

	return "", errors.New("could not find quickflare-tray.exe or QuickFlare.exe")
}

func startTrayProcess() error {
	bin, err := findTrayExecutable()
	if err != nil {
		return err
	}

	cmd := exec.Command(bin)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch %s: %w", bin, err)
	}
	return nil
}
