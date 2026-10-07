package ui

import (
	_ "embed"
	"log"
	"os"
	"runtime"
	"runtime/debug"

	"fyne.io/systray"
	"gioui.org/app"

	"github.com/San-Shiro/QuickFlare/internal/autostart"
	"github.com/San-Shiro/QuickFlare/internal/ipc"
)

//go:embed assets/tray.ico
var trayIcon []byte

//go:embed assets/tray_disabled.ico
var trayIconDisabled []byte

// Run wires the tray to the panel and blocks.
//
// Threading: Gio requires app.Main() on the main goroutine, so systray and the
// panel's event loop each get their own. systray.Run must lock its dedicated OS thread
// so the Win32 message queue (bound to the thread creating the SystrayClass window)
// is permanently pumped by GetMessage without being preempted or migrated by Go's scheduler.
func Run() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tray] FATAL PANIC in Run: %v\nStack trace:\n%s", r, debug.Stack())
		}
	}()
	log.Printf("[tray] Initializing panel...")
	p := NewPanel()
	log.Printf("[tray] Panel initialized successfully")

	log.Printf("[tray] Starting IPC server...")
	ipcServer, err := ipc.StartServer(ipc.Handlers{
		OnQuit: func() {
			log.Printf("[tray] OnQuit received via IPC")
			p.Shutdown()
			systray.Quit()
			os.Exit(0)
		},
		OnPause: func() error {
			p.SetDisabled(true)
			return nil
		},
		OnResume: func() error {
			p.SetDisabled(false)
			return nil
		},
		OnReload: func() error {
			p.ReloadFromDisk()
			return nil
		},
		OnOpen: func() error {
			log.Printf("[tray] OnOpen received via IPC")
			p.Open()
			return nil
		},
		OnStatus: func() ipc.StatusData {
			return p.StatusData()
		},
	})
	if err != nil {
		log.Printf("[tray] Warning: IPC server failed to start: %v", err)
	} else {
		log.Printf("[tray] IPC server started successfully")
	}
	if ipcServer != nil {
		defer ipcServer.Close()
	}

	go func() {
		runtime.LockOSThread()
		AttachDefaultDesktop()
		log.Printf("[tray] Entering systray.Run event loop...")
		systray.Run(func() { onReady(p) }, func() {
			if ipcServer != nil {
				_ = ipcServer.Close()
			}
			onExit(p)
		})
	}()

	go func() {
		log.Printf("[tray] Entering panel.Loop event loop...")
		err := p.Loop()
		log.Printf("[tray] panel.Loop exited (err: %v)", err)
		p.Shutdown()
		if ipcServer != nil {
			_ = ipcServer.Close()
		}
		if err != nil {
			os.Exit(1)
		}
		systray.Quit()
		os.Exit(0)
	}()

	app.Main()
	log.Printf("[tray] app.Main() returned! Exiting Run().")
}

func onReady(p *Panel) {
	log.Printf("[tray] Systray onReady: configuring icon, menu, and callbacks...")
	initialDisabled := p.IsDisabled()
	if initialDisabled {
		systray.SetIcon(trayIconDisabled)
		systray.SetTooltip("QuickFlare - Routes paused (disabled)")
	} else {
		systray.SetIcon(trayIcon)
		systray.SetTooltip("QuickFlare - Cloudflare tunnel routes")
	}
	systray.SetTitle("QuickFlare")

	// Left-click the tray icon toggles the panel, WARP-style.
	systray.SetOnTapped(p.Toggle)

	mOpen := systray.AddMenuItem("Open", "Show the QuickFlare panel")

	disableTitle := "Disable Routes"
	if initialDisabled {
		disableTitle = "Enable Routes"
	}
	mDisable := systray.AddMenuItem(disableTitle, "Pause or resume the Cloudflare connector")

	p.OnDisableChanged(func(disabled bool) {
		if disabled {
			mDisable.SetTitle("Enable Routes")
			systray.SetIcon(trayIconDisabled)
			systray.SetTooltip("QuickFlare - Routes paused (disabled)")
		} else {
			mDisable.SetTitle("Disable Routes")
			systray.SetIcon(trayIcon)
			systray.SetTooltip("QuickFlare - Cloudflare tunnel routes")
		}
	})

	// "Start with Windows" lives here as well as in Settings because Settings
	// is only reachable once a token is verified or a quick tunnel is
	// running. On a fresh install the onboarding screen has no way through to
	// it, which left the setting impossible to turn on at exactly the moment
	// someone would want to.
	var mStartup *systray.MenuItem
	if autostart.Supported() {
		systray.AddSeparator()
		on, err := autostart.Enabled()
		if err != nil {
			dbg("autostart state: %v", err)
		}
		mStartup = systray.AddMenuItemCheckbox(
			"Start with Windows", "Launch QuickFlare when you sign in", on)
	}

	systray.AddSeparator()
	mReport := systray.AddMenuItem("Report an Issue...", "Report a bug or problem on GitHub")
	mLogs := systray.AddMenuItem("Open Logs Folder", "View diagnostic log files")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Exit QuickFlare")

	startupClicks := make(chan struct{})
	if mStartup != nil {
		go func() {
			for range mStartup.ClickedCh {
				startupClicks <- struct{}{}
			}
		}()
	}

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				p.Open()

			case <-mDisable.ClickedCh:
				p.ToggleDisabled()

			case <-startupClicks:
				on, err := p.SetAutostart(!mStartup.Checked())
				if err != nil {
					dbg("autostart toggle: %v", err)
				}
				// Follow the machine, not the click: if the write failed the
				// tick stays where the registry actually is.
				if on {
					mStartup.Check()
				} else {
					mStartup.Uncheck()
				}

			case <-mReport.ClickedCh:
				_ = openURL(bugReportURL)

			case <-mLogs.ClickedCh:
				p.openLogsDirectory()

			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// onExit runs when the tray quits. It must tear the tunnels down first.
//
// os.Exit does not unwind anything: no deferred calls, no Gio DestroyEvent,
// and - critically - no signal to the cloudflared children. Quitting used to
// orphan every connector, so the tunnel and any quick links carried on
// serving after the app was gone, with no UI left to stop them. The only
// place that can now be fixed is here, before the process dies.
func onExit(p *Panel) {
	log.Printf("[tray] Systray onExit: shutting down tunnels and exiting")
	p.Shutdown()
	os.Exit(0)
}
