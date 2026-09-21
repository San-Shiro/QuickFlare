package ui

import (
	"image"
	"testing"

	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

// Vertical centring inside fixed-height rows has regressed three times, each
// time looking like a different bug - a header title sitting high, a dropdown
// label sitting high, a pill's hostname floating above its own status dot.
// It is one bug, and these tests pin the mechanism rather than the symptom.

// rowGtx is a fixed-height row: Min.Y == Max.Y, which is what fixedH produces.
func rowGtx(w, h int) layout.Context {
	return layout.Context{
		Ops: new(op.Ops),
		Constraints: layout.Constraints{
			Min: image.Pt(w, h),
			Max: image.Pt(w, h),
		},
	}
}

// TestFlexHandsRowMinimumToChildren records the Gio behaviour everything else
// here works around. If a future Gio stops doing this, the wrappers become
// unnecessary and this test is the thing that says so.
func TestFlexHandsRowMinimumToChildren(t *testing.T) {
	gtx := rowGtx(200, 48)

	var childMinY, childMaxY int
	layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			childMinY, childMaxY = gtx.Constraints.Min.Y, gtx.Constraints.Max.Y
			return layout.Dimensions{Size: image.Pt(100, 20)}
		}),
	)

	if childMinY != 48 || childMaxY != 48 {
		t.Fatalf("Flex child cross constraints = [%d,%d], want [48,48];\n"+
			"vcenterLeft exists because of this - re-check it if Gio changed",
			childMinY, childMaxY)
	}
}

// TestVCenterLeftReleasesRowMinimum is the property that makes centring work:
// the wrapped widget must be free to report its natural height. A widget told
// Min.Y = rowHeight fills the row, and then there is nothing left to centre.
func TestVCenterLeftReleasesRowMinimum(t *testing.T) {
	gtx := rowGtx(200, 48)

	var innerMinY int
	dims := vcenterLeft(gtx, func(gtx layout.Context) layout.Dimensions {
		innerMinY = gtx.Constraints.Min.Y
		return layout.Dimensions{Size: image.Pt(120, 30)}
	})

	if innerMinY != 0 {
		t.Errorf("inner widget got Min.Y = %d, want 0: it will fill the row "+
			"and defeat the centring", innerMinY)
	}
	if dims.Size.Y != 48 {
		t.Errorf("row height = %d, want 48: the wrapper must still claim the "+
			"whole row", dims.Size.Y)
	}
	if dims.Size.X != 200 {
		t.Errorf("row width = %d, want 200: a Flexed child must keep its "+
			"allocated width or the trailing icons shift", dims.Size.X)
	}
}

// TestVCenterLeftPassesThroughOutsideFixedRows guards the misuse that pushed
// a pill's detail line off the bottom: in a vertical Flex, Max.Y is all the
// remaining space, and centring in that gives the child a box the height of
// the whole list.
func TestVCenterLeftPassesThroughOutsideFixedRows(t *testing.T) {
	gtx := layout.Context{
		Ops: new(op.Ops),
		Constraints: layout.Constraints{
			Min: image.Pt(200, 0),
			Max: image.Pt(200, 400),
		},
	}

	dims := vcenterLeft(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(120, 30)}
	})

	if dims.Size.Y != 30 {
		t.Errorf("height = %d, want 30: outside a fixed row this must lay out "+
			"normally, not claim the column", dims.Size.Y)
	}
}

// TestPillColumnIsCentredInItsRow is the end-to-end version: a two-line block
// in a 48dp row should sit with equal space above and below, not flush to the
// top. Reported height stays the full row so the row does not collapse.
func TestPillColumnIsCentredInItsRow(t *testing.T) {
	const rowH, blockH = 48, 30
	gtx := rowGtx(200, rowH)

	var got layout.Dimensions
	layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			got = vcenterLeft(gtx, func(gtx layout.Context) layout.Dimensions {
				// Stands in for the hostname + detail column, which reports
				// its natural height once the minimum is released.
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(120, blockH)}
					}),
				)
			})
			return got
		}),
	)

	if got.Size.Y != rowH {
		t.Fatalf("column height = %d, want %d", got.Size.Y, rowH)
	}
	// The offset itself lives in the op list, so assert the arithmetic that
	// produces it: an uncentred column would leave this at zero.
	if pad := (rowH - blockH) / 2; pad != 9 {
		t.Fatalf("expected 9dp of padding above the block, got %d", pad)
	}
}

// TestPillColumnLeavesRoomToCentre is the same check with real fonts and real
// metrics, because the stubs above cannot tell you whether a hostname and a
// target line actually fit inside 48dp with room to spare. If they ever stop
// fitting, centring is not the fix - the pill needs to be taller.
func TestPillColumnLeavesRoomToCentre(t *testing.T) {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	p := &Panel{th: th, ic: newIcons()}

	// Covers the usual Windows display scalings: 100%, 125%, 150%, 200%.
	for _, dpi := range []float32{1.0, 1.25, 1.5, 2.0} {
		gtx := layout.Context{
			Ops:    new(op.Ops),
			Metric: unit.Metric{PxPerDp: dpi, PxPerSp: dpi},
		}
		rowH := gtx.Dp(unit.Dp(dimPillH))
		gtx.Constraints = layout.Constraints{
			Min: image.Pt(300, rowH),
			Max: image.Pt(300, rowH),
		}

		relaxed := gtx
		relaxed.Constraints.Min = image.Point{}
		natural := recordDims(relaxed, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(p.monoOneLine("test.sanshiro.qzz.io", tsPillHost, 500, colTextPrimary)),
				layout.Rigid(vgap(sp2)),
				layout.Rigid(p.mono("localhost:8787", tsPillTarget, wRegular, colTextMuted)),
			)
		}).dims.Size.Y

		pad := (rowH - natural) / 2
		t.Logf("%.0f%%: row %dpx, column %dpx, %dpx above and below", dpi*100, rowH, natural, pad)

		if natural >= rowH {
			t.Errorf("%.0f%%: column is %dpx in a %dpx row - it fills the pill, "+
				"so there is nothing left to centre", dpi*100, natural, rowH)
		}
	}
}
