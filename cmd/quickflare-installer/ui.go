package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/San-Shiro/QuickFlare/internal/installer"
)

//go:embed assets/fonts/Inter.ttf
var fontInter []byte

//go:embed assets/fonts/JetBrainsMono.ttf
var fontJetBrainsMono []byte

//go:embed assets/logo.png
var logoPNG []byte

var (
	colBg              = hex(0x16181D)
	colSurface         = hex(0x1F232B)
	colSurfaceHover    = hex(0x282D37)
	colSurfaceSelected = hex(0x232934)
	colSurfaceRaised   = hex(0x2C323D)
	colBorderSubtle    = hex(0x2E3440)
	colBorderSelected  = hex(0xF6821F)
	colAccent          = hex(0xF6821F)
	colAccentHover     = hex(0xFA9338)
	colAccentPressed   = hex(0xD86D14)
	colTextPrimary     = hex(0xF4F5F6)
	colTextMuted       = hex(0x9AA1A8)
	colTextDim         = hex(0x606771)
	colSuccess         = hex(0x4CC38A)
	colError           = hex(0xE5484D)
	colErrorSubtle     = hex(0x3B1F22)
)

const faceMono font.Typeface = "JetBrains Mono"

func hex(v uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 0xFF,
	}
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

type AppState int

const (
	StateChoose AppState = iota
	StateInstalling
	StateInstalled
	StateUninstallPrompt
	StateUninstalling
	StateUninstalled
	StateError
)

type InstallerUI struct {
	w       *app.Window
	th      *material.Theme
	logoOp  paint.ImageOp
	icCheck *widget.Icon
	icError *widget.Icon
	state   AppState

	// Configuration & Options
	installTray     bool
	addToPath       bool
	createShortcuts bool
	launchAfter     bool
	removeRoutes    bool
	removeConfig    bool
	targetDir       string

	// Interactive clickables
	cardTrayClick    widget.Clickable
	cardCliClick     widget.Clickable
	pathCheckClick   widget.Clickable
	shortCheckClick  widget.Clickable
	launchCheckClick widget.Clickable
	routesCheckClick widget.Clickable
	configCheckClick widget.Clickable

	installBtn   widget.Clickable
	cancelBtn    widget.Clickable
	finishBtn    widget.Clickable
	uninstallBtn widget.Clickable
	keepBtn      widget.Clickable
	retryBtn     widget.Clickable
	closeBtn     widget.Clickable

	// Progress state
	mu         sync.Mutex
	progress   float32
	statusStep string
	errMsg     string

	// Post channel to serialize state updates onto the Gio event loop
	post chan func()
}

func loadFonts() []font.FontFace {
	var collection []font.FontFace
	fInter, errInter := opentype.Parse(fontInter)
	fMono, errMono := opentype.Parse(fontJetBrainsMono)

	if errInter == nil {
		for _, tf := range []font.Typeface{"", "Inter", "sans-serif", "Go"} {
			collection = append(collection,
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Normal}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Medium}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.SemiBold}, Face: fInter},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Bold}, Face: fInter},
			)
		}
	}
	if errMono == nil {
		for _, tf := range []font.Typeface{faceMono, "monospace", "JetBrains Mono"} {
			collection = append(collection,
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Normal}, Face: fMono},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.Medium}, Face: fMono},
				font.FontFace{Font: font.Font{Typeface: tf, Weight: font.SemiBold}, Face: fMono},
			)
		}
	}
	collection = append(collection, gofont.Collection()...)
	return collection
}

func NewInstallerUI(initialState AppState) *InstallerUI {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(loadFonts()))
	th.Palette = material.Palette{
		Bg:         colBg,
		Fg:         colTextPrimary,
		ContrastBg: colAccent,
		ContrastFg: colBg,
	}

	ui := &InstallerUI{
		th:              th,
		state:           initialState,
		installTray:     true, // Default: CLI with System Tray (strict: no tray-only option)
		addToPath:       true,
		createShortcuts: true,
		launchAfter:     true,
		removeRoutes:    true, // Unpublish active routes on uninstall by default
		removeConfig:    true, // Remove configs on uninstall by default
		targetDir:       installer.DefaultInstallDir(),
		post:            make(chan func(), 32),
	}

	if img, _, err := image.Decode(bytes.NewReader(logoPNG)); err == nil {
		ui.logoOp = paint.NewImageOp(img)
	}

	if ic, err := widget.NewIcon(icons.NavigationCheck); err == nil {
		ui.icCheck = ic
	}
	if ic, err := widget.NewIcon(icons.AlertErrorOutline); err == nil {
		ui.icError = ic
	}

	return ui
}

func (ui *InstallerUI) SetWindow(w *app.Window) {
	ui.w = w
}

func (ui *InstallerUI) postUpdate(f func()) {
	select {
	case ui.post <- f:
		if ui.w != nil {
			ui.w.Invalidate()
		}
	default:
		go func() {
			ui.post <- f
			if ui.w != nil {
				ui.w.Invalidate()
			}
		}()
	}
}

func (ui *InstallerUI) drainPosts() {
	for {
		select {
		case f := <-ui.post:
			f()
		default:
			return
		}
	}
}

func (ui *InstallerUI) Layout(gtx layout.Context) layout.Dimensions {
	ui.drainPosts()

	// Handle clicks
	if ui.state == StateChoose {
		if ui.cardTrayClick.Clicked(gtx) {
			ui.installTray = true
		}
		if ui.cardCliClick.Clicked(gtx) {
			ui.installTray = false
		}
		if ui.pathCheckClick.Clicked(gtx) {
			ui.addToPath = !ui.addToPath
		}
		if ui.shortCheckClick.Clicked(gtx) && ui.installTray {
			ui.createShortcuts = !ui.createShortcuts
		}
		if ui.launchCheckClick.Clicked(gtx) && ui.installTray {
			ui.launchAfter = !ui.launchAfter
		}
		if ui.cancelBtn.Clicked(gtx) {
			os.Exit(0)
		}
		if ui.installBtn.Clicked(gtx) {
			ui.startInstall()
		}
	} else if ui.state == StateUninstallPrompt {
		if ui.routesCheckClick.Clicked(gtx) {
			ui.removeRoutes = !ui.removeRoutes
		}
		if ui.configCheckClick.Clicked(gtx) {
			ui.removeConfig = !ui.removeConfig
		}
		if ui.keepBtn.Clicked(gtx) {
			os.Exit(0)
		}
		if ui.uninstallBtn.Clicked(gtx) {
			ui.startUninstall()
		}
	} else if ui.state == StateInstalled {
		if ui.finishBtn.Clicked(gtx) {
			if ui.launchAfter && ui.installTray {
				trayExe := filepath.Join(ui.targetDir, "quickflare-tray.exe")
				_ = exec.Command(trayExe).Start()
			}
			os.Exit(0)
		}
	} else if ui.state == StateUninstalled || ui.state == StateError {
		if ui.closeBtn.Clicked(gtx) {
			os.Exit(0)
		}
		if ui.retryBtn.Clicked(gtx) {
			ui.state = StateChoose
		}
	}

	// Paint background
	paint.FillShape(gtx.Ops, colBg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	// Outer padding
	return layout.UniformInset(unit.Dp(20)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		switch ui.state {
		case StateChoose:
			return ui.layoutChoose(gtx)
		case StateInstalling:
			return ui.layoutInstalling(gtx)
		case StateInstalled:
			return ui.layoutInstalled(gtx)
		case StateUninstallPrompt:
			return ui.layoutUninstallPrompt(gtx)
		case StateUninstalling:
			return ui.layoutUninstalling(gtx)
		case StateUninstalled:
			return ui.layoutUninstalled(gtx)
		case StateError:
			return ui.layoutError(gtx)
		default:
			return layout.Dimensions{}
		}
	})
}

// ---------- View Layouts ----------

func (ui *InstallerUI) layoutChoose(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		// Header
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "QuickFlare Setup", "Select your installation experience and options")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		// Experience selection cards
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSectionTitle(gtx, "INSTALLATION EXPERIENCE")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Card 1: CLI with System Tray
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCard(gtx, &ui.cardTrayClick, ui.installTray,
				"CLI with System Tray", "(Recommended)",
				"Includes the background tray popover, live route status, and the quickflare CLI tool.")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		// Card 2: CLI Only
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCard(gtx, &ui.cardCliClick, !ui.installTray,
				"CLI Only", "",
				"Lightweight standalone command-line tool. No system tray service or GUI popover.")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),

		// Options
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSectionTitle(gtx, "CONFIGURATION OPTIONS")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCheckbox(gtx, &ui.pathCheckClick, ui.addToPath, true,
				"Add QuickFlare to User PATH", "Enables running 'quickflare' directly in CMD and PowerShell")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCheckbox(gtx, &ui.shortCheckClick, ui.createShortcuts && ui.installTray, ui.installTray,
				"Create Start Menu shortcuts", "Add QuickFlare and Uninstaller to Start Menu")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCheckbox(gtx, &ui.launchCheckClick, ui.launchAfter && ui.installTray, ui.installTray,
				"Launch QuickFlare after installation", "Start the tray app immediately upon completion")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),

		// Target Directory preview
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderInstallPathBox(gtx)
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),

		// Footer Buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonSecondary(gtx, &ui.cancelBtn, "Cancel")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonPrimary(gtx, &ui.installBtn, "Install QuickFlare")
				}),
			)
		}),
	)
}

func (ui *InstallerUI) layoutInstalling(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	prog := ui.progress
	step := ui.statusStep
	ui.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "Installing QuickFlare", "Please wait while QuickFlare is configured on your system")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return spinner(gtx, 40, colAccent)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(13.5), step)
						l.Color = colTextPrimary
						l.Font.Weight = font.SemiBold
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.renderProgressBar(gtx, prog, 360)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						pct := fmt.Sprintf("%d%%", int(prog*100))
						l := material.Label(ui.th, unit.Sp(11), pct)
						l.Color = colTextMuted
						l.Font.Typeface = faceMono
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
				)
			})
		}),
	)
}

func (ui *InstallerUI) layoutInstalled(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "Installation Complete", "QuickFlare is now ready to use on your PC")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		// Success Hero Box
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSurfaceBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								sz := gtx.Dp(unit.Dp(28))
								gtx.Constraints = layout.Exact(image.Pt(sz, sz))
								if ui.icCheck != nil {
									return ui.icCheck.Layout(gtx, colSuccess)
								}
								return layout.Dimensions{}
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Label(ui.th, unit.Sp(14), "Successfully installed QuickFlare")
								l.Color = colTextPrimary
								l.Font.Weight = font.SemiBold
								return l.Layout(gtx)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(12), "Installed location:")
						l.Color = colTextMuted
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(11.5), ui.targetDir)
						l.Color = colTextPrimary
						l.Font.Typeface = faceMono
						return l.Layout(gtx)
					}),
				)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),

		// CLI Quick Tip Box
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSurfaceBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(12), "Command Line Usage:")
						l.Color = colAccent
						l.Font.Weight = font.SemiBold
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(11), "quickflare up 3000          # publish localhost:3000 instantly")
						l.Color = colTextPrimary
						l.Font.Typeface = faceMono
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(11), "quickflare status           # inspect tunnels and routes")
						l.Color = colTextPrimary
						l.Font.Typeface = faceMono
						return l.Layout(gtx)
					}),
				)
			})
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),

		// Footer
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			btnText := "Finish"
			if ui.launchAfter && ui.installTray {
				btnText = "Launch QuickFlare & Finish"
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonPrimary(gtx, &ui.finishBtn, btnText)
				}),
			)
		}),
	)
}

func (ui *InstallerUI) layoutUninstallPrompt(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "Uninstall QuickFlare", "Cleanly remove QuickFlare from your system")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSurfaceBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(13), "QuickFlare will be removed from:")
						l.Color = colTextMuted
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(11.5), ui.targetDir)
						l.Color = colTextPrimary
						l.Font.Typeface = faceMono
						return l.Layout(gtx)
					}),
				)
			})
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSectionTitle(gtx, "CLEANUP OPTIONS")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCheckbox(gtx, &ui.routesCheckClick, ui.removeRoutes, true,
				"Unpublish active Cloudflare routes via API",
				"Gracefully removes DNS CNAME records and tunnel ingress routes from your Cloudflare account")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderCheckbox(gtx, &ui.configCheckClick, ui.removeConfig, true,
				"Remove configuration and saved credentials",
				"Deletes %APPDATA%\\QuickFlare containing settings and saved API tokens")
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),

		// Footer Buttons
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonSecondary(gtx, &ui.keepBtn, "Keep QuickFlare")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonDanger(gtx, &ui.uninstallBtn, "Uninstall QuickFlare")
				}),
			)
		}),
	)
}

func (ui *InstallerUI) layoutUninstalling(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	prog := ui.progress
	step := ui.statusStep
	ui.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "Uninstalling QuickFlare", "Please wait while components are cleanly removed")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return spinner(gtx, 40, colError)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(13.5), step)
						l.Color = colTextPrimary
						l.Font.Weight = font.SemiBold
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return ui.renderProgressBar(gtx, prog, 360)
					}),
				)
			})
		}),
	)
}

func (ui *InstallerUI) layoutUninstalled(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "QuickFlare Removed", "Uninstallation finished successfully")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSurfaceBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(13), "All QuickFlare binaries, PATH environment variables, and shortcuts have been removed.")
						l.Color = colTextPrimary
						return l.Layout(gtx)
					}),
				)
			})
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonSecondary(gtx, &ui.closeBtn, "Close")
				}),
			)
		}),
	)
}

func (ui *InstallerUI) layoutError(gtx layout.Context) layout.Dimensions {
	ui.mu.Lock()
	errTxt := ui.errMsg
	ui.mu.Unlock()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderHeader(gtx, "Setup Error", "An error occurred during operation")
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(20)}.Layout),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return ui.renderSurfaceBox(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(13), "Error details:")
						l.Color = colError
						l.Font.Weight = font.SemiBold
						return l.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(11.5), errTxt)
						l.Color = colTextPrimary
						return l.Layout(gtx)
					}),
				)
			})
		}),

		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),

		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonSecondary(gtx, &ui.closeBtn, "Close")
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.buttonPrimary(gtx, &ui.retryBtn, "Back / Retry")
				}),
			)
		}),
	)
}

// ---------- Operations ----------

func (ui *InstallerUI) startInstall() {
	ui.state = StateInstalling
	ui.progress = 0.05
	ui.statusStep = "Starting installation..."

	opt := installer.Options{
		InstallDir:      ui.targetDir,
		InstallTray:     ui.installTray,
		AddToPath:       ui.addToPath,
		CreateShortcuts: ui.createShortcuts && ui.installTray,
		LaunchAfter:     false, // We launch after on finish click to avoid races
	}

	go func() {
		ctx := context.Background()
		err := installer.Install(ctx, opt, func(step string, prog float32) {
			ui.postUpdate(func() {
				ui.mu.Lock()
				ui.statusStep = step
				ui.progress = prog
				ui.mu.Unlock()
			})
		})

		ui.postUpdate(func() {
			if err != nil {
				ui.mu.Lock()
				ui.errMsg = err.Error()
				ui.mu.Unlock()
				ui.state = StateError
			} else {
				ui.state = StateInstalled
			}
		})
	}()
}

func (ui *InstallerUI) startUninstall() {
	ui.state = StateUninstalling
	ui.progress = 0.05
	ui.statusStep = "Preparing uninstallation..."

	opt := installer.Options{
		InstallDir:   ui.targetDir,
		RemoveRoutes: ui.removeRoutes,
		RemoveConfig: ui.removeConfig,
	}

	go func() {
		ctx := context.Background()
		err := installer.Uninstall(ctx, opt, func(step string, prog float32) {
			ui.postUpdate(func() {
				ui.mu.Lock()
				ui.statusStep = step
				ui.progress = prog
				ui.mu.Unlock()
			})
		})

		ui.postUpdate(func() {
			if err != nil {
				ui.mu.Lock()
				ui.errMsg = err.Error()
				ui.mu.Unlock()
				ui.state = StateError
			} else {
				ui.state = StateUninstalled
			}
		})
	}()
}

// ---------- UI Component Helpers ----------

func (ui *InstallerUI) renderHeader(gtx layout.Context, title, subtitle string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ui.logoOp.Size().X > 0 {
				d := gtx.Dp(unit.Dp(36))
				gtx.Constraints = layout.Exact(image.Pt(d, d))
				rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(d, d)}, gtx.Dp(unit.Dp(8)))
				p := rr.Push(gtx.Ops)
				sz := ui.logoOp.Size()
				scaleOp := op.Affine(f32.Affine2D{}.Scale(
					f32.Pt(0, 0),
					f32.Pt(float32(d)/float32(sz.X), float32(d)/float32(sz.Y)),
				)).Push(gtx.Ops)
				ui.logoOp.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				scaleOp.Pop()
				p.Pop()
				return layout.Dimensions{Size: image.Pt(d, d)}
			}
			return layout.Dimensions{}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							l := material.Label(ui.th, unit.Sp(17), title)
							l.Color = colTextPrimary
							l.Font.Weight = font.Bold
							return l.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.renderPill(gtx, "v"+installer.Version, colAccent, withAlpha(colAccent, 28))
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Label(ui.th, unit.Sp(11.5), subtitle)
					l.Color = colTextMuted
					return l.Layout(gtx)
				}),
			)
		}),
	)
}

func (ui *InstallerUI) renderSectionTitle(gtx layout.Context, text string) layout.Dimensions {
	l := material.Label(ui.th, unit.Sp(10.5), text)
	l.Color = colTextDim
	l.Font.Weight = font.Bold
	return l.Layout(gtx)
}

func (ui *InstallerUI) renderCard(gtx layout.Context, c *widget.Clickable, selected bool, title, badge, desc string) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colSurface
		borderColor := colBorderSubtle
		borderWidth := unit.Dp(1)

		if selected {
			bg = colSurfaceSelected
			borderColor = colBorderSelected
			borderWidth = unit.Dp(1.5)
		} else if c.Hovered() {
			bg = colSurfaceHover
		}

		return widget.Border{
			Color:        borderColor,
			CornerRadius: unit.Dp(9),
			Width:        borderWidth,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := op.Record(gtx.Ops)
			dims := layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					// Custom Radio Indicator
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						sz := gtx.Dp(unit.Dp(18))
						gtx.Constraints = layout.Exact(image.Pt(sz, sz))
						radioBg := colSurfaceRaised
						if selected {
							radioBg = colAccent
						}
						// Outer ring
						rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(sz, sz)}, sz/2)
						paint.FillShape(gtx.Ops, radioBg, rr.Op(gtx.Ops))

						// Inner circle
						if selected {
							innerSz := gtx.Dp(unit.Dp(8))
							innerOffset := (sz - innerSz) / 2
							innerRect := image.Rectangle{
								Min: image.Pt(innerOffset, innerOffset),
								Max: image.Pt(innerOffset+innerSz, innerOffset+innerSz),
							}
							innerRR := clip.UniformRRect(innerRect, innerSz/2)
							paint.FillShape(gtx.Ops, colBg, innerRR.Op(gtx.Ops))
						} else {
							innerSz := gtx.Dp(unit.Dp(14))
							innerOffset := (sz - innerSz) / 2
							innerRect := image.Rectangle{
								Min: image.Pt(innerOffset, innerOffset),
								Max: image.Pt(innerOffset+innerSz, innerOffset+innerSz),
							}
							innerRR := clip.UniformRRect(innerRect, innerSz/2)
							paint.FillShape(gtx.Ops, colSurface, innerRR.Op(gtx.Ops))
						}
						return layout.Dimensions{Size: image.Pt(sz, sz)}
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										l := material.Label(ui.th, unit.Sp(13.5), title)
										l.Color = colTextPrimary
										l.Font.Weight = font.SemiBold
										return l.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if badge != "" {
											return ui.renderPill(gtx, badge, colAccent, withAlpha(colAccent, 24))
										}
										return layout.Dimensions{}
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := material.Label(ui.th, unit.Sp(11), desc)
								l.Color = colTextMuted
								return l.Layout(gtx)
							}),
						)
					}),
				)
			})
			call := macro.Stop()

			fillRRect(gtx, dims.Size, 9, bg)
			call.Add(gtx.Ops)
			return dims
		})
	})
}

func (ui *InstallerUI) renderCheckbox(gtx layout.Context, c *widget.Clickable, checked, enabled bool, title, desc string) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Dp(unit.Dp(16))
				gtx.Constraints = layout.Exact(image.Pt(sz, sz))
				boxBg := colSurfaceRaised
				if !enabled {
					boxBg = withAlpha(colSurfaceRaised, 100)
				} else if checked {
					boxBg = colAccent
				}
				rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(sz, sz)}, gtx.Dp(unit.Dp(4)))
				paint.FillShape(gtx.Ops, boxBg, rr.Op(gtx.Ops))

				if checked {
					pad := gtx.Dp(unit.Dp(2))
					gtx.Constraints = layout.Exact(image.Pt(sz-pad*2, sz-pad*2))
					if ui.icCheck != nil {
						return layout.Inset{Left: unit.Dp(2), Top: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return ui.icCheck.Layout(gtx, colBg)
						})
					}
				}
				return layout.Dimensions{Size: image.Pt(sz, sz)}
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(12), title)
						if enabled {
							l.Color = colTextPrimary
						} else {
							l.Color = colTextDim
						}
						l.Font.Weight = font.Medium
						return l.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						l := material.Label(ui.th, unit.Sp(10.5), desc)
						if enabled {
							l.Color = colTextMuted
						} else {
							l.Color = colTextDim
						}
						return l.Layout(gtx)
					}),
				)
			}),
		)
	})
}

func (ui *InstallerUI) renderInstallPathBox(gtx layout.Context) layout.Dimensions {
	return widget.Border{
		Color:        colBorderSubtle,
		CornerRadius: unit.Dp(8),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					l := material.Label(ui.th, unit.Sp(11), "Destination:")
					l.Color = colTextMuted
					return l.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					l := material.Label(ui.th, unit.Sp(11), ui.targetDir)
					l.Color = colTextPrimary
					l.Font.Typeface = faceMono
					return l.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()

		fillRRect(gtx, dims.Size, 8, colSurface)
		call.Add(gtx.Ops)
		return dims
	})
}

func (ui *InstallerUI) renderSurfaceBox(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return widget.Border{
		Color:        colBorderSubtle,
		CornerRadius: unit.Dp(9),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(unit.Dp(12)).Layout(gtx, content)
		call := macro.Stop()

		fillRRect(gtx, dims.Size, 9, colSurface)
		call.Add(gtx.Ops)
		return dims
	})
}

func (ui *InstallerUI) renderPill(gtx layout.Context, textStr string, fg, bg color.NRGBA) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{
		Left: unit.Dp(7), Right: unit.Dp(7),
		Top: unit.Dp(2), Bottom: unit.Dp(2),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Label(ui.th, unit.Sp(10), textStr)
		l.Color = fg
		l.Font.Weight = font.Bold
		return l.Layout(gtx)
	})
	call := macro.Stop()

	fillRRect(gtx, dims.Size, 6, bg)
	call.Add(gtx.Ops)
	return dims
}

func (ui *InstallerUI) renderProgressBar(gtx layout.Context, prog float32, widthDp int) layout.Dimensions {
	w := gtx.Dp(unit.Dp(widthDp))
	h := gtx.Dp(unit.Dp(8))
	if prog < 0 {
		prog = 0
	}
	if prog > 1 {
		prog = 1
	}

	gtx.Constraints = layout.Exact(image.Pt(w, h))

	// Track
	rrTrack := clip.UniformRRect(image.Rectangle{Max: image.Pt(w, h)}, h/2)
	paint.FillShape(gtx.Ops, colSurfaceRaised, rrTrack.Op(gtx.Ops))

	// Filled portion
	fillW := int(float32(w) * prog)
	if fillW > 0 {
		rrFill := clip.UniformRRect(image.Rectangle{Max: image.Pt(fillW, h)}, h/2)
		paint.FillShape(gtx.Ops, colAccent, rrFill.Op(gtx.Ops))
	}

	return layout.Dimensions{Size: image.Pt(w, h)}
}

func (ui *InstallerUI) buttonPrimary(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colAccent
		if c.Pressed() {
			bg = colAccentPressed
		} else if c.Hovered() {
			bg = colAccentHover
		}
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Left: unit.Dp(18), Right: unit.Dp(18),
			Top: unit.Dp(9), Bottom: unit.Dp(9),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Label(ui.th, unit.Sp(13), label)
			l.Color = colBg
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		})
		call := macro.Stop()

		fillRRect(gtx, dims.Size, 8, bg)
		call.Add(gtx.Ops)
		return dims
	})
}

func (ui *InstallerUI) buttonSecondary(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return widget.Border{
			Color:        colBorderSubtle,
			CornerRadius: unit.Dp(8),
			Width:        unit.Dp(1),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := colBg
			if c.Hovered() {
				bg = colSurfaceHover
			}
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{
				Left: unit.Dp(16), Right: unit.Dp(16),
				Top: unit.Dp(8), Bottom: unit.Dp(8),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Label(ui.th, unit.Sp(13), label)
				l.Color = colTextMuted
				l.Font.Weight = font.Medium
				return l.Layout(gtx)
			})
			call := macro.Stop()

			fillRRect(gtx, dims.Size, 8, bg)
			call.Add(gtx.Ops)
			return dims
		})
	})
}

func (ui *InstallerUI) buttonDanger(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colError
		if c.Pressed() {
			bg = hex(0xB52B30)
		} else if c.Hovered() {
			bg = hex(0xF2555A)
		}
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Left: unit.Dp(18), Right: unit.Dp(18),
			Top: unit.Dp(9), Bottom: unit.Dp(9),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := material.Label(ui.th, unit.Sp(13), label)
			l.Color = colTextPrimary
			l.Font.Weight = font.Bold
			return l.Layout(gtx)
		})
		call := macro.Stop()

		fillRRect(gtx, dims.Size, 8, bg)
		call.Add(gtx.Ops)
		return dims
	})
}

func fillRRect(gtx layout.Context, size image.Point, radius int, c color.NRGBA) {
	rr := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(radius)))
	paint.FillShape(gtx.Ops, c, rr.Op(gtx.Ops))
}

func spinner(gtx layout.Context, sizeDp int, c color.NRGBA) layout.Dimensions {
	d := gtx.Dp(unit.Dp(sizeDp))
	if d <= 0 {
		d = 20
	}
	center := f32.Pt(float32(d)/2, float32(d)/2)
	radius := float32(d)/2 - 2.5
	if radius < 2 {
		radius = float32(d) / 2
	}
	strokeWidth := float32(2.5)

	gtx.Execute(op.InvalidateCmd{})

	now := gtx.Now
	if now.IsZero() {
		now = time.Now()
	}
	nanos := now.UnixNano() % 900_000_000
	progress := float32(nanos) / 900_000_000.0
	angle := progress * 2 * math.Pi

	// Faint track ring
	trackColor := c
	trackColor.A = 35
	var pTrack clip.Path
	pTrack.Begin(gtx.Ops)
	pTrack.MoveTo(f32.Pt(center.X+radius, center.Y))
	pTrack.ArcTo(center, center, 2*math.Pi)
	trackSpec := pTrack.End()
	paint.FillShape(gtx.Ops, trackColor, clip.Stroke{Path: trackSpec, Width: strokeWidth}.Op())

	// Active rotating arc
	aff := op.Affine(f32.Affine2D{}.Rotate(center, angle)).Push(gtx.Ops)
	var pArc clip.Path
	pArc.Begin(gtx.Ops)
	pArc.MoveTo(f32.Pt(center.X+radius, center.Y))
	pArc.ArcTo(center, center, 1.5*math.Pi)
	arcSpec := pArc.End()
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: arcSpec, Width: strokeWidth}.Op())
	aff.Pop()

	return layout.Dimensions{Size: image.Pt(d, d)}
}
