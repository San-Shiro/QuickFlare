package ui

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// iconSet holds the glyphs the panel uses. Decoding is done once at startup:
// widget.NewIcon parses the vector data, which is wasted work per frame.
type iconSet struct {
	copy     *widget.Icon
	stop     *widget.Icon
	delete   *widget.Icon
	add      *widget.Icon
	settings *widget.Icon
	close    *widget.Icon
	chevron  *widget.Icon
	check    *widget.Icon
	external *widget.Icon
}

func newIcons() *iconSet {
	must := func(data []byte) *widget.Icon {
		ic, err := widget.NewIcon(data)
		if err != nil {
			// The icon data is compiled in, so a failure here is a build
			// problem, not a runtime condition worth handling.
			panic("ui: bad icon data: " + err.Error())
		}
		return ic
	}
	return &iconSet{
		copy:     must(icons.ContentContentCopy),
		stop:     must(icons.AVStop),
		delete:   must(icons.ActionDelete),
		add:      must(icons.ContentAdd),
		settings: must(icons.ActionSettings),
		close:    must(icons.NavigationClose),
		chevron:  must(icons.NavigationExpandMore),
		check:    must(icons.NavigationCheck),
		external: must(icons.ActionOpenInNew),
	}
}

// ---------- painting primitives ----------

// fillRRect paints a rounded rectangle of the given size.
func fillRRect(gtx layout.Context, size image.Point, radius int, c color.NRGBA) {
	rr := clip.UniformRRect(image.Rectangle{Max: size}, gtx.Dp(unit.Dp(radius)))
	paint.FillShape(gtx.Ops, c, rr.Op(gtx.Ops))
}

// surface draws w on a rounded filled background that matches w's height and
// fills the available width.
func surface(gtx layout.Context, c color.NRGBA, radius int, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()

	size := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	fillRRect(gtx, size, radius, c)
	call.Add(gtx.Ops)
	return layout.Dimensions{Size: size}
}

// outlined draws w inside a 1dp border.
func outlined(gtx layout.Context, c color.NRGBA, radius int, w layout.Widget) layout.Dimensions {
	return widget.Border{
		Color:        c,
		CornerRadius: unit.Dp(radius),
		Width:        unit.Dp(borderWidth),
	}.Layout(gtx, w)
}

// fixedH forces a widget to an exact height, which is how the spec states
// every control dimension.
func fixedH(gtx layout.Context, dp int, w layout.Widget) layout.Dimensions {
	h := gtx.Dp(unit.Dp(dp))
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h
	dims := w(gtx)
	dims.Size.Y = h
	return dims
}

// centered centres w on both axes within the current constraints.
func centered(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return layout.Center.Layout(gtx, w)
}

// ---------- text ----------

// text builds a label at a given size, weight and colour.
func (p *Panel) text(s string, size unit.Sp, weight font.Weight, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(p.th, size, s)
		l.Color = c
		l.Font.Weight = weight
		return l.Layout(gtx)
	}
}

// centeredText is text that centres its own wrapped lines, for the empty
// states where a two-line sentence sits under a centred title. Centring the
// block without centring the lines inside it leaves the second line looking
// misaligned against the first.
func (p *Panel) centeredText(s string, size unit.Sp, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(p.th, size, s)
		l.Color = c
		l.Alignment = text.Middle
		return l.Layout(gtx)
	}
}

// monoOneLine is a monospaced label that truncates rather than wraps.
func (p *Panel) monoOneLine(s string, size unit.Sp, weight font.Weight, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(p.th, size, s)
		l.Color = c
		l.Font.Weight = weight
		l.Font.Typeface = faceMono
		l.MaxLines = 1
		l.Truncator = "…"
		return l.Layout(gtx)
	}
}

// oneLine is text that truncates rather than wraps. The status bar is a fixed
// 30dp strip: anything that wraps there overflows the panel instead of being
// clipped, which is how a long reconcile message ended up spilling over the
// footer icons.
func (p *Panel) oneLine(s string, size unit.Sp, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(p.th, size, s)
		l.Color = c
		l.MaxLines = 1
		l.Truncator = "…"
		return l.Layout(gtx)
	}
}

// mono builds a monospaced label, for hostnames and ports.
func (p *Panel) mono(s string, size unit.Sp, weight font.Weight, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(p.th, size, s)
		l.Color = c
		l.Font.Weight = weight
		l.Font.Typeface = faceMono
		l.MaxLines = 1
		return l.Layout(gtx)
	}
}

func (p *Panel) heading(s string) layout.Widget {
	return p.text(s, tsHeading, wSemibold, colTextPrimary)
}

func (p *Panel) title(s string) layout.Widget {
	return p.text(s, tsTitle, wSemibold, colTextPrimary)
}

func (p *Panel) body(s string, c color.NRGBA) layout.Widget {
	return p.text(s, tsBody, wRegular, c)
}

func (p *Panel) caption(s string, c color.NRGBA) layout.Widget {
	return p.text(s, tsCaption, wRegular, c)
}

// ---------- status dot ----------

// statusDot is a filled circle sized per the spec: 7dp in lists, 6dp in the
// status bar.
func statusDot(c color.NRGBA, sizeDp int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		d := gtx.Dp(unit.Dp(sizeDp))
		rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(d, d)}, d/2)
		paint.FillShape(gtx.Ops, c, rr.Op(gtx.Ops))
		return layout.Dimensions{Size: image.Pt(d, d)}
	}
}

// alignedDot draws a status dot inside a box matched to a caption line's
// height rather than the dot's own tiny size. A Flex row centres each child
// on its OWN reported height, so a bare 6-7dp dot next to a ~14dp text line
// centres on itself and reads as floating slightly high relative to the
// text's optical centre. Giving it the text's line height to centre within
// instead fixes that without a hand-tuned pixel offset.
func alignedDot(c color.NRGBA, dotDp, lineDp int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(unit.Dp(dotDp)), gtx.Dp(unit.Dp(lineDp))))
		return centered(gtx, statusDot(c, dotDp))
	}
}

// ---------- buttons ----------

// primaryButton is the accent-filled action. Per the spec its label takes the
// background colour, never white - orange is not a strong enough field for
// white text at this size.
func (p *Panel) primaryButton(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return fixedH(gtx, dimButtonH, func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := colAccent
			if c.Pressed() {
				bg = colAccentPressed
			}
			fillRRect(gtx, gtx.Constraints.Max, rControl, bg)
			return centered(gtx, p.text(label, tsButton, wSemibold, colBackground))
		})
	})
}

// secondaryButton is the quiet counterpart: outlined, muted label.
//
// The label carries its own horizontal padding rather than relying on
// leftover space inside a stretched box, so the button looks the same
// whether a caller stretches it (Flexed footer) or lets it size to its own
// label (Min.X zeroed, as the dashboard link does).
func (p *Panel) secondaryButton(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return fixedH(gtx, dimButtonH, func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			// Measure before painting: an auto-width button's border hugs its
			// label, so a hover wash sized to Constraints.Max would spill
			// past the outline it is meant to fill.
			rec := recordDims(gtx, func(gtx layout.Context) layout.Dimensions {
				return outlined(gtx, colBorder, rControl, func(gtx layout.Context) layout.Dimensions {
					return centered(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{
							Left: unit.Dp(sp6), Right: unit.Dp(sp6),
						}.Layout(gtx, p.text(label, tsButton, wSemibold, colTextMuted))
					})
				})
			})
			if c.Hovered() {
				fillRRect(gtx, rec.dims.Size, rControl, colSurfaceRaised)
			}
			rec.call.Add(gtx.Ops)
			return rec.dims
		})
	})
}

// iconButton is a square control holding one glyph.
func (p *Panel) iconButton(gtx layout.Context, c *widget.Clickable, ic *widget.Icon, sizeDp int, tint color.NRGBA) layout.Dimensions {
	side := gtx.Dp(unit.Dp(sizeDp))
	gtx.Constraints = layout.Exact(image.Pt(side, side))

	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if c.Hovered() {
			bg := colSurfaceRaised
			if tint == colError {
				bg = colErrorSubtle
			}
			fillRRect(gtx, gtx.Constraints.Max, rControl, bg)
		}
		return centered(gtx, func(gtx layout.Context) layout.Dimensions {
			glyph := gtx.Dp(unit.Dp(sizeDp - 2*iconGlyphInset))
			gtx.Constraints = layout.Exact(image.Pt(glyph, glyph))
			return ic.Layout(gtx, tint)
		})
	})
}

// ---------- inputs ----------

// input is a text field on the sunken field colour.
func (p *Panel) input(ed *widget.Editor, hint string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return fixedH(gtx, dimInputH, func(gtx layout.Context) layout.Dimensions {
			focused := gtx.Source.Focused(ed)
			fillRRect(gtx, gtx.Constraints.Max, rControl, colInputField)

			bc := colBorderSubtle
			if focused {
				bc = colAccent
			}
			return outlined(gtx, bc, rControl, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(sp5)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					// No layout.W here. It relaxes Min.X, so the editor sized
					// itself to its own text and registered a pointer area
					// only that wide - the field looked full width but was
					// clickable only where the text happened to be. Letting
					// it fill the inset claims the whole box.
					e := material.Editor(p.th, ed, hint)
					e.Color = colTextPrimary
					e.HintColor = colTextDisabled
					e.TextSize = tsBody
					return e.Layout(gtx)
				})
			})
		})
	}
}

// fieldLabel is the caption above an input.
func (p *Panel) fieldLabel(s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(sp3)}.Layout(gtx, p.caption(s, colTextMuted))
	}
}

// toggle is the on/off switch used in Settings.
func (p *Panel) toggle(gtx layout.Context, c *widget.Clickable, on bool) layout.Dimensions {
	w := gtx.Dp(unit.Dp(dimToggleW))
	h := gtx.Dp(unit.Dp(dimToggleH))
	gtx.Constraints = layout.Exact(image.Pt(w, h))

	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		track := colSurfaceRaised
		if on {
			track = colAccent
		}
		rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(w, h)}, h/2)
		paint.FillShape(gtx.Ops, track, rr.Op(gtx.Ops))

		knob := gtx.Dp(unit.Dp(dimToggleKnob))
		pad := (h - knob) / 2
		x := pad
		if on {
			x = w - knob - pad
		}
		defer op.Offset(image.Pt(x, pad)).Push(gtx.Ops).Pop()
		kr := clip.UniformRRect(image.Rectangle{Max: image.Pt(knob, knob)}, knob/2)
		paint.FillShape(gtx.Ops, colTextPrimary, kr.Op(gtx.Ops))

		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// ---------- segmented control ----------

// segmented is the two-option switch between the Domains and Quick lists.
func (p *Panel) segmented(gtx layout.Context, sel *int, opts [2]string, clicks *[2]widget.Clickable) layout.Dimensions {
	return fixedH(gtx, dimSegH, func(gtx layout.Context) layout.Dimensions {
		fillRRect(gtx, gtx.Constraints.Max, rSegOuter, colSurface)

		return layout.UniformInset(unit.Dp(dimSegPad)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.segmentTab(gtx, &clicks[0], opts[0], *sel == 0)
				}),
				layout.Rigid(hgap(dimSegGap)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return p.segmentTab(gtx, &clicks[1], opts[1], *sel == 1)
				}),
			)
		})
	})
}

func (p *Panel) segmentTab(gtx layout.Context, c *widget.Clickable, label string, active bool) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		fg := colTextMuted
		if active {
			fillRRect(gtx, gtx.Constraints.Max, rSegThumb, colSurfaceRaised)
			fg = colTextPrimary
		} else if c.Hovered() {
			fillRRect(gtx, gtx.Constraints.Max, rSegThumb, withAlpha(colSurfaceRaised, 0x80))
		}
		return centered(gtx, p.text(label, tsButton, wSemibold, fg))
	})
}

// ---------- spacing ----------

func vgap(dp int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(0, gtx.Dp(unit.Dp(dp)))}
	}
}

func hgap(dp int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(unit.Dp(dp)), 0)}
	}
}

// flexFill eats leftover vertical space so footers sit at the bottom.
func flexFill(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: gtx.Constraints.Min}
}

// recorded is a laid-out widget whose size is known before it is drawn, so a
// background can be sized to fit it.
type recorded struct {
	call op.CallOp
	dims layout.Dimensions
}

func recordDims(gtx layout.Context, w layout.Widget) recorded {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	return recorded{call: macro.Stop(), dims: dims}
}

// destructiveButton is the primary button in error red, for actions that
// remove something. It is deliberately not the accent colour: the confirm
// step exists to slow the user down, and a button that looks identical to
// every other primary action does not.
func (p *Panel) destructiveButton(gtx layout.Context, c *widget.Clickable, label string) layout.Dimensions {
	return fixedH(gtx, dimButtonH, func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			bg := colError
			if c.Pressed() {
				bg = withAlpha(colError, 0xcc)
			}
			fillRRect(gtx, gtx.Constraints.Max, rControl, bg)
			return centered(gtx, p.text(label, tsButton, wSemibold, colTextPrimary))
		})
	})
}

// vcenterLeft lays a widget out left-aligned and vertically centred inside
// the current row height.
//
// THE RULE, because this keeps coming back: Flex passes its cross-axis
// MINIMUM down to every child (gioui.org/layout/flex.go - crossMin is read
// from the row's constraints and handed straight to each child), and it then
// centres each child on the size that child REPORTS. Anything that honours
// Min.Y therefore fills the row, reports the full row height, and
// Alignment: Middle centres a box that is already the whole row - a no-op.
// The content inside it stays wherever it drew, which for text and for a
// vertical Flex is the top.
//
// That catches more than it looks: material.Label runs its result through
// Constraints.Constrain, which clamps UP to Min, so even a plain caption
// does this. layout.W does not help - it relaxes Min.X only.
//
// So: anything textual sitting directly in a fixedH row needs this wrapper.
// Widgets that set their own Constraints (iconButton, statusDot, centered
// buttons) are already immune. Wrap the whole block, never the individual
// labels inside it - see the pill, where the column is wrapped once.
func vcenterLeft(gtx layout.Context, w layout.Widget) layout.Dimensions {
	// Only meaningful inside a fixed-height row, where Min.Y == Max.Y. In a
	// vertical Flex, Max.Y is all the space left in the column, and centring
	// within that would give the child a box the height of the whole list.
	// Lay out normally instead of silently exploding the layout.
	if gtx.Constraints.Min.Y != gtx.Constraints.Max.Y {
		return w(gtx)
	}
	boxH := gtx.Constraints.Max.Y

	inner := gtx
	inner.Constraints.Min = image.Point{}
	rec := recordDims(inner, w)

	if dy := (boxH - rec.dims.Size.Y) / 2; dy > 0 {
		defer op.Offset(image.Pt(0, dy)).Push(gtx.Ops).Pop()
	}
	rec.call.Add(gtx.Ops)

	size := image.Pt(rec.dims.Size.X, boxH)
	if size.X < gtx.Constraints.Min.X {
		size.X = gtx.Constraints.Min.X
	}
	return layout.Dimensions{Size: size}
}
