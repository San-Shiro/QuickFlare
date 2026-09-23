//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func broadcastSettingChange() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	sendMessageTimeout := user32.NewProc("SendMessageTimeoutW")
	envPtr, _ := windows.UTF16PtrFromString("Environment")

	const (
		HWND_BROADCAST   = 0xFFFF
		WM_SETTINGCHANGE = 0x001A
		SMTO_ABORTIFHUNG = 0x0002
	)

	var result uintptr
	_, _, _ = sendMessageTimeout.Call(
		uintptr(HWND_BROADCAST),
		uintptr(WM_SETTINGCHANGE),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(SMTO_ABORTIFHUNG),
		5000,
		uintptr(unsafe.Pointer(&result)),
	)
}

func cmdPath(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("path", flag.ExitOnError)
	fs.Parse(args)

	sub := "status"
	if len(fs.Args()) > 0 {
		sub = fs.Args()[0]
	}

	selfPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	selfDir := filepath.Dir(selfPath)

	k, err := registry.OpenKey(registry.CURRENT_USER, "Environment", registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open user environment registry: %w", err)
	}
	defer k.Close()

	curPath, _, err := k.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("read user PATH: %w", err)
	}

	parts := strings.Split(curPath, ";")
	cleanParts := make([]string, 0, len(parts))
	alreadyIn := false
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		cleanParts = append(cleanParts, trimmed)
		if strings.EqualFold(trimmed, selfDir) {
			alreadyIn = true
		}
	}

	switch sub {
	case "status":
		fmt.Printf("Directory:   %s\n", selfDir)
		if alreadyIn {
			fmt.Println("User PATH:   Registered (available in cmd / PowerShell)")
		} else {
			fmt.Println("User PATH:   Not registered")
			fmt.Println("\nRun 'quickflare path install' to register this directory into your user PATH.")
		}
		return nil

	case "install", "add":
		if alreadyIn {
			fmt.Printf("%s is already in user PATH.\n", selfDir)
			return nil
		}
		cleanParts = append(cleanParts, selfDir)
		newVal := strings.Join(cleanParts, ";")
		if err := k.SetStringValue("Path", newVal); err != nil {
			return fmt.Errorf("write user PATH: %w", err)
		}
		broadcastSettingChange()
		fmt.Printf("Successfully added %s to user PATH.\n", selfDir)
		fmt.Println("Open a new cmd or PowerShell window and type 'quickflare' to use it.")
		return nil

	case "remove", "uninstall", "rm":
		if !alreadyIn {
			fmt.Printf("%s is not in user PATH.\n", selfDir)
			return nil
		}
		var remaining []string
		for _, p := range cleanParts {
			if !strings.EqualFold(p, selfDir) {
				remaining = append(remaining, p)
			}
		}
		newVal := strings.Join(remaining, ";")
		if err := k.SetStringValue("Path", newVal); err != nil {
			return fmt.Errorf("write user PATH: %w", err)
		}
		broadcastSettingChange()
		fmt.Printf("Successfully removed %s from user PATH.\n", selfDir)
		return nil

	default:
		return fmt.Errorf("unknown path action %q (use: install, remove, status)", sub)
	}
}
