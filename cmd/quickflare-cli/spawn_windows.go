//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
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

	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(bin)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(bin))

	// ShellExecuteW delegates launching to Windows Shell (explorer.exe),
	// completely decoupling the tray application from the caller's console,
	// terminal Job Object, and process group.
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		uintptr(unsafe.Pointer(dir)),
		windows.SW_SHOWNORMAL,
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecute failed with error code %d", ret)
	}
	return nil
}
