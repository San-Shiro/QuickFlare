package ui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/San-Shiro/QuickFlare/internal/autostart"
	"github.com/San-Shiro/QuickFlare/internal/cloudflare"
	"github.com/San-Shiro/QuickFlare/internal/config"
	"github.com/San-Shiro/QuickFlare/internal/core"
	"github.com/San-Shiro/QuickFlare/internal/supervisor"
)

// view is which screen the panel is showing.
type view int

const (
	viewOnboarding view = iota
	viewTokenSetup
	viewMain
	viewAddRoute
	viewSettings
	viewConfirmDelete
)

// listMode is which source the main list is showing.
const (
	modeDomains = 0
	modeQuick   = 1
)

// connStatus is core.Status under the name this package has always used for
// it. Aliased rather than redeclared so there is exactly one set of status
// values across the tray app, the CLI and the tests.
type connStatus = core.Status

const (
	statusIdle      = core.StatusIdle
	statusStarting  = core.StatusStarting
	statusConnected = core.StatusConnected
	statusError     = core.StatusError
)

// statusColor maps a status to its dot colour. A free function, not a method:
// connStatus is an alias for a type in another package, and methods can only
// be defined where the type is.
func statusColor(s connStatus) color.NRGBA {
	switch s {
	case statusConnected:
		return colSuccess
	case statusStarting:
		return colWarning
	case statusError:
		return colError
	}
	return colTextDisabled
}

// Route is a core.Route plus the widget state one list row needs.
//
// The domain fields are embedded rather than copied, so r.Hostname and
// r.Status still read the same at every call site while there is only one
// definition of what a route is.
type Route struct {
	core.Route

	copyBtn  widget.Clickable
	stopBtn  widget.Clickable
	copiedAt time.Time
}

// QuickEntry is one anonymous trycloudflare.com tunnel.
type QuickEntry struct {
	Port   int
	URL    string
	Status connStatus
	Err    string

	tunnel *supervisor.Quick

	// cancel stops the tunnel even before StartQuick has returned a handle.
	// Without it, hitting Stop during the ~seconds a quick tunnel takes to
	// come up dropped the row while leaving cloudflared running - a public
	// link the user believed they had closed, with nothing left to close it.
	cancel context.CancelFunc

	copyBtn  widget.Clickable
	stopBtn  widget.Clickable
	copiedAt time.Time
}

// Settings is the tunnel configuration the user supplies once.
type Settings struct {
	APIToken string
}

// zoneOption is one Cloudflare zone the verified token can reach.
type zoneOption struct {
	Name      string
	ID        string
	AccountID string
}

// Panel is the tray popover: a borderless, always-on-top window that we place
// ourselves next to the tray icon. It is created once and shown/hidden, rather
// than built and destroyed per click, so opening it stays instant.
type Panel struct {
	w  *app.Window
	th *material.Theme
	ic *iconSet

	// hwnd and visible are touched from both the tray goroutine (Toggle) and
	// the Gio loop (window creation), so they live behind a mutex. So is
	// everything driving the fade and the focus watcher, which each run on
	// goroutines of their own.
	mu      sync.Mutex
	hwnd    uintptr
	visible bool

	// alpha is the window opacity the last fade step actually applied, so a
	// fade that interrupts another picks up where it left off rather than
	// snapping to an end state first.
	alpha uint8
	// fadeGen invalidates a running fade. Each new fade takes the next
	// number; a fade whose number is stale stops instead of fighting the one
	// that replaced it.
	fadeGen uint64

	// shownAt gives the focus watcher a grace period - foreground does not
	// move to the panel the instant it is shown, and without this the watcher
	// would close it again immediately.
	shownAt time.Time
	// autoHidAt is when the watcher last closed the panel, used to tell a
	// tray click that closed it from one that should open it. See Toggle.
	autoHidAt time.Time
	// watchOnce keeps a single focus watcher running: Gio can publish the
	// view event more than once, and setHandle runs for each.
	watchOnce sync.Once

	// post carries state changes from background goroutines onto the Gio
	// loop. Everything below this line is then touched by one goroutine only,
	// so the UI state needs no locking of its own.
	post chan func()

	view       view
	mode       int
	status     string
	statusTone color.NRGBA

	settings Settings
	routes   []Route
	quicks   []*QuickEntry
	zones    []zoneOption
	domainIx int

	// cf is set once a token has verified; nil until then. verifying guards
	// against a second Verify click starting an overlapping request.
	cf        *cloudflare.Client
	verifying bool

	// savedDomain is the zone remembered from the last run, used to restore
	// the picker once the live zone list comes back.
	savedDomain string

	// tunnelID and tunnel are QuickFlare's cloudflared tunnel: the id DNS
	// points at, and the connector process actually carrying traffic. Both
	// are empty until ensureTunnel has run.
	tunnelID string
	tunnel   *supervisor.Tunnel

	// closers holds one teardown func per cloudflared process started.
	//
	// Shutdown runs on the tray's goroutine while p.quicks and p.tunnel are
	// owned by the Gio loop, so reading those directly at exit would race
	// the UI mutating them. This registry is the one piece of shared state
	// between the two, and it only ever grows - which makes guarding it a
	// single mutex rather than an ownership question.
	closersMu    sync.Mutex
	closers      []func()
	shutdownOnce sync.Once

	binPath string
	binErr  error

	cfEngineVersion string

	// Onboarding
	choice      int
	choiceBtns  [2]widget.Clickable
	continueBtn widget.Clickable

	// Token setup / settings
	autostartOn  bool
	autostartBtn widget.Clickable
	openDashBtn  widget.Clickable
	docsSetupBtn widget.Clickable
	setupList    layout.List
	tokenEd      widget.Editor
	verifyBtn    widget.Clickable

	// Main panel
	closeBtn    widget.Clickable
	docsBtn     widget.Clickable
	segBtns     [2]widget.Clickable
	domainBtn   widget.Clickable
	domainOpen  bool
	domainRows  []widget.Clickable
	addTokenBtn widget.Clickable
	connList    layout.List
	addBtn      widget.Clickable
	settingsBtn widget.Clickable
	emptyAddBtn widget.Clickable

	// Delete confirmation. Held by hostname rather than index: the row can
	// move while the dialog is open.
	pendingDelete string
	confirmBtn    widget.Clickable

	// Add route
	hostEd    widget.Editor
	portEd    widget.Editor
	createBtn widget.Clickable
	cancelBtn widget.Clickable

	// Live port reachability cache
	portsMu        sync.RWMutex
	portsListening map[int]bool
}

const (
	panelW = unit.Dp(dimPanelW)
	panelH = unit.Dp(dimPanelH)

	dashboardTokenURL = "https://dash.cloudflare.com/profile/api-tokens"

	// docsURL is the project README, opened by the docs button.
	docsURL = "https://github.com/San-Shiro/QuickFlare#readme"

	// guideURL is the dedicated step-by-step Cloudflare API token setup guide.
	guideURL = "https://github.com/San-Shiro/QuickFlare/blob/main/docs/CLOUDFLARE_API_TOKEN.md"

	// copiedFor is how long a pill shows its copied confirmation.
	copiedFor = 1600 * time.Millisecond
)

func NewPanel() *Panel {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	th.Palette = material.Palette{
		Bg:         colBackground,
		Fg:         colTextPrimary,
		ContrastBg: colAccent,
		ContrastFg: colBackground,
	}

	w := new(app.Window)
	w.Option(
		app.Title("QuickFlare"),
		app.Size(panelW, panelH),
		app.MinSize(panelW, panelH),
		app.MaxSize(panelW, panelH),
		app.Decorated(false),
		app.TopMost(true),
	)

	p := &Panel{
		w:          w,
		th:         th,
		ic:         newIcons(),
		post:       make(chan func(), 32),
		view:       viewOnboarding,
		statusTone: colTextDisabled,
	}
	for _, ed := range []*widget.Editor{&p.hostEd, &p.portEd, &p.tokenEd} {
		ed.SingleLine = true
	}
	p.tokenEd.Mask = '•'
	p.connList.Axis = layout.Vertical
	p.setupList.Axis = layout.Vertical

	p.binPath, p.binErr = supervisor.FindBinary()
	p.cfEngineVersion = supervisor.EngineVersion(p.binPath)

	if autostart.Supported() {
		p.autostartOn, _ = autostart.Enabled()
		// Repoint a stale entry at this build. Silent on failure: a broken
		// startup entry is worth fixing, not worth a startup error dialog.
		if err := autostart.Refresh(); err != nil {
			dbg("autostart refresh: %v", err)
		}
	}

	p.restore()
	return p
}

// SetAutostart turns the login item on or off and reports where it ended up.
//
// Shared by the tray menu and the Settings toggle so the two can never
// disagree, and safe to call from either goroutine: it touches only the
// registry plus one bool the UI reads.
//
// The result follows the machine, not the request. It re-reads the registry
// afterwards and returns that, so a failed write leaves both the menu tick
// and the Settings switch showing off rather than claiming a setting that
// never took.
func (p *Panel) SetAutostart(on bool) (bool, error) {
	var err error
	if on {
		err = autostart.Enable()
	} else {
		err = autostart.Disable()
	}

	actual, readErr := autostart.Enabled()
	if readErr != nil {
		if err == nil {
			err = readErr
		}
		actual = false
	}

	p.dispatch(func() {
		p.autostartOn = actual
		switch {
		case err != nil:
			p.setStatus("Could not change startup setting: "+err.Error(), colError)
		case actual:
			p.setStatus("QuickFlare will start with Windows", colSuccess)
		default:
			p.setStatus("QuickFlare will not start with Windows", colTextDisabled)
		}
	})
	return actual, err
}

// toggleAutostart flips the setting from the Settings screen.
func (p *Panel) toggleAutostart() {
	want := !p.autostartOn
	go p.SetAutostart(want)
}

// storedRoutes is the route list in its persisted form.
func (p *Panel) storedRoutes() []config.StoredRoute {
	out := make([]config.StoredRoute, 0, len(p.routes))
	for _, r := range p.routes {
		out = append(out, config.StoredRoute{
			Hostname: r.Hostname,
			Target:   r.Target,
			ZoneID:   r.ZoneID,
		})
	}
	return out
}

// restore brings back the last session's token and zone.
//
// Zones themselves are deliberately not cached: which domains a token can
// reach, and whether their wildcards are still free, are facts about
// Cloudflare's state rather than ours, and a stale copy would be worse than
// no copy. The token is what gets persisted; the zone list is re-derived
// from it on every launch.
func (p *Panel) restore() {
	cfg, err := config.Load()
	if err != nil {
		dbg("config load failed: %v", err)
		p.setStatus("Could not read saved settings", colWarning)
		return
	}
	p.savedDomain = cfg.Domain

	// Load the cached list straight away so the panel is not empty while
	// the network catches up - but mark every row unverified, because
	// nothing here has been checked against Cloudflare yet.
	for _, sr := range cfg.Routes {
		p.routes = append(p.routes, Route{Route: core.Route{
			Hostname: sr.Hostname,
			Target:   sr.Target,
			ZoneID:   sr.ZoneID,
			Status:   statusIdle,
			Detail:   "checking...",
		}})
	}

	if cfg.APIToken == "" {
		return
	}
	p.settings.APIToken = cfg.APIToken
	p.tokenEd.SetText(cfg.APIToken)
	p.startVerify(cfg.APIToken)
}

func (p *Panel) setStatus(msg string, tone color.NRGBA) {
	p.status = msg
	p.statusTone = tone
}

// dispatch hands a state change to the Gio loop and wakes it.
func (p *Panel) dispatch(f func()) {
	select {
	case p.post <- f:
	default: // queue full: drop rather than block a background goroutine
	}
	p.w.Invalidate()
}

// Toggle shows or hides the panel. It is called from the tray goroutine and
// does its work directly through the window handle.
//
// This deliberately does NOT hand the job to the Gio event loop. A hidden
// window receives no paint messages, so Invalidate cannot wake the loop while
// the panel is down - the code that un-hides it would be waiting on an event
// that only un-hiding could produce. Placement is pure Win32 on the HWND and
// needs nothing from Gio, so it happens here instead.
func (p *Panel) Toggle() {
	p.mu.Lock()
	h, wasVisible := p.hwnd, p.visible
	if h == 0 {
		p.mu.Unlock()
		dbg("toggle ignored: no window handle yet")
		return
	}

	// Clicking the tray while the panel is open moves the foreground to the
	// shell first, so the focus watcher has almost always closed the panel
	// by the time this click arrives. Taken at face value it reads as
	// "closed, therefore open it" - and the panel could never be closed from
	// the tray at all. Treat a click landing right after an automatic close
	// as the click that caused it.
	if !wasVisible && !p.autoHidAt.IsZero() && time.Since(p.autoHidAt) < 400*time.Millisecond {
		p.autoHidAt = time.Time{}
		p.mu.Unlock()
		dbg("toggle swallowed: focus watcher had just closed the panel")
		return
	}

	p.visible = !wasVisible
	p.mu.Unlock()

	dbg("toggle: wasVisible=%v hwnd=%#x", wasVisible, h)

	if wasVisible {
		p.hidePanel(h)
		return
	}
	p.showPanel(h)
}

// Open shows the panel, for the tray menu's Open item.
//
// Not Toggle: opening the tray menu takes the foreground away from the panel,
// so the watcher has already closed it by the time the item is clicked, and
// Toggle's guard against that could swallow a click on a control that says
// "Open". A menu item with a verb on it should do the verb.
func (p *Panel) Open() {
	p.mu.Lock()
	h := p.hwnd
	if h == 0 {
		p.mu.Unlock()
		return
	}
	p.visible = true
	p.autoHidAt = time.Time{}
	p.mu.Unlock()

	p.showPanel(h)
}

// Dismiss hides the panel, for the in-panel close button.
func (p *Panel) Dismiss() {
	p.mu.Lock()
	h := p.hwnd
	p.visible = false
	p.mu.Unlock()
	p.hidePanel(h)
}

// Fades are short on purpose. This is a tray popover: anything slower than
// about a tenth of a second stops reading as polish and starts reading as the
// panel being slow to open.
const (
	fadeDuration = 110 * time.Millisecond
	fadeSteps    = 11
)

// showPanel places the panel, reveals it and fades it in.
func (p *Panel) showPanel(h uintptr) {
	AnchorToTray(h)

	// Transparent before Show, not after. Showing at full opacity and only
	// then starting the fade flashes the finished panel for a frame, which
	// is more jarring than no animation at all.
	SetAlpha(h, 0)
	p.mu.Lock()
	p.alpha = 0
	p.shownAt = time.Now()
	p.mu.Unlock()

	// Queued before the window is up: posts are drained on the next frame,
	// and the next frame is the first one the user sees.
	p.dispatch(p.resetToHome)

	Show(h)
	Focus(h)
	p.w.Invalidate()
	p.fadeTo(h, 255, nil)
	p.triggerPortProbe()
}

// hidePanel fades the panel out and then hides it.
func (p *Panel) hidePanel(h uintptr) {
	p.fadeTo(h, 0, func() { Hide(h) })
}

// fadeTo animates window opacity to target and then runs done.
//
// done runs only if the fade finished as the current one, which is what stops
// a superseded fade-out from hiding a window that has since been re-opened.
func (p *Panel) fadeTo(h uintptr, target uint8, done func()) {
	p.mu.Lock()
	p.fadeGen++
	gen := p.fadeGen
	from := p.alpha
	p.mu.Unlock()

	if from == target {
		if done != nil {
			done()
		}
		return
	}

	go func() {
		step := fadeDuration / fadeSteps
		for i := 1; i <= fadeSteps; i++ {
			time.Sleep(step)
			v := uint8(int(from) + (int(target)-int(from))*i/fadeSteps)

			p.mu.Lock()
			if p.fadeGen != gen {
				p.mu.Unlock()
				return
			}
			p.alpha = v
			p.mu.Unlock()

			SetAlpha(h, v)
		}
		p.mu.Lock()
		current := p.fadeGen == gen
		p.mu.Unlock()
		if current && done != nil {
			done()
		}
	}()
}

// resetToHome puts the panel back on its home screen. Runs on the Gio loop.
//
// A popover is not a document: coming back to a half-filled Add route form,
// or to the delete confirmation for a route that has since gone, is
// disorienting in a way that a full-size window is not.
func (p *Panel) resetToHome() {
	p.view = p.homeView()
	p.domainOpen = false
	p.pendingDelete = ""
}

// watchFocus closes the panel when the foreground moves elsewhere.
//
// Polling rather than a WNDPROC subclass. Gio does not surface window
// activation, and the alternative - hooking the window procedure - would run
// our code on Gio's own thread, which is precisely where every deadlock in
// this package has come from. A 150ms poll cannot wedge the message pump.
func (p *Panel) watchFocus() {
	const (
		tick = 150 * time.Millisecond
		// Long enough for the foreground to actually land on the panel after
		// Show; below about 250ms the watcher races the window it is
		// watching and closes it on the way up.
		grace = 350 * time.Millisecond
	)

	t := time.NewTicker(tick)
	defer t.Stop()
	for range t.C {
		p.mu.Lock()
		h, visible, shownAt := p.hwnd, p.visible, p.shownAt
		p.mu.Unlock()

		if h == 0 || !visible || time.Since(shownAt) < grace {
			continue
		}

		fg := Foreground()
		if fg == 0 || fg == h {
			continue
		}

		p.mu.Lock()
		p.visible = false
		p.autoHidAt = time.Now()
		p.mu.Unlock()

		dbg("foreground moved to %#x; closing panel", fg)
		p.hidePanel(h)
	}
}

// setHandle records the native handle, places the panel, then hides it.
//
// The order matters. Gio creates the window visible, and a window only pumps
// messages while it is visible - so this is the one moment we can reposition
// it synchronously and know the move has landed. Hiding first would queue the
// move against a sleeping thread, where it sits unapplied.
func (p *Panel) setHandle(h uintptr) {
	p.mu.Lock()
	p.hwnd = h
	p.visible = false
	p.mu.Unlock()

	AnchorToTray(h)
	Hide(h)

	// HideFromTaskbar must not run on this goroutine. SetWindowLongPtr SENDS
	// WM_STYLECHANGING to the window's owning thread and blocks until it is
	// handled - and this goroutine drives Gio's event loop, so blocking here
	// stalls the very pump the call waits on. Off-thread, the loop keeps
	// pumping and the call returns. The short delay lets the asynchronous
	// Hide land first, since the taskbar button is only dropped while hidden.
	go func() {
		// EnableFade first and without waiting: it writes the ex-style, so it
		// has to be off this goroutine, but nothing else has to land before
		// it - and until it runs, a very early tray click would open the
		// panel with no fade at all.
		EnableFade(h)

		time.Sleep(250 * time.Millisecond)
		// Same thread rule as HideFromTaskbar: anything that can make the
		// window repaint or restyle is kept off the goroutine driving Gio's
		// pump. Applied while hidden so the first Show is already correct.
		StyleWindow(h)
		HideFromTaskbar(h)
		dbg("panel parked; rect now %s", rectString(h))
	}()

	p.watchOnce.Do(func() { go p.watchFocus() })
}

// Loop runs the Gio event loop. It must run on its own goroutine, with
// app.Main() owning the main one.
func (p *Panel) Loop() error {
	var ops op.Ops
	for {
		e := p.w.Event()

		if h, ok := ViewHandle(e); ok && h != 0 {
			dbg("ViewHandle event hwnd=%#x", h)
			p.setHandle(h)
		}

		switch e := e.(type) {
		case app.DestroyEvent:
			p.stopAllQuick()
			return e.Err
		case app.FrameEvent:
			p.drainPosts()
			gtx := app.NewContext(&ops, e)
			p.handleInput(gtx)
			p.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (p *Panel) drainPosts() {
	for {
		select {
		case f := <-p.post:
			f()
		default:
			return
		}
	}
}

// handleInput processes button presses before the frame is laid out.
func (p *Panel) handleInput(gtx layout.Context) {
	// The close affordance exists on every screen except onboarding, which
	// has no exit until a mode is chosen.
	if p.view != viewOnboarding && p.closeBtn.Clicked(gtx) {
		p.Dismiss()
	}

	switch p.view {
	case viewOnboarding:
		for i := range p.choiceBtns {
			if p.choiceBtns[i].Clicked(gtx) {
				p.choice = i
			}
		}
		if p.continueBtn.Clicked(gtx) {
			if p.choice == 0 {
				p.tokenEd.SetText(p.settings.APIToken)
				p.view = viewTokenSetup
			} else {
				p.mode = modeQuick
				p.view = viewMain
			}
		}

	case viewTokenSetup, viewSettings:
		if p.docsSetupBtn.Clicked(gtx) {
			openURL(guideURL)
		}
		if p.openDashBtn.Clicked(gtx) {
			if err := openURL(dashboardTokenURL); err != nil {
				p.setStatus("Could not open the browser", colError)
			}
		}
		if p.cancelBtn.Clicked(gtx) {
			p.view = p.homeView()
		}
		if p.verifyBtn.Clicked(gtx) {
			p.startVerify(p.tokenEd.Text())
		}
		if p.autostartBtn.Clicked(gtx) {
			p.toggleAutostart()
		}

	case viewMain:
		p.handleMainInput(gtx)

	case viewConfirmDelete:
		if p.cancelBtn.Clicked(gtx) {
			p.pendingDelete = ""
			p.view = viewMain
		}
		if p.confirmBtn.Clicked(gtx) {
			p.deleteRoute(p.pendingDelete)
			p.pendingDelete = ""
			p.view = viewMain
		}

	case viewAddRoute:
		if p.cancelBtn.Clicked(gtx) {
			p.view = viewMain
		}
		if p.createBtn.Clicked(gtx) {
			if p.mode == modeQuick {
				port, err := parsePort(strings.TrimSpace(p.portEd.Text()))
				if err != nil {
					p.setStatus(err.Error(), colWarning)
				} else {
					p.startQuick(port)
				}
				break
			}
			if r, ok := p.buildRoute(); ok {
				p.routes = append(p.routes, r)
				// Not success yet: nothing has been published at this
				// point. Claiming it here is what left a green "Added ..."
				// sitting under a route that had already failed.
				p.setStatus("Publishing "+shortHost(r.Hostname)+"...", colWarning)
				p.view = viewMain
				p.saveConfig()
				// The whole point of connecting an API token is that this
				// needs no separate click: publish DNS and ingress now
				// rather than leaving the row as UI-only intent.
				p.provisionRoute(r.Hostname, r.Target)
				p.triggerPortProbe()
			}
		}
	}
}

func (p *Panel) handleMainInput(gtx layout.Context) {
	if p.addTokenBtn.Clicked(gtx) {
		p.tokenEd.SetText(p.settings.APIToken)
		p.view = viewTokenSetup
		return
	}

	for i := range p.segBtns {
		if p.segBtns[i].Clicked(gtx) {
			p.mode = i
			p.domainOpen = false
			p.triggerPortProbe()
		}
	}
	if p.domainBtn.Clicked(gtx) {
		p.domainOpen = !p.domainOpen
	}
	for i := range p.domainRows {
		if !p.domainRows[i].Clicked(gtx) || i >= len(p.zones) {
			continue
		}
		p.domainIx = i
		p.domainOpen = false
		p.saveConfig()
	}
	if p.settingsBtn.Clicked(gtx) {
		p.tokenEd.SetText(p.settings.APIToken)
		p.view = viewSettings
	}
	if p.docsBtn.Clicked(gtx) {
		if docsURL == "" {
			p.setStatus("No repository yet - docs link not set", colWarning)
		} else {
			openURL(docsURL)
		}
	}
	if p.addBtn.Clicked(gtx) || p.emptyAddBtn.Clicked(gtx) {
		p.startAdd()
	}

	if p.mode == modeDomains {
		for i := range p.routes {
			if p.routes[i].copyBtn.Clicked(gtx) {
				p.copyConn(gtx, "https://"+p.routes[i].Hostname, &p.routes[i].copiedAt)
			}
			if p.routes[i].stopBtn.Clicked(gtx) {
				// Deleting a route tears down a real DNS record and ingress
				// rule on the account - not just a row in a list - so it
				// asks first, and says what it is about to remove.
				p.pendingDelete = p.routes[i].Hostname
				p.view = viewConfirmDelete
				break
			}
		}
		return
	}

	for i := range p.quicks {
		if p.quicks[i].copyBtn.Clicked(gtx) && p.quicks[i].URL != "" {
			p.copyConn(gtx, p.quicks[i].URL, &p.quicks[i].copiedAt)
		}
		if p.quicks[i].stopBtn.Clicked(gtx) {
			p.stopQuick(i)
			break
		}
	}
}

// startAdd opens the right creation flow for the current list.
func (p *Panel) startAdd() {
	if p.mode == modeQuick {
		p.portEd.SetText("")
		p.startQuickPrompt()
		return
	}
	if !p.configured() {
		p.setStatus("Connect a domain before adding routes", colWarning)
		p.view = viewTokenSetup
		return
	}
	p.hostEd.SetText("")
	p.portEd.SetText("")
	p.view = viewAddRoute
}

// copyConn puts a URL on the clipboard and marks the pill as copied.
func (p *Panel) copyConn(gtx layout.Context, url string, stamp *time.Time) {
	writeClipboard(gtx, url)
	*stamp = time.Now()
	p.setStatus("Copied "+url, colSuccess)

	// Repaint once the confirmation should lapse; nothing else would wake the
	// loop while the panel sits idle.
	go func() {
		time.Sleep(copiedFor + 50*time.Millisecond)
		p.dispatch(func() {})
	}()
}

func (p *Panel) homeView() view {
	if p.configured() || len(p.quicks) > 0 {
		return viewMain
	}
	return viewOnboarding
}

// configured reports whether routes can be created: a token has verified and
// a zone is selected. Unlike the old typed-domain flow, this can never be
// true with a domain that doesn't actually exist on the account - zones only
// ever come from Cloudflare's own answer to Zones().
func (p *Panel) configured() bool {
	return p.settings.APIToken != "" && p.domainIx >= 0 && p.domainIx < len(p.zones)
}

// startVerify checks the token against Cloudflare and lists every zone it can
// reach.
//
// This replaces asking for a domain up front: the token is the only thing
// QuickFlare cannot derive on its own, and typing a domain by hand is exactly
// the class of mistake a live zone list exists to prevent.
//
// Zones are no longer screened by wildcard availability - QuickFlare claims one
// record per route rather than *.<zone>, so a zone being partly in use no
// longer disqualifies it. Availability is now a per-hostname question,
// answered when a route is actually created.
func (p *Panel) startVerify(token string) {
	if p.verifying {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" {
		p.setStatus("Paste your Cloudflare API token", colWarning)
		return
	}

	p.verifying = true
	p.setStatus("Verifying token...", colWarning)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		client := cloudflare.New(token, "")
		if _, err := client.VerifyToken(ctx); err != nil {
			msg := "Could not verify token"
			if cloudflare.IsUnauthorized(err) {
				msg = "Token was rejected - check it was copied in full"
			}
			p.dispatch(func() {
				p.verifying = false
				p.setStatus(msg, colError)
			})
			return
		}

		zones, err := client.Zones(ctx, "")
		if err != nil {
			p.dispatch(func() {
				p.verifying = false
				p.setStatus("Token verified, but listing zones failed: "+err.Error(), colError)
			})
			return
		}
		if len(zones) == 0 {
			p.dispatch(func() {
				p.verifying = false
				p.setStatus("Token verified, but it has no zones - add a domain to Cloudflare first", colWarning)
			})
			return
		}

		// A zone's own record already names its account, and a personal
		// token is typically scoped to one - so there is no need for a
		// separate account lookup or picker.
		client.SetAccountID(zones[0].Account.ID)

		opts := make([]zoneOption, len(zones))
		for i, z := range zones {
			opts[i] = zoneOption{Name: z.Name, ID: z.ID, AccountID: z.Account.ID}
		}

		p.dispatch(func() {
			p.verifying = false
			p.cf = client
			p.settings.APIToken = token
			p.zones = opts
			p.domainIx = p.pickZone(opts)
			p.mode = modeDomains
			p.setStatus(fmt.Sprintf("Connected - %d domain(s) found", len(opts)), colSuccess)
			p.view = viewMain
			p.saveConfig()
			// A verified token is the first point at which a tunnel can be
			// created, and nothing can be published before one exists.
			p.ensureTunnel()
		})
	}()
}

// pickZone chooses which zone to land on after a verify: the one in use last
// time if it is still reachable, otherwise the first one listed.
func (p *Panel) pickZone(opts []zoneOption) int {
	if p.savedDomain != "" {
		for i, o := range opts {
			if o.Name == p.savedDomain {
				return i
			}
		}
	}
	return 0
}

// saveConfig persists the token and selected zone. A failure here is worth
// saying out loud - silently forgetting the token every launch is exactly
// the behaviour this replaced.
func (p *Panel) saveConfig() {
	p.savedDomain = p.currentDomain()
	cfg := &config.Config{
		APIToken: p.settings.APIToken,
		Domain:   p.savedDomain,
		Routes:   p.storedRoutes(),
	}
	if err := cfg.Save(); err != nil {
		dbg("config save failed: %v", err)
		p.setStatus("Could not save settings: "+err.Error(), colWarning)
	}
}

// buildRoute turns the add-route form into a Route, or reports it is not ready.
func (p *Panel) buildRoute() (Route, bool) {
	if !p.configured() {
		p.setStatus("Connect a domain before adding routes", colWarning)
		return Route{}, false
	}

	host := strings.TrimSpace(p.hostEd.Text())
	port := strings.TrimSpace(p.portEd.Text())
	if host == "" || port == "" {
		p.setStatus("Subdomain and port are both required", colWarning)
		return Route{}, false
	}
	if err := validateLabel(host); err != nil {
		p.setStatus(err.Error(), colWarning)
		return Route{}, false
	}

	hostname := host + "." + p.currentDomain()
	// Two rows publishing the same hostname fight over one DNS record and
	// one ingress rule - and deleting either would tear down what the other
	// still depends on.
	for i := range p.routes {
		if strings.EqualFold(p.routes[i].Hostname, hostname) {
			p.setStatus(hostname+" is already in this list", colWarning)
			return Route{}, false
		}
	}
	if _, err := parsePort(port); err != nil {
		p.setStatus(err.Error(), colWarning)
		return Route{}, false
	}

	return Route{Route: core.Route{
		Hostname: hostname,
		Target:   "localhost:" + port,
		Status:   statusStarting,
		ZoneID:   p.currentZoneID(),
	}}, true
}

// deleteRoute tears the route down on Cloudflare and drops it from the list.
//
// The row goes immediately rather than waiting on the API: teardown is
// best-effort and reports leftovers in the status bar, and a row still
// sitting there after the user confirmed reads as the click not working.
func (p *Panel) deleteRoute(hostname string) {
	for i := range p.routes {
		if p.routes[i].Hostname != hostname {
			continue
		}
		r := p.routes[i]
		p.routes = append(p.routes[:i], p.routes[i+1:]...)
		p.setStatus("Removing "+hostname+"...", colWarning)
		p.saveConfig()
		p.deprovisionRoute(r)
		return
	}
}

// shortHost is the leading label of a hostname, for status messages where
// the full name is both too long for the strip and already visible in the
// row being talked about.
func shortHost(hostname string) string {
	if i := strings.IndexByte(hostname, '.'); i > 0 {
		return hostname[:i]
	}
	return hostname
}

// routeByHostname finds a route for display, or nil if it is gone.
func (p *Panel) routeByHostname(hostname string) *Route {
	for i := range p.routes {
		if p.routes[i].Hostname == hostname {
			return &p.routes[i]
		}
	}
	return nil
}

// setRouteStatus updates one route by hostname. Routes are addressed by name
// rather than index because this runs after a round trip to Cloudflare, by
// which point the route could have been removed or the slice reordered -
// looking it up again is what keeps a stale index from silently touching the
// wrong row.
func (p *Panel) setRouteStatus(hostname string, status connStatus, detail string) {
	for i := range p.routes {
		if p.routes[i].Hostname != hostname {
			continue
		}
		p.routes[i].Status = status
		p.routes[i].Detail = detail

		// Keep the footer honest, and short. It previously kept whatever
		// optimistic message was set when the route was added, so a failed
		// route sat under a green success line contradicting its own row -
		// and then repeated the hostname the row already shows, next to a
		// raw API error, in a strip one line tall.
		switch status {
		case statusError:
			p.setStatus(shortHost(hostname)+" failed", colError)
		case statusConnected:
			p.setStatus(shortHost(hostname)+" is live", colSuccess)
		}
		return
	}
}

func (p *Panel) currentDomain() string {
	if p.domainIx >= 0 && p.domainIx < len(p.zones) {
		return p.zones[p.domainIx].Name
	}
	return ""
}

// startQuickPrompt reuses the add-route screen for the port-only quick form.
func (p *Panel) startQuickPrompt() {
	p.view = viewAddRoute
}

// startQuick opens an anonymous tunnel on a background goroutine. It can take
// several seconds for cloudflared to report a URL, so the entry appears
// immediately in a starting state and fills in when it lands.
func (p *Panel) startQuick(port int) {
	if p.binErr != nil {
		p.setStatus("cloudflared not found on this machine", colError)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry := &QuickEntry{Port: port, Status: statusStarting, cancel: cancel}
	p.quicks = append(p.quicks, entry)
	p.addCloser(entry.close)
	p.mode = modeQuick
	p.setStatus("Opening a quick tunnel...", colWarning)
	p.view = viewMain

	p.triggerPortProbe()

	binPath := p.binPath
	go func() {
		q, err := supervisor.StartQuick(ctx, binPath, port)
		p.dispatch(func() {
			if err != nil {
				entry.Status = statusError
				entry.Err = err.Error()
				p.setStatus("Quick tunnel failed", colError)
				return
			}
			entry.tunnel = q
			entry.URL = q.URL()
			entry.Status = statusConnected
			p.setStatus("Quick tunnel open", colSuccess)
			p.triggerPortProbe()
		})
	}()
}

// stopQuick closes one tunnel and drops it from the list. Killing the process
// is the whole revocation story for an anonymous link.
func (p *Panel) stopQuick(i int) {
	if i < 0 || i >= len(p.quicks) {
		return
	}
	p.quicks[i].close()
	p.quicks = append(p.quicks[:i], p.quicks[i+1:]...)
	p.setStatus("Quick tunnel closed", colTextDisabled)
}

// close tears the tunnel down whether or not it finished starting.
func (q *QuickEntry) close() {
	if q.cancel != nil {
		q.cancel()
	}
	if q.tunnel != nil {
		q.tunnel.Close()
	}
}

func (p *Panel) stopAllQuick() {
	for _, q := range p.quicks {
		q.close()
	}
	p.quicks = nil
}

// addCloser registers a teardown func to run at exit.
func (p *Panel) addCloser(f func()) {
	p.closersMu.Lock()
	p.closers = append(p.closers, f)
	p.closersMu.Unlock()
}

// Shutdown stops every cloudflared process QuickFlare started.
//
// Safe to call more than once and from any goroutine: it is reached both
// from the tray's exit handler and from the window closing, and whichever
// arrives second must not panic or double-kill. Each teardown is itself
// idempotent, so a process already stopped from the UI is simply a no-op
// here.
func (p *Panel) Shutdown() {
	p.shutdownOnce.Do(func() {
		p.closersMu.Lock()
		closers := p.closers
		p.closers = nil
		p.closersMu.Unlock()

		for _, f := range closers {
			f()
		}
	})
}

// isPortListening returns whether a local service is listening on port, and
// whether that port has been probed at least once.
func (p *Panel) isPortListening(port int) (listening bool, checked bool) {
	if port <= 0 {
		return false, false
	}
	p.portsMu.RLock()
	defer p.portsMu.RUnlock()
	if p.portsListening == nil {
		return false, false
	}
	listening, checked = p.portsListening[port]
	return listening, checked
}

// triggerPortProbe checks all configured routes and active quick tunnels
// asynchronously on-demand (e.g. when panel is opened/focused, or when
// routes/quicks are added). It dials each port once with a short timeout.
func (p *Panel) triggerPortProbe() {
	p.mu.Lock()
	var ports []int
	seen := make(map[int]bool)
	for i := range p.routes {
		port := extractPort(p.routes[i].Target)
		if port > 0 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	for i := range p.quicks {
		port := p.quicks[i].Port
		if port > 0 && !seen[port] {
			seen[port] = true
			ports = append(ports, port)
		}
	}
	p.mu.Unlock()

	if len(ports) == 0 {
		return
	}

	go func() {
		results := make(map[int]bool, len(ports))
		for _, port := range ports {
			results[port] = checkPortListening(port)
		}

		p.portsMu.Lock()
		changed := false
		if p.portsListening == nil {
			p.portsListening = make(map[int]bool)
		}
		for port, listening := range results {
			if cur, ok := p.portsListening[port]; !ok || cur != listening {
				p.portsListening[port] = listening
				changed = true
			}
		}
		p.portsMu.Unlock()

		if changed {
			p.dispatch(func() {})
		}
	}()
}

// parsePort and validateLabel forward to core so the CLI and the panel
// reject exactly the same input.
func parsePort(s string) (int, error) { return core.ParsePort(s) }

func validateLabel(label string) error { return core.ValidateLabel(label) }

// statusText is the footer message, falling back to a summary of the session.
func (p *Panel) statusText() string {
	if p.status != "" {
		return p.status
	}
	n := 0
	for _, q := range p.quicks {
		if q.Status == statusConnected {
			n++
		}
	}
	for _, r := range p.routes {
		if r.Status == statusConnected {
			n++
		}
	}
	switch {
	case n == 1:
		return "1 tunnel connected"
	case n > 1:
		return fmt.Sprintf("%d tunnels connected", n)
	case p.configured():
		return p.currentDomain()
	}
	return "Not configured"
}

func (p *Panel) layout(gtx layout.Context) layout.Dimensions {
	// Flat, not rounded. The window is rectangular and DWM already rounds and
	// clips it (see StyleWindow); painting a second, smaller radius here left
	// the corners between the two shapes unpainted. Fill the whole surface and
	// let the compositor decide where the edge is.
	paint.FillShape(gtx.Ops, colBackground,
		clip.Rect{Max: gtx.Constraints.Max}.Op())

	switch p.view {
	case viewOnboarding:
		return p.onboardingView(gtx)
	case viewTokenSetup, viewSettings:
		return p.tokenSetupView(gtx)
	case viewAddRoute:
		return p.addRouteView(gtx)
	case viewConfirmDelete:
		return p.confirmDeleteView(gtx)
	default:
		return p.mainView(gtx)
	}
}
