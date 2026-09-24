package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/San-Shiro/QuickFlare/internal/installer"
)

func main() {
	var (
		flagSilent      = flag.Bool("silent", false, "Run in silent (headless) mode without GUI")
		flagUninstall   = flag.Bool("uninstall", false, "Launch uninstallation mode")
		flagCliOnly     = flag.Bool("cli-only", false, "Install CLI only (no tray popover)")
		flagNoPath      = flag.Bool("no-path", false, "Do not add installation directory to User PATH")
		flagInstallDir  = flag.String("dir", "", "Custom installation directory")
		flagKeepRoutes  = flag.Bool("keep-routes", false, "Do not unpublish Cloudflare routes on uninstall")
		flagKeepConfig  = flag.Bool("keep-config", false, "Do not delete %APPDATA%\\QuickFlare configuration")
	)

	// Check for Windows-style "/uninstall" or subcommand "uninstall"
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(arg, "/uninstall") || strings.EqualFold(arg, "uninstall") {
			*flagUninstall = true
		}
		if strings.EqualFold(arg, "/silent") || strings.EqualFold(arg, "/s") {
			*flagSilent = true
		}
	}

	flag.Parse()

	// Load binary payloads
	_ = loadPayload()

	targetDir := *flagInstallDir
	if targetDir == "" {
		targetDir = installer.DefaultInstallDir()
	}

	// Silent / Headless Mode
	if *flagSilent {
		ctx := context.Background()
		if *flagUninstall {
			fmt.Printf("QuickFlare: silent uninstall initiated...\n")
			opt := installer.Options{
				InstallDir:   targetDir,
				RemoveRoutes: !*flagKeepRoutes,
				RemoveConfig: !*flagKeepConfig,
			}
			err := installer.Uninstall(ctx, opt, func(step string, prog float32) {
				fmt.Printf("[%3.0f%%] %s\n", prog*100, step)
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "Uninstallation error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("QuickFlare uninstalled successfully.")
			os.Exit(0)
		}

		fmt.Printf("QuickFlare: silent install initiated...\n")
		opt := installer.Options{
			InstallDir:      targetDir,
			InstallTray:     !*flagCliOnly,
			AddToPath:       !*flagNoPath,
			CreateShortcuts: !*flagCliOnly,
			LaunchAfter:     false,
		}
		err := installer.Install(ctx, opt, func(step string, prog float32) {
			fmt.Printf("[%3.0f%%] %s\n", prog*100, step)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Installation error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("QuickFlare installed successfully.")
		os.Exit(0)
	}

	// Interactive GUI Mode
	initState := StateChoose
	windowTitle := "QuickFlare Setup"
	if *flagUninstall {
		initState = StateUninstallPrompt
		windowTitle = "QuickFlare Uninstaller"
	}

	ui := NewInstallerUI(initState)
	if *flagCliOnly {
		ui.installTray = false
	}
	if *flagNoPath {
		ui.addToPath = false
	}
	if *flagInstallDir != "" {
		ui.targetDir = *flagInstallDir
	}
	if *flagKeepRoutes {
		ui.removeRoutes = false
	}
	if *flagKeepConfig {
		ui.removeConfig = false
	}

	go func() {
		w := new(app.Window)
		ui.SetWindow(w)

		winW := unit.Dp(540)
		winH := unit.Dp(510)

		w.Option(
			app.Title(windowTitle),
			app.Size(winW, winH),
			app.MinSize(winW, winH),
			app.MaxSize(winW, winH),
			app.Decorated(true),
		)

		var ops op.Ops
		for {
			e := w.Event()

			if hwnd, ok := ViewHandle(e); ok && hwnd != 0 {
				go StyleInstallerWindow(hwnd)
			}

			switch e := e.(type) {
			case app.DestroyEvent:
				os.Exit(0)
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				ui.Layout(gtx)
				e.Frame(gtx.Ops)
			}
		}
	}()

	app.Main()
}
