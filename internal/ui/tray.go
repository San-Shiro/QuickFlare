package ui

import (
	_ "embed"
	"os"

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
// panel's event loop each get their own. systray.Run locks its own OS thread
// internally, which is what lets the two message loops coexist on Windows.
func Run() {
	p := NewPanel()

	ipcServer, _ := ipc.StartServer(ipc.Handlers{
		OnQuit: func() {
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
			p.Open()
			return nil
		},
		OnStatus: func() ipc.StatusData {
			return p.StatusData()
		},
	})
	if ipcServer != nil {
		defer ipcServer.Close()
	}

	go systray.Run(func() { onReady(p) }, func() {
		if ipcServer != nil {
			_ = ipcServer.Close()
		}
		onExit(p)
	})

	go func() {
		err := p.Loop()
		p.Shutdown()
		if ipcServer != nil {
			_ = ipcServer.Close()
		}
		if err != nil {
			os.Exit(1)
		}
		systray.Quit()
	}()

	app.Main()
}

func onReady(p *Panel) {
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
	p.Shutdown()
	os.Exit(0)
}
