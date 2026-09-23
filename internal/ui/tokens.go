package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/unit"
)

// Design tokens transcribed from the QuickFlare tray-app design spec.
// Values are authoritative - prefer a token
// over a literal anywhere in this package, so a design change lands in one
// place rather than being hunted through layout code.

// Colour tokens.
var (
	colBackground    = hex(0x17191B)
	colSurface       = hex(0x212427)
	colSurfaceRaised = hex(0x2B2F33)
	colInputField    = hex(0x121415)
	colBorderSubtle  = hex(0x2E3337)
	colBorder        = hex(0x3B4045)
	colTextPrimary   = hex(0xF4F5F6)
	colTextMuted     = hex(0x9AA1A8)
	colTextDisabled  = hex(0x5E656B)
	colAccent        = hex(0xF6821F)
	colAccentPressed = hex(0xC4671A)
	colSuccess       = hex(0x4CC38A)
	colWarning       = hex(0xF5B544)
	colError         = hex(0xE5484D)
	colErrorSubtle   = hex(0x381E20)
)

// Spacing scale, in dp.
const (
	sp1 = 2  // dot <-> label micro-gap
	sp2 = 4  // hostname <-> target line
	sp3 = 6  // gap between pills; segmented inner
	sp4 = 8  // icon-button gaps, footer buttons
	sp5 = 10 // pill internal gap, input padding
	sp6 = 12 // pill left inset, card padding
	sp7 = 16 // panel horizontal inset
	sp8 = 20 // onboarding inset, empty-state padding
	sp9 = 24 // onboarding top inset
)

// Corner radii, in dp.
const (
	// No rPanel: the panel itself is not rounded in-app. DWM rounds and
	// clips the window, and a second radius painted inside only left the
	// corners between the two shapes bare - see StyleWindow.
	rCard       = 10 // connection pill, card, empty-state box
	rSegOuter   = 9
	rSegThumb   = 7
	rControl    = 8 // button, input, icon button
	rBadge      = 8 // app mark, step badge
	borderWidth = 1
)

// Component dimensions, in dp.
const (
	dimPanelW       = 340
	dimPanelH       = 460
	dimHeaderH      = 44
	dimFooterH      = 44
	dimStatusBarH   = 30
	dimPillH        = 48
	dimPillGap      = sp3
	dimDot          = 7
	dimDotStatusBar = 6
	dimIconBtnList  = 28
	dimIconBtnFoot  = 30

	// iconGlyphInset is the gap between an icon button's box and the glyph
	// inside it. It is what makes an icon button's optical edge sit inside
	// its layout edge - so a button flush against the panel gutter looks
	// indented next to text that is genuinely flush. Pull the gutter in by
	// this much wherever an icon button sits at the edge.
	iconGlyphInset = 6
	dimSegH        = 32
	dimSegPad      = 3
	dimSegGap      = 3
	dimDropdownH   = 32
	dimDropdownRow = 30
	dimInputH      = 36
	dimButtonH     = 36
	dimPermRowH    = 26 // compact: merged into one bordered list, not 3 separate pills
	dimToggleW     = 34
	dimToggleH     = 20
	dimToggleKnob  = 14
)

// Type scale, in sp. Weight 600 is semibold.
const (
	tsHeading    unit.Sp = 17   // onboarding H1
	tsTitle      unit.Sp = 14   // header, screen name
	tsBody       unit.Sp = 12.5 // prose
	tsButton     unit.Sp = 12.5 // button label
	tsCaption    unit.Sp = 11   // caption, field label
	tsPillHost   unit.Sp = 11.5 // mono
	tsPillTarget unit.Sp = 10.5 // mono
)

const (
	wRegular  font.Weight = font.Normal
	wSemibold font.Weight = 600
)

// faceMono is the typeface used for hostnames and ports, where column
// alignment and character distinction matter more than warmth. The design
// specifies IBM Plex Mono; Go Mono stands in until the Plex faces are
// embedded, since Gio needs TTF/OTF and the design bundle ships woff2.
const faceMono font.Typeface = "Go Mono"

func hex(v uint32) color.NRGBA {
	return color.NRGBA{
		R: uint8(v >> 16),
		G: uint8(v >> 8),
		B: uint8(v),
		A: 0xff,
	}
}

// withAlpha returns c at the given opacity, for hover and pressed washes.
func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}
