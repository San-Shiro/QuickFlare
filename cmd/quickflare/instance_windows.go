//go:build windows

package main

import (
	"context"
	"time"

	"github.com/San-Shiro/QuickFlare/internal/ipc"
	"golang.org/x/sys/windows"
)

var singleInstanceMutex windows.Handle

func acquireSingleInstance() bool {
	name, err := windows.UTF16PtrFromString("Local\\QuickFlare_Tray_SingleInstance_Mutex")
	if err != nil {
		return true
	}
	h, err := windows.CreateMutex(nil, true, name)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			_ = windows.CloseHandle(h)
		}
		// An instance is already running. Signal it to open its panel.
		client, err := ipc.Discover()
		if err == nil && client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = client.Open(ctx)
		}
		return false
	}
	singleInstanceMutex = h
	return true
}
