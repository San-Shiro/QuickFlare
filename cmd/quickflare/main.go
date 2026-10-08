// Command quickflare is a tray application for publishing localhost services
// through a Cloudflare Tunnel.
package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"

	"github.com/San-Shiro/QuickFlare/internal/ui"
	"golang.org/x/sys/windows"
)

func setupLogging() *os.File {
	localApp := os.Getenv("LOCALAPPDATA")
	if localApp == "" {
		return nil
	}
	dir := filepath.Join(localApp, "QuickFlare")
	_ = os.MkdirAll(dir, 0755)
	logPath := filepath.Join(dir, "tray.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil
	}
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	os.Stdout = f
	os.Stderr = f
	_ = windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(f.Fd()))
	return f
}

func main() {
	runtime.LockOSThread()
	ui.AttachDefaultDesktop()
	logFile := setupLogging()
	if logFile != nil {
		defer logFile.Close()
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[quickflare-tray] FATAL PANIC in main: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()

	log.Printf("[quickflare-tray] Starting QuickFlare Tray...")

	if !acquireSingleInstance() {
		log.Printf("[quickflare-tray] Another instance is already running; exiting.")
		return
	}
	log.Printf("[quickflare-tray] Acquired single instance lock, starting UI...")
	ui.Run()
	log.Printf("[quickflare-tray] ui.Run() returned! main() exiting.")
}
