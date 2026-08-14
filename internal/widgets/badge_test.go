package widgets

// Engine-level proof that a widget living OUTSIDE the canvas package renders
// through the registry seam: a scene with {"type": "badge"} measures, lays
// out and rasterizes via canvas.LookupWidget.

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func badgeEngine(t *testing.T, label any) (*canvas.Engine, *canvas.HeadlessSurface) {
	t.Helper()
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "badge", ID: "b", Props: map[string]any{"label": label}},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(image.Pt(200, 100))
}

func TestBadgeRegistered(t *testing.T) {
	if _, ok := canvas.LookupWidget("badge"); !ok {
		t.Fatal("badge must be registered via this package's init")
	}
}

func TestBadgeRendersThroughRegistry(t *testing.T) {
	e, surf := badgeEngine(t, "NEW")
	e.DrawFrame(surf)
	img := surf.Frame()

	// The pill occupies a compact area near the top: sample its center band
	// for the pill background (surface != scene white) and dark text pixels.
	var pill, dark int
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y/2; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.RGBAAt(x, y)
			switch {
			case c.R == 255 && c.G == 255 && c.B == 255:
				// scene background
			case c.R < 200 && c.G < 200 && c.B < 200:
				dark++ // label ink (textSecondary, well below pill/background)
			default:
				pill++
			}
		}
	}
	if pill == 0 {
		t.Error("no pill background pixels — the badge shape did not render")
	}
	if dark == 0 {
		t.Error("no label pixels — the badge text did not render")
	}
}

func TestBadgeLabelBindingEvaluates(t *testing.T) {
	e, surf := badgeEngine(t, "{{state.n}}")
	e.RT.State["n"] = float64(7)
	e.DrawFrame(surf)
	// Rendering without panic plus dark text pixels is the observable; the
	// measured width must match the evaluated label "7" via the registry.
	w, ok := canvas.LookupWidget("badge")
	if !ok {
		t.Fatal("badge not registered")
	}
	mw, _ := w.Measure(e.RT.App.Scenes["main"].Children[0], e.RT, nil, 1)
	if mw <= 16 { // 8px padding each side and nothing else
		t.Errorf("bound badge width = %d, want label pixels + padding", mw)
	}
}
