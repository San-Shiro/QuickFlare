package ui

import (
	_ "embed"
	"os"

	"fyne.io/systray"
	"gioui.org/app"

	"github.com/San-Shiro/QuickFlare/internal/autostart"
)

//go:embed assets/tray.ico
var trayIcon []byte

// Run wires the tray to the panel and blocks.
//
// Threading: Gio requires app.Main() on the main goroutine, so systray and the
// panel's event loop each get their own. systray.Run locks its own OS thread
// internally, which is what lets the two message loops coexist on Windows.
func Run() {
	p := NewPanel()

	go systray.Run(func() { onReady(p) }, func() { onExit(p) })

	go func() {
		err := p.Loop()
		p.Shutdown()
		if err != nil {
			os.Exit(1)
		}
		systray.Quit()
	}()

	app.Main()
}

func onReady(p *Panel) {
	systray.SetIcon(trayIcon)
	systray.SetTitle("QuickFlare")
	systray.SetTooltip("QuickFlare - Cloudflare tunnel routes")

	// Left-click the tray icon toggles the panel, WARP-style.
	systray.SetOnTapped(p.Toggle)

	mOpen := systray.AddMenuItem("Open", "Show the QuickFlare panel")

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
