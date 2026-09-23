package ui

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/San-Shiro/QuickFlare/internal/autostart"
)

// panelInset is the horizontal gutter shared by every screen.
func panelInset(w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(sp7), Right: unit.Dp(sp7)}.Layout(gtx, w)
	}
}

// headerInset is panelInset with the right gutter pulled in so a trailing
// icon button is optically aligned - see the note in header().
func headerInset(w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{
			Left:  unit.Dp(sp7),
			Right: unit.Dp(sp7 - iconGlyphInset),
		}.Layout(gtx, w)
	}
}

// ---------- chrome ----------

// appMark draws the QuickFlare logo.
//
// Decoded once at startup rather than per frame - the panel repaints on every
// hover and a PNG decode per frame would be pure waste.
func appMark(sizeDp int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		d := gtx.Dp(unit.Dp(sizeDp))
		if logoOp == (paint.ImageOp{}) {
			// No logo compiled in: fall back to a plain tile rather than
			// leaving a hole in the header.
			fillRRect(gtx, image.Pt(d, d), rBadge, colAccent)
			return layout.Dimensions{Size: image.Pt(d, d)}
		}

		sz := logoOp.Size()
		defer op.Affine(f32.Affine2D{}.Scale(
			f32.Pt(0, 0),
			f32.Pt(float32(d)/float32(sz.X), float32(d)/float32(sz.Y)),
		)).Push(gtx.Ops).Pop()

		logoOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(d, d)}
	}
}

// header is the 44dp title bar. The close affordance is drawn in-panel
// because the window is borderless and has no system title bar.
func (p *Panel) header(gtx layout.Context, title string, showClose bool) layout.Dimensions {
	return fixedH(gtx, dimHeaderH, func(gtx layout.Context) layout.Dimensions {
		// Not panelInset: the close button's glyph sits iconGlyphInset
		// inside its own box, so a box flush with the gutter puts the X
		// visibly further in than every field and row below it. Pulling the
		// right gutter in by that amount lines the glyph up with them
		// instead of the invisible box around it.
		return headerInset(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(appMark(18)),
				layout.Rigid(hgap(sp5)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return vcenterLeft(gtx, p.title(title))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.view != viewMain {
						return layout.Dimensions{}
					}
					badgeText := "ACTIVE"
					badgeColor := colAccent
					if p.disabled {
						badgeText = "PAUSED"
						badgeColor = colTextMuted
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: unit.Dp(sp3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return vcenterLeft(gtx, p.mono(badgeText, tsCaption, wSemibold, badgeColor))
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.toggle(gtx, &p.disableToggle, !p.disabled)
						}),
						layout.Rigid(hgap(sp4)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !showClose {
						return layout.Dimensions{}
					}
					return p.iconButton(gtx, &p.closeBtn, p.ic.close, dimIconBtnList, colTextMuted)
				}),
			)
		})(gtx)
	})
}

// statusBar is the 30dp footer: one status line, plus the add and settings
// controls. The spec replaces full-width buttons with icons here so the list
// keeps as much vertical room as possible.
func (p *Panel) statusBar(gtx layout.Context, withActions bool) layout.Dimensions {
	return fixedH(gtx, dimStatusBarH+sp4, func(gtx layout.Context) layout.Dimensions {
		return panelInset(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					tone := p.statusTone
					if p.disabled {
						tone = colWarning
					}
					return alignedDot(tone, dimDotStatusBar, 16)(gtx)
				}),
				layout.Rigid(hgap(sp3)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.ellipsisCaption(gtx, p.statusText())
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !withActions {
						return layout.Dimensions{}
					}
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.iconButton(gtx, &p.docsBtn, p.ic.external, dimIconBtnFoot, colTextDisabled)
						}),
						layout.Rigid(hgap(sp1)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.iconButton(gtx, &p.settingsBtn, p.ic.settings, dimIconBtnFoot, colTextMuted)
						}),
						layout.Rigid(hgap(sp1)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.iconButton(gtx, &p.addBtn, p.ic.add, dimIconBtnFoot, colAccent)
						}),
					)
				}),
			)
		})(gtx)
	})
}

// ellipsisCaption renders one line of status text, vertically centred in the
// status bar.
func (p *Panel) ellipsisCaption(gtx layout.Context, s string) layout.Dimensions {
	return vcenterLeft(gtx, p.oneLine(s, tsCaption, colTextMuted))
}

// ---------- 01 onboarding ----------

func (p *Panel) onboardingView(gtx layout.Context) layout.Dimensions {
	return layout.Inset{
		Top: unit.Dp(sp9), Bottom: unit.Dp(sp8),
		Left: unit.Dp(sp8), Right: unit.Dp(sp8),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(appMark(28)),
			layout.Rigid(vgap(sp6)),
			layout.Rigid(p.heading("Publish localhost to the internet.")),
			layout.Rigid(vgap(sp8)),

			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.choiceCard(gtx, 0, "Connect a domain", true,
					"Your own hostname, kept between restarts. Needs a Cloudflare account.")
			}),
			layout.Rigid(vgap(sp4)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.choiceCard(gtx, 1, "Quick tunnel", false,
					"A temporary link, no account. Anyone with it can open it.")
			}),

			layout.Flexed(1, flexFill),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.primaryButton(gtx, &p.continueBtn, "Continue")
			}),
		)
	})
}

// choiceCard is a selectable option on the onboarding screen. Selection is
// explicit rather than navigating on click, so the two tradeoffs can be read
// side by side before committing.
func (p *Panel) choiceCard(gtx layout.Context, idx int, title string, recommended bool, body string) layout.Dimensions {
	selected := p.choice == idx
	btn := &p.choiceBtns[idx]

	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colSurface
		if btn.Hovered() && !selected {
			bg = colSurfaceRaised
		}
		border := colBorderSubtle
		if selected {
			border = colAccent
		}

		return surface(gtx, bg, rCard, func(gtx layout.Context) layout.Dimensions {
			return outlined(gtx, border, rCard, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(sp6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(p.text(title, tsBody, wSemibold, colTextPrimary)),
								layout.Rigid(hgap(sp3)),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !recommended {
										return layout.Dimensions{}
									}
									return p.badge(gtx, "RECOMMENDED")
								}),
							)
						}),
						layout.Rigid(vgap(sp2)),
						layout.Rigid(p.caption(body, colTextMuted)),
					)
				})
			})
		})
	})
}

func (p *Panel) badge(gtx layout.Context, s string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := recordDims(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{
				Left: unit.Dp(sp2), Right: unit.Dp(sp2),
				Top: unit.Dp(1), Bottom: unit.Dp(1),
			}.Layout(gtx, p.text(s, unit.Sp(8.5), wSemibold, colAccent))
		})
		fillRRect(gtx, macro.dims.Size, 4, withAlpha(colAccent, 0x22))
		macro.call.Add(gtx.Ops)
		return macro.dims
	})
}

// ---------- 02 token setup ----------

func (p *Panel) tokenSetupView(gtx layout.Context) layout.Dimensions {
	title := "Cloudflare API token"
	rows := p.setupRows()
	if p.view == viewSettings {
		title = "Settings"
		rows = p.settingsRows()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(gtx, title, true)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				// Scrolls only when it has to. The token steps fit without
				// one; Settings adds an App section on top of them and
				// previously pushed the autostart toggle behind the footer
				// buttons, where it could not be reached at all.
				return p.setupList.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					return rows[i](gtx)
				})
			})(gtx)
		}),
		layout.Rigid(vgap(sp4)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return p.secondaryButton(gtx, &p.cancelBtn, "Back")
					}),
					layout.Rigid(hgap(sp4)),
					layout.Flexed(1.4, func(gtx layout.Context) layout.Dimensions {
						label := "Verify token"
						if p.verifying {
							label = "Verifying..."
						}
						return p.primaryButton(gtx, &p.verifyBtn, label)
					}),
				)
			})(gtx)
		}),
		layout.Rigid(p.statusBarRigid(false)),
	)
}

// sectionLabel divides a settings screen into groups.
func (p *Panel) sectionLabel(s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(sp3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(p.caption(s, colTextDisabled)),
				layout.Rigid(vgap(sp2)),
				layout.Rigid(divider),
			)
		})
	}
}

// settingToggle is one labelled on/off row.
func (p *Panel) settingToggle(label string, c *widget.Clickable, on bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(sp4), Bottom: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return vcenterLeft(gtx, p.body(label, colTextPrimary))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.toggle(gtx, c, on)
				}),
			)
		})
	}
}

// setupRows is the first-run walkthrough: someone who has never made a
// Cloudflare API token needs to be told exactly which permissions to tick,
// because picking Read instead of Edit is the failure that produces a token
// which verifies and then cannot do anything.
func (p *Panel) setupRows() []layout.Widget {
	return []layout.Widget{
		p.step(1, "Open the dashboard → My Profile → API Tokens → Create Custom Token."),
		p.dashboardButton(sp9),
		p.step(2, "Add both permissions:"),
		p.permissionList(
			[2]string{"Account", "Cloudflare Tunnel"},
			[2]string{"Zone", "DNS"},
		),
		vgap(sp5),
		p.step(3, "Paste the token below."),
		vgap(sp2),
		p.fieldLabel("API token"),
		p.input(&p.tokenEd, "••••••••••••••••••••"),
	}
}

// settingsRows is the returning-user view. The walkthrough is deliberately
// absent: someone opening Settings already has a working token and is here to
// change something, not to be re-taught how to mint one. The steps pushed the
// App section off the bottom of the panel, which is how the startup toggle
// ended up unreachable.
func (p *Panel) settingsRows() []layout.Widget {
	rows := []layout.Widget{
		p.sectionLabel("Cloudflare"),
		p.fieldLabel("API token"),
		p.input(&p.tokenEd, "••••••••••••••••••••"),
		vgap(sp4),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(128))
					return p.secondaryButton(gtx, &p.openDashBtn, "Open dashboard")
				}),
				layout.Rigid(hgap(sp4)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(80))
					return p.secondaryButton(gtx, &p.docsSetupBtn, "Guide")
				}),
			)
		},
	}
	rows = append(rows,
		vgap(sp7),
		p.sectionLabel("App"),
	)
	if autostart.Supported() {
		rows = append(rows,
			p.settingToggle("Start with Windows", &p.autostartBtn, p.autostartOn),
		)
	}
	rows = append(rows,
		p.settingToggle("Restore last state on launch", &p.restoreStateBtn, p.restoreLastState),
	)

	engineVer := p.cfEngineVersion
	if engineVer == "" {
		engineVer = "inbuilt"
	} else {
		engineVer = "v" + engineVer + " (inbuilt)"
	}

	rows = append(rows,
		vgap(sp7),
		p.sectionLabel("Engine & Version"),
		p.statRow("QuickFlare", "v0.3.1"),
		p.statRow("Cloudflare", engineVer),
	)

	return append(rows, vgap(sp5))
}

// statRow renders a key-value metric row in settings.
func (p *Panel) statRow(label, val string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(sp2), Bottom: unit.Dp(sp2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return vcenterLeft(gtx, p.caption(label, colTextMuted))
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.E.Layout(gtx, p.mono(val, tsCaption, wRegular, colTextPrimary))
				}),
			)
		})
	}
}

// dashboardButton renders the "Open dashboard" and "Guide" actions,
// indented to sit under the step that owns it.
func (p *Panel) dashboardButton(indent int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(indent), Bottom: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(128))
					return p.secondaryButton(gtx, &p.openDashBtn, "Open dashboard")
				}),
				layout.Rigid(hgap(sp4)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = 0
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(80))
					return p.secondaryButton(gtx, &p.docsSetupBtn, "Guide")
				}),
			)
		})
	}
}

// step renders a numbered instruction with a badge.
func (p *Panel) step(n int, body string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d := gtx.Dp(unit.Dp(16))
					gtx.Constraints = layout.Exact(image.Pt(d, d))
					fillRRect(gtx, image.Pt(d, d), 6, colSurfaceRaised)
					return centered(gtx, p.text(fmt.Sprint(n), unit.Sp(9.5), wSemibold, colTextMuted))
				}),
				layout.Rigid(hgap(sp4)),
				layout.Flexed(1, p.caption(body, colTextMuted)),
			)
		})
	}
}

// permissionList renders the required token scopes as one bordered block with
// thin dividers between rows, rather than three separate pills each carrying
// their own margin and background. Edit is highlighted in each row because
// picking Read here is the single most common setup mistake.
func (p *Panel) permissionList(perms ...[2]string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		// Flush under step 2's text - see the note on the dashboard button.
		return layout.Inset{Left: unit.Dp(sp9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return outlined(gtx, colBorderSubtle, rControl, func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(perms)*2-1)
				for i, perm := range perms {
					if i > 0 {
						children = append(children, layout.Rigid(divider))
					}
					perm := perm
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.permissionRow(gtx, perm[0], perm[1])
					}))
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
			})
		})
	}
}

func (p *Panel) permissionRow(gtx layout.Context, scope, resource string) layout.Dimensions {
	return fixedH(gtx, dimPermRowH, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(sp5), Right: unit.Dp(sp5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Both labels go through vcenterLeft for the same reason as the
			// pill: a bare label honours the row's Min.Y and draws at the
			// top of it.
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return vcenterLeft(gtx, p.caption(scope+" · "+resource, colTextMuted))
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return vcenterLeft(gtx, p.text("Edit", tsCaption, wSemibold, colAccent))
				}),
			)
		})
	})
}

// divider is a 1dp rule between rows in a merged list.
func divider(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(borderWidth))
	paint.FillShape(gtx.Ops, colBorderSubtle, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, h)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
}

// statusBarRigid wraps the status bar for use as a Flex child.
func (p *Panel) statusBarRigid(withActions bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return p.statusBar(gtx, withActions)
	}
}

// ---------- 03 main panel ----------

func (p *Panel) mainView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(gtx, "QuickFlare", true)
		}),
		layout.Rigid(vgap(sp4)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return p.segmented(gtx, &p.mode, [2]string{"Domains", "Quick"}, &p.segBtns)
			})(gtx)
		}),
		// The domain picker belongs to the Domains list, so it sits under
		// the switch that selects it - and is absent entirely on Quick,
		// where an anonymous tunnel has no zone to choose.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.mode != modeDomains {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(sp4)}.Layout(gtx, panelInset(func(gtx layout.Context) layout.Dimensions {
				return p.domainPicker(gtx)
			}))
		}),
		layout.Rigid(vgap(sp5)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelInset(p.connectionList)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.mode != modeQuick || len(p.quicks) == 0 {
				return layout.Dimensions{}
			}
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(sp3)}.Layout(gtx,
					p.caption("Quick links are unauthenticated and disappear when QuickFlare quits.", colTextDisabled))
			})(gtx)
		}),
		layout.Rigid(p.statusBarRigid(true)),
	)
}

// addTokenPrompt replaces the domain dropdown until a token and domain are
// connected. Clicking it goes straight to token setup, same as the footer
// "+" does when nothing is configured yet.
func (p *Panel) addTokenPrompt(gtx layout.Context) layout.Dimensions {
	return fixedH(gtx, dimDropdownH, func(gtx layout.Context) layout.Dimensions {
		return p.addTokenBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := colSurface
			if p.addTokenBtn.Hovered() {
				bg = colSurfaceRaised
			}
			fillRRect(gtx, gtx.Constraints.Max, rControl, bg)
			return outlined(gtx, withAlpha(colAccent, 0x55), rControl, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(sp5), Right: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							d := gtx.Dp(unit.Dp(14))
							gtx.Constraints = layout.Exact(image.Pt(d, d))
							return p.ic.add.Layout(gtx, colAccent)
						}),
						layout.Rigid(hgap(sp3)),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return vcenterLeft(gtx, p.text("Add API token", tsBody, wSemibold, colAccent))
						}),
					)
				})
			})
		})
	})
}

// domainPicker shows which zone routes are published on, and lets the user
// switch between the zones the token can reach.
//
// With no API token yet there is nothing to pick - showing a dropdown that
// only ever says "No domain connected" invites clicking a control that does
// nothing. Show the actual next step instead.
func (p *Panel) domainPicker(gtx layout.Context) layout.Dimensions {
	if !p.configured() {
		return p.addTokenPrompt(gtx)
	}

	dims := p.domainButton(gtx)

	if p.domainOpen && len(p.zones) > 0 {
		// An overlay, not a sibling in the column. Laid out as a Rigid the
		// open menu pushed the tab switch and the whole route list down the
		// panel and yanked them back on close; recorded here and replayed
		// after everything else, it paints on top and costs no layout space.
		macro := op.Record(gtx.Ops)
		off := op.Offset(image.Pt(0, dims.Size.Y+gtx.Dp(unit.Dp(sp2)))).Push(gtx.Ops)
		p.domainMenu(gtx)
		off.Pop()
		op.Defer(gtx.Ops, macro.Stop())
	}
	return dims
}

// domainButton is the closed state of the picker.
func (p *Panel) domainButton(gtx layout.Context) layout.Dimensions {
	return fixedH(gtx, dimDropdownH, func(gtx layout.Context) layout.Dimensions {
		return p.domainBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := colSurface
			if p.domainBtn.Hovered() {
				bg = colSurfaceRaised
			}
			fillRRect(gtx, gtx.Constraints.Max, rControl, bg)
			return layout.Inset{Left: unit.Dp(sp5), Right: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return vcenterLeft(gtx, p.mono(p.currentDomain(), tsPillHost, wRegular, colTextPrimary))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						d := gtx.Dp(unit.Dp(14))
						gtx.Constraints = layout.Exact(image.Pt(d, d))
						return p.ic.chevron.Layout(gtx, colTextMuted)
					}),
				)
			})
		})
	})
}

// domainMenu is drawn as an overlay, so it paints its own opaque background
// and border - there is content underneath it, not panel ground.
func (p *Panel) domainMenu(gtx layout.Context) layout.Dimensions {
	for len(p.domainRows) < len(p.zones) {
		p.domainRows = append(p.domainRows, widget.Clickable{})
	}
	return surface(gtx, colSurfaceRaised, rControl, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(p.zones))
		for i := range p.zones {
			i := i
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.domainRow(gtx, i)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

// domainRow renders one zone in the picker. Every zone the token can reach is
// selectable: QuickFlare takes one DNS record per route rather than the whole
// wildcard, so a zone already serving other things is still usable. Whether a
// particular hostname is free is asked when that route is created.
func (p *Panel) domainRow(gtx layout.Context, i int) layout.Dimensions {
	z := p.zones[i]
	return fixedH(gtx, dimDropdownRow, func(gtx layout.Context) layout.Dimensions {
		return p.domainRows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if p.domainRows[i].Hovered() {
				fillRRect(gtx, gtx.Constraints.Max, rControl, colSurface)
			}
			return layout.Inset{Left: unit.Dp(sp5), Right: unit.Dp(sp5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return vcenterLeft(gtx, p.mono(z.Name, tsPillTarget, wRegular, colTextPrimary))
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i != p.domainIx {
							return layout.Dimensions{}
						}
						d := gtx.Dp(unit.Dp(12))
						gtx.Constraints = layout.Exact(image.Pt(d, d))
						return p.ic.check.Layout(gtx, colAccent)
					}),
				)
			})
		})
	})
}

// connectionList renders whichever list the segmented control selects.
func (p *Panel) connectionList(gtx layout.Context) layout.Dimensions {
	if p.mode == modeDomains {
		if len(p.routes) == 0 {
			return p.emptyState(gtx,
				"No routes on "+orPlaceholder(p.currentDomain(), "your domain"),
				"Add one to map a subdomain to a port on this machine.",
				"Add route")
		}
		return p.connList.Layout(gtx, len(p.routes), func(gtx layout.Context, i int) layout.Dimensions {
			r := &p.routes[i]
			port := extractPort(r.Target)
			listening, hasCheck := p.isPortListening(port)
			return p.pill(gtx, pillData{
				host:          r.Hostname,
				detail:        routeDetail(r),
				status:        r.Status,
				portListening: listening,
				hasPortCheck:  hasCheck,
				copyBtn:       &r.copyBtn,
				stopBtn:       &r.stopBtn,
				copiedAt:      r.copiedAt,
			})
		})
	}

	if len(p.quicks) == 0 {
		return p.emptyState(gtx,
			"No quick tunnels running",
			"Start one to get a temporary public link. It ends when QuickFlare quits.",
			"Start quick tunnel")
	}
	return p.connList.Layout(gtx, len(p.quicks), func(gtx layout.Context, i int) layout.Dimensions {
		q := p.quicks[i]
		listening, hasCheck := p.isPortListening(q.Port)
		return p.pill(gtx, pillData{
			host:          quickHost(q),
			detail:        quickDetail(q),
			status:        q.Status,
			portListening: listening,
			hasPortCheck:  hasCheck,
			copyBtn:       &q.copyBtn,
			stopBtn:       &q.stopBtn,
			copiedAt:      q.copiedAt,
		})
	})
}

func routeDetail(r *Route) string {
	if r.Detail != "" {
		return r.Detail
	}
	return r.Target
}

func quickHost(q *QuickEntry) string {
	if q.URL == "" {
		return "—"
	}
	return strings.TrimPrefix(strings.TrimPrefix(q.URL, "https://"), "http://")
}

func quickDetail(q *QuickEntry) string {
	switch {
	case q.Status == statusError && q.Err != "":
		return q.Err
	case q.URL == "":
		return fmt.Sprintf("requesting link · :%d", q.Port)
	}
	return fmt.Sprintf("localhost:%d", q.Port)
}

func orPlaceholder(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ---------- connection pill ----------

type pillData struct {
	host          string
	detail        string
	status        connStatus
	portListening bool
	hasPortCheck  bool
	copyBtn       *widget.Clickable
	stopBtn       *widget.Clickable
	copiedAt      time.Time
}

// pill is one connection row: 48dp tall, dense enough that ten fit without
// the panel feeling like a spreadsheet.
func (p *Panel) pill(gtx layout.Context, d pillData) layout.Dimensions {
	copied := time.Since(d.copiedAt) < copiedFor

	return layout.Inset{Bottom: unit.Dp(dimPillGap)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return fixedH(gtx, dimPillH, func(gtx layout.Context) layout.Dimensions {
			bg := colSurface
			if d.copyBtn.Hovered() || d.stopBtn.Hovered() {
				bg = colSurfaceRaised
			}
			fillRRect(gtx, gtx.Constraints.Max, rCard, bg)

			dotColor := statusColor(d.status)
			if p.disabled {
				dotColor = colTextDisabled
			} else if d.status == statusConnected && d.hasPortCheck && !d.portListening {
				dotColor = colWarning
			}

			return layout.Inset{Left: unit.Dp(sp6), Right: unit.Dp(sp4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(statusDot(dotColor, dimDot)),
					layout.Rigid(hgap(sp5)),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						// vcenterLeft wraps the whole column, not the labels
						// inside it. Flex hands every child its own cross-axis
						// MINIMUM, so in this 48dp row the column is told
						// Min.Y == Max.Y == 48dp, fills it, and draws both
						// lines from the top - which leaves Alignment: Middle
						// with nothing left to centre and floats the text
						// above the dot and the icons.
						return vcenterLeft(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(p.monoOneLine(truncateHost(d.host), tsPillHost, 500, colTextPrimary)),
								layout.Rigid(vgap(sp2)),
								layout.Rigid(p.pillDetail(d, copied)),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						ic, tint := p.ic.copy, colTextMuted
						if copied {
							ic, tint = p.ic.check, colSuccess
						}
						return p.iconButton(gtx, d.copyBtn, ic, dimIconBtnList, tint)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.iconButton(gtx, d.stopBtn, p.ic.delete, dimIconBtnList, colError)
					}),
				)
			})
		})
	})
}

func (p *Panel) pillDetail(d pillData, copied bool) layout.Widget {
	if copied {
		return p.mono("copied to clipboard", tsPillTarget, wRegular, colSuccess)
	}
	if p.disabled {
		txt := d.detail
		if !strings.Contains(txt, "paused") {
			txt = txt + " · paused"
		}
		return p.mono(txt, tsPillTarget, wRegular, colTextDisabled)
	}
	tone := colTextMuted
	text := d.detail
	if d.status == statusError {
		tone = colError
	} else if d.status == statusStarting {
		tone = colWarning
	} else if d.status == statusConnected && d.hasPortCheck && !d.portListening {
		tone = colWarning
		text = d.detail + " · no service listening"
	}
	return p.mono(text, tsPillTarget, wRegular, tone)
}

// emptyState fills the list area when there is nothing to show.
func (p *Panel) emptyState(gtx layout.Context, title, body, action string) layout.Dimensions {
	return centered(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: unit.Dp(sp8), Right: unit.Dp(sp8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(p.centeredText(title, tsBody, colTextMuted)),
				layout.Rigid(vgap(sp3)),
				layout.Rigid(p.centeredText(body, tsCaption, colTextDisabled)),
				layout.Rigid(vgap(sp6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(unit.Dp(150))
					return p.secondaryButton(gtx, &p.emptyAddBtn, action)
				}),
			)
		})
	})
}

// ---------- 04 add route ----------

func (p *Panel) addRouteView(gtx layout.Context) layout.Dimensions {
	if p.mode == modeQuick {
		return p.addQuickView(gtx)
	}

	suffix := "." + orPlaceholder(p.currentDomain(), "example.com")
	result := "—"
	if h := strings.TrimSpace(p.hostEd.Text()); h != "" {
		result = h + suffix
	}
	target := "localhost:" + orPlaceholder(strings.TrimSpace(p.portEd.Text()), "…")

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(gtx, "New route", true)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(p.fieldLabel("Subdomain")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, p.input(&p.hostEd, "dashboard")),
							layout.Rigid(hgap(sp4)),
							layout.Rigid(p.mono(suffix, tsPillTarget, wRegular, colTextDisabled)),
						)
					}),
					layout.Rigid(vgap(sp5)),

					layout.Rigid(p.fieldLabel("Local port")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(p.mono("localhost:", tsPillTarget, wRegular, colTextDisabled)),
							layout.Rigid(hgap(sp4)),
							layout.Flexed(1, p.input(&p.portEd, "3000")),
						)
					}),
					layout.Rigid(vgap(sp5)),

					layout.Rigid(p.fieldLabel("Result")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return surface(gtx, colSurface, rCard, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(sp5)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(1, p.mono(truncateHost(result), tsPillHost, 500, colTextPrimary)),
									layout.Rigid(hgap(sp3)),
									layout.Rigid(p.mono("→ "+target, tsPillTarget, wRegular, colTextMuted)),
								)
							})
						})
					}),
					layout.Rigid(vgap(sp5)),

					layout.Rigid(p.caption(
						"The route is public once it is live. Anyone who knows the address can reach it.",
						colTextDisabled)),
					layout.Flexed(1, flexFill),
				)
			})(gtx)
		}),
		layout.Rigid(p.formFooter("Create route")),
		layout.Rigid(p.statusBarRigid(false)),
	)
}

// addQuickView is the port-only form; an anonymous tunnel has no other
// configuration to offer.
func (p *Panel) addQuickView(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(gtx, "New quick tunnel", true)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(p.fieldLabel("Local port")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(p.mono("localhost:", tsPillTarget, wRegular, colTextDisabled)),
							layout.Rigid(hgap(sp4)),
							layout.Flexed(1, p.input(&p.portEd, "3000")),
						)
					}),
					layout.Rigid(vgap(sp5)),
					layout.Rigid(p.caption(
						"Cloudflare gives you a random trycloudflare.com address. It cannot be "+
							"password protected, changes every time, and stops when QuickFlare quits.",
						colTextDisabled)),
					layout.Rigid(vgap(sp4)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.binErr == nil {
							return layout.Dimensions{}
						}
						return p.caption("cloudflared was not found - install it and restart QuickFlare.", colError)(gtx)
					}),
					layout.Flexed(1, flexFill),
				)
			})(gtx)
		}),
		layout.Rigid(p.formFooter("Start tunnel")),
		layout.Rigid(p.statusBarRigid(false)),
	)
}

func (p *Panel) formFooter(primary string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return panelInset(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.secondaryButton(gtx, &p.cancelBtn, "Cancel")
				}),
				layout.Rigid(hgap(sp4)),
				layout.Flexed(1.4, func(gtx layout.Context) layout.Dimensions {
					return p.primaryButton(gtx, &p.createBtn, primary)
				}),
			)
		})(gtx)
	}
}

// ---------- confirm delete ----------

// confirmDeleteView asks before tearing a route down.
//
// It lists what is about to be removed rather than asking a bare "are you
// sure": these are account-level deletions on Cloudflare, not a row leaving a
// list, and the difference between "unpublish this" and "delete a DNS record
// and a tunnel route" is exactly what a confirmation should be making
// visible.
func (p *Panel) confirmDeleteView(gtx layout.Context) layout.Dimensions {
	r := p.routeByHostname(p.pendingDelete)
	if r == nil {
		// Route vanished while the dialog was open - nothing to confirm.
		return p.mainView(gtx)
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.header(gtx, "Remove route", false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return surface(gtx, colSurface, rCard, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(unit.Dp(sp6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(p.mono(truncateHost(r.Hostname), tsPillHost, 500, colTextPrimary)),
									layout.Rigid(vgap(sp2)),
									layout.Rigid(p.mono(r.Target, tsPillTarget, wRegular, colTextMuted)),
								)
							})
						})
					}),
					layout.Rigid(vgap(sp6)),
					layout.Rigid(p.caption("This removes from Cloudflare:", colTextMuted)),
					layout.Rigid(vgap(sp3)),
					layout.Rigid(p.removalItem("Its DNS record, if QuickFlare created it")),
					layout.Rigid(p.removalItem("Its route in the tunnel")),
					layout.Rigid(vgap(sp5)),
					layout.Rigid(p.caption(
						"The subdomain becomes free to use again. Anything running on "+
							r.Target+" keeps running.", colTextDisabled)),
					layout.Flexed(1, flexFill),
				)
			})(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return panelInset(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return p.secondaryButton(gtx, &p.cancelBtn, "Cancel")
					}),
					layout.Rigid(hgap(sp4)),
					layout.Flexed(1.4, func(gtx layout.Context) layout.Dimensions {
						return p.destructiveButton(gtx, &p.confirmBtn, "Remove route")
					}),
				)
			})(gtx)
		}),
		layout.Rigid(p.statusBarRigid(false)),
	)
}

// removalItem is one bullet in the confirmation list.
func (p *Panel) removalItem(s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(sp2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(alignedDot(colError, 4, 16)),
				layout.Rigid(hgap(sp4)),
				layout.Flexed(1, p.caption(s, colTextMuted)),
			)
		})
	}
}
