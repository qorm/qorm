package canvas

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/op"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// Text stroke must ink pixels around the glyph that plain fill does not.
func TestTextStrokeInksOutline(t *testing.T) {
	size := image.Pt(48, 24)
	// Fill-only
	plain := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.TextOp{Text: "A", Pos: image.Pt(8, 4), Scale: 1.4})
	SoftwareRenderer{}.Render(ops, plain)

	// With stroke
	stroked := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops2 := &op.Ops{}
	ops2.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops2.Add(op.TextOp{
		Text: "A", Pos: image.Pt(8, 4), Scale: 1.4,
		StrokeColor: color.RGBA{255, 0, 0, 255}, StrokeWidth: 1.5,
	})
	SoftwareRenderer{}.Render(ops2, stroked)

	// Count pixels that are redder in stroked than plain (stroke halo).
	extra := 0
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			p, s := plain.RGBAAt(x, y), stroked.RGBAAt(x, y)
			if s.R > p.R+20 {
				extra++
			}
		}
	}
	if extra < 4 {
		t.Fatalf("text stroke must ink outline pixels beyond fill-only; extra red-ish=%d", extra)
	}
}

// Text shadow must darken pixels offset from the glyph (under the fill).
func TestTextShadowOffset(t *testing.T) {
	size := image.Pt(48, 28)
	plain := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops := &op.Ops{}
	ops.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops.Add(op.TextOp{Text: "B", Pos: image.Pt(6, 4), Scale: 1.4})
	SoftwareRenderer{}.Render(ops, plain)

	shadowed := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
	ops2 := &op.Ops{}
	ops2.Add(op.ColorOp{Color: color.RGBA{0, 0, 0, 255}})
	ops2.Add(op.TextOp{
		Text: "B", Pos: image.Pt(6, 4), Scale: 1.4,
		ShadowColor: color.RGBA{0, 0, 0, 180}, ShadowX: 3, ShadowY: 3,
	})
	SoftwareRenderer{}.Render(ops2, shadowed)

	// Pixels to the bottom-right of the glyph should be darker with shadow.
	darker := 0
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			p, s := plain.RGBAAt(x, y), shadowed.RGBAAt(x, y)
			// Darker = lower RGB sum (white bg → gray shadow).
			psum := int(p.R) + int(p.G) + int(p.B)
			ssum := int(s.R) + int(s.G) + int(s.B)
			if ssum < psum-30 {
				darker++
			}
		}
	}
	if darker < 4 {
		t.Fatalf("text shadow must darken offset pixels; darker=%d", darker)
	}
}

// Style keys textStroke* / textShadow* parse into NodeStyle.
func TestParseTextDecorStyle(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "text", Props: map[string]any{"text": "Hi"}, Style: map[string]any{
		"textStrokeColor": "#ff0000",
		"textStrokeWidth": 1.5,
		"textShadowColor": "#00000080",
		"textShadowBlur":  2.0,
		"textShadowX":     1.0,
		"textShadowY":     2.0,
		"transition":      "0.3s spring",
	}}
	s := parseStyle(n, rt)
	if s.TextStrokeColor.R != 255 || s.TextStrokeWidth != 1.5 {
		t.Errorf("text stroke = %v / %v", s.TextStrokeColor, s.TextStrokeWidth)
	}
	if s.TextShadowY != 2 || s.TextShadowBlur != 2 {
		t.Errorf("text shadow Y/blur = %v / %v", s.TextShadowY, s.TextShadowBlur)
	}
	if s.Transition != 300*time.Millisecond || s.TransitionEasing != "spring" {
		t.Errorf("transition = %v easing=%q, want 300ms spring", s.Transition, s.TransitionEasing)
	}
}

// Ops fingerprint is stable for equal lists and changes when content changes.
func TestOpsFingerprintStable(t *testing.T) {
	a := buildTestOps()
	b := buildTestOps()
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("identical ops must share fingerprint")
	}
	b.Add(op.TextOp{Text: "X", Pos: image.Pt(0, 0), Scale: 1})
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("different ops must change fingerprint")
	}
}

// A second dirty frame with unchanged scene skips raster work (Render==0)
// but still reports rendered so the host can Present.
func TestRenderIntoSkipsIdenticalOps(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "t", Props: map[string]any{"text": "Hello"},
			Style: map[string]any{"fontSize": 14.0, "color": "#000000"}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(120, 40))

	st1 := e.DrawFrame(surf)
	if surf.Presents != 1 {
		t.Fatalf("first frame must present, presents=%d", surf.Presents)
	}
	if st1.Render == 0 {
		t.Fatal("first frame must raster")
	}

	// Spurious dirty with no visual change: present yes, re-raster no.
	e.MarkDirty()
	rendered, st2 := e.RenderInto(surf.Size(), surf.Scale(), surf.Backbuffer())
	if !rendered {
		t.Fatal("identical ops still return true so the host Presents")
	}
	if st2.Render != 0 {
		t.Fatalf("identical ops must skip raster (Render=%s, want 0)", st2.Render)
	}
	// Change content → must raster again.
	root.Children[0].Props["text"] = "World"
	e.MarkDirty()
	rendered, st3 := e.RenderInto(surf.Size(), surf.Scale(), surf.Backbuffer())
	if !rendered || st3.Render == 0 {
		t.Fatalf("changed text must re-raster (rendered=%v Render=%s)", rendered, st3.Render)
	}
}
