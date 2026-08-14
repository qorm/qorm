package canvas

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// filter: blur() must soft-spread opaque content into neighboring pixels.
func TestFilterBlurSoftensEdges(t *testing.T) {
	size := image.Pt(64, 64)
	// Crisp square via layer without blur vs with blur.
	crisp := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops := &op.Ops{}
	ops.Add(op.LayerOp{Blur: 0})
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.ClipOp{Rect: image.Rect(20, 20, 44, 44)})
	ops.Add(op.PaintOp{})
	ops.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops, crisp)

	blurred := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops2 := &op.Ops{}
	ops2.Add(op.LayerOp{Blur: 4})
	ops2.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops2.Add(op.ClipOp{Rect: image.Rect(20, 20, 44, 44)})
	ops2.Add(op.PaintOp{})
	ops2.Add(op.EndLayerOp{})
	SoftwareRenderer{}.Render(ops2, blurred)

	// Just outside the crisp square: white on crisp, gray on blurred.
	if c := crisp.RGBAAt(18, 32); c.R < 250 {
		t.Fatalf("crisp outside edge should stay white, got %v", c)
	}
	if c := blurred.RGBAAt(18, 32); c.R > 240 {
		t.Fatalf("blurred outside edge must pick up dark content, got %v", c)
	}
	// Far from the square stays white either way.
	if c := blurred.RGBAAt(2, 2); c.R < 250 {
		t.Fatalf("far corner must stay white after blur, got %v", c)
	}
}

// Inset box shadow darkens the inner edge of a filled rect, not outside.
func TestBoxShadowInsetInnerOnly(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	ops := &op.Ops{}
	ops.Add(op.RRectOp{
		Rect: image.Rect(8, 8, 40, 40), Radius: 0,
		Fill:   color.RGBA{255, 255, 255, 255},
		Shadow: color.RGBA{0, 0, 0, 200}, ShadowBlur: 6, ShadowY: 3,
		ShadowInset: true,
	})
	SoftwareRenderer{}.Render(ops, img)

	// Outside the rect: pure white background (no outer drop).
	if c := img.RGBAAt(4, 4); c.R < 250 {
		t.Errorf("inset must not darken outside the rect, got %v", c)
	}
	// Positive shadowY shifts the blocker down → dark band on the TOP inner edge.
	if c := img.RGBAAt(24, 10); c.R > 240 {
		t.Errorf("inset shadow should darken top-inner edge, got %v", c)
	}
	// Deep interior (away from soft edge) stays near fill white.
	if c := img.RGBAAt(24, 24); c.R < 200 {
		t.Errorf("center should stay mostly fill-white, got %v", c)
	}
}

func TestParseFilterBlurAndInset(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{
		"filter":         "blur(8px)",
		"boxShadowInset": true,
		"boxShadowColor": "#000000",
		"boxShadowBlur":  4.0,
	}}
	s := parseStyle(n, rt)
	if s.FilterBlur != 8 {
		t.Errorf("FilterBlur = %v, want 8", s.FilterBlur)
	}
	if !s.BoxShadowInset {
		t.Error("BoxShadowInset must be true")
	}
	// Numeric blur key wins when set after filter in applyStyleProps order —
	// blur key is applied after filter, so it overrides.
	n2 := &model.Node{Type: "box", Style: map[string]any{"blur": 3.5}}
	if s2 := parseStyle(n2, rt); s2.FilterBlur != 3.5 {
		t.Errorf("blur key = %v, want 3.5", s2.FilterBlur)
	}
}

// Absolute x/y with transition must tween between positions.
func TestPosTransitionTweens(t *testing.T) {
	delete(globalAnimStates, "pos1")
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()

	base := NodeStyle{HasPos: true, PosX: 0, PosY: 0, Transition: 200 * time.Millisecond, Width: 20, Height: 20}
	UpdateAndGetAnimatedStyleD("pos1", base, rt, base.Transition)

	target := base
	target.PosX = 100
	target.PosY = 50
	cur, running := UpdateAndGetAnimatedStyleD("pos1", target, rt, target.Transition)
	if !running {
		t.Fatal("pos transition must be in flight right after retarget")
	}
	// t may be ~0 on a fast clock (still running); must not have landed on the target yet.
	if cur.PosX >= 100 {
		t.Errorf("mid PosX = %v, want still short of 100", cur.PosX)
	}
	if cur.PosY >= 50 {
		t.Errorf("mid PosY = %v, want still short of 50", cur.PosY)
	}
}

// End-to-end: style filter blur reaches the group and softens paint.
func TestFilterBlurEndToEnd(t *testing.T) {
	box := &model.Node{Type: "box", ID: "b",
		Style: map[string]any{
			"width": 30.0, "height": 30.0, "x": 20.0, "y": 20.0,
			"background": "#000000", "filter": "blur(3px)",
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)

	// Sample just outside the box — blur should have darkened it.
	// Box is at (20,20)-(50,50) with absolute pos; column may add layout offset.
	// Find any non-white pixel near the likely edge by scanning.
	soft := 0
	frame := surf.Frame()
	for y := 0; y < 80; y++ {
		for x := 0; x < 80; x++ {
			c := frame.RGBAAt(x, y)
			if c.R > 10 && c.R < 245 && c.G == c.R && c.B == c.R {
				soft++
			}
		}
	}
	if soft < 4 {
		t.Fatalf("filter blur end-to-end must produce soft gray edge pixels; soft=%d", soft)
	}
}
