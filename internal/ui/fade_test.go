package ui

import (
	"image"
	"math"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget/material"
	"github.com/San-Shiro/QuickFlare/internal/core"
)

func TestLoadFontCollection(t *testing.T) {
	col := loadFontCollection()
	if len(col) == 0 {
		t.Fatal("loadFontCollection returned empty collection")
	}
	var hasInter, hasMono bool
	for _, ff := range col {
		if ff.Font.Typeface == "Inter" {
			hasInter = true
		}
		if ff.Font.Typeface == faceMono {
			hasMono = true
		}
	}
	if !hasInter {
		t.Errorf("collection missing Inter font face")
	}
	if !hasMono {
		t.Errorf("collection missing %s font face", faceMono)
	}
}

func TestPushOpacityAndRecord(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Now: time.Now(),
		Constraints: layout.Constraints{
			Min: image.Pt(100, 100),
			Max: image.Pt(200, 200),
		},
	}

	macro := op.Record(gtx.Ops)
	paint.ColorOp{Color: colAccent}.Add(gtx.Ops)
	call := macro.Stop()

	st := paint.PushOpacity(gtx.Ops, 0.5)
	call.Add(gtx.Ops)
	st.Pop()
}

func TestArcToCircle(t *testing.T) {
	var ops op.Ops
	var p clip.Path
	p.Begin(&ops)
	center := f32.Pt(10, 10)
	radius := float32(8)
	p.MoveTo(f32.Pt(center.X+radius, center.Y))
	p.ArcTo(center, center, 1.5*math.Pi)
	spec := p.End()
	stroke := clip.Stroke{Path: spec, Width: 2}.Op()
	paint.FillShape(&ops, colAccent, stroke)
}

func TestSpinnerRotation(t *testing.T) {
	var ops op.Ops
	center := f32.Pt(10, 10)
	aff := op.Affine(f32.Affine2D{}.Rotate(center, 0.5)).Push(&ops)
	aff.Pop()
}

func TestViewTransitionAndHasStarting(t *testing.T) {
	p := &Panel{
		view: viewMain,
	}
	if p.hasStartingRoutes() {
		t.Errorf("expected no starting routes initially")
	}

	p.routes = []Route{
		{Route: core.Route{Hostname: "api.example.com", Status: statusStarting}},
	}
	if !p.hasStartingRoutes() {
		t.Errorf("expected hasStartingRoutes to be true when a route is statusStarting")
	}

	p.routes[0].Status = statusConnected
	if p.hasStartingRoutes() {
		t.Errorf("expected hasStartingRoutes to be false when routes are connected")
	}

	// Test setView
	p.setView(viewAddRoute)
	if p.view != viewAddRoute {
		t.Errorf("expected view to be viewAddRoute, got %v", p.view)
	}
	if !p.trans.active {
		t.Errorf("expected trans.active to be true")
	}
	if p.trans.fromView != viewMain || p.trans.toView != viewAddRoute {
		t.Errorf("expected transition from viewMain to viewAddRoute, got %v -> %v", p.trans.fromView, p.trans.toView)
	}
}

func TestDeleteOverlayLayout(t *testing.T) {
	p := &Panel{
		th:            material.NewTheme(),
		pendingDelete: "demo.example.com",
	}

	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Now: time.Now(),
		Constraints: layout.Constraints{
			Min: image.Pt(340, 460),
			Max: image.Pt(340, 460),
		},
	}

	dims := p.deleteOverlay(gtx)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Errorf("deleteOverlay produced empty dimensions: %+v", dims)
	}
}

func TestSpinnerLayout(t *testing.T) {
	var ops op.Ops
	gtx := layout.Context{
		Ops: &ops,
		Now: time.Now(),
		Constraints: layout.Constraints{
			Min: image.Pt(10, 10),
			Max: image.Pt(20, 20),
		},
	}

	dims := spinner(gtx, 12, colAccent)
	if dims.Size.X == 0 || dims.Size.Y == 0 {
		t.Errorf("spinner produced empty dimensions: %+v", dims)
	}
}
