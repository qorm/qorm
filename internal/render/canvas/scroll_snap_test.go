package canvas

import (
	"image"
	"testing"
	"time"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

// scrollSnapFixture builds a vertical scroll with three 100px pages and
// scrollSnapType y mandatory.
func scrollSnapFixture(t *testing.T) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	mkPage := func(id, label string) *model.Node {
		return &model.Node{Type: "box", ID: id,
			Style: map[string]any{
				"width": 120.0, "height": 100.0, "background": "#cccccc",
				"scrollSnapAlign": "start",
			},
			Children: []*model.Node{
				{Type: "text", Props: map[string]any{"text": label}},
			}}
	}
	scroll := &model.Node{Type: "scroll", ID: "sc",
		Style: map[string]any{
			"width": 120.0, "height": 100.0,
			"scrollSnapType": "y mandatory",
		},
		Children: []*model.Node{
			mkPage("p0", "A"),
			mkPage("p1", "B"),
			mkPage("p2", "C"),
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{scroll}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(140, 120))
	e.DrawFrame(surf)
	return e, surf, scroll
}

func TestScrollSnapConfigParse(t *testing.T) {
	m := &model.Node{Style: map[string]any{"scrollSnapType": "x proximity"}}
	axis, mand := scrollSnapConfig(m)
	if axis != "x" || mand {
		t.Errorf("x proximity = %q mand=%v", axis, mand)
	}
	m.Style["scrollSnapType"] = "y mandatory"
	axis, mand = scrollSnapConfig(m)
	if axis != "y" || !mand {
		t.Errorf("y mandatory = %q mand=%v", axis, mand)
	}
}

func TestScrollSnapArmsAfterCoast(t *testing.T) {
	e, surf, scroll := scrollSnapFixture(t)
	// Scroll midway between page 0 and 1 (offset ~50 of 100-tall pages).
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{scroll: {Y: 50}}
	e.MarkDirty()
	e.DrawFrame(surf)

	pos := e.Inter.ScrollOffsets[scroll]
	mom := ScrollMomentum{}
	if !e.tryArmScrollSnap(scroll, &pos, &mom) {
		t.Fatal("mandatory snap mid-page must arm")
	}
	if !mom.HasSnapY {
		t.Fatal("expected Y snap target")
	}
	// Nearest of 0 or 100 from 50 — either is fine; must not stay at 50.
	if mom.SnapToY != 0 && mom.SnapToY != 100 {
		t.Errorf("SnapToY = %v, want 0 or 100", mom.SnapToY)
	}
	if !mom.Snap || !mom.Active {
		t.Error("snap momentum must be active")
	}
}

func TestScrollSnapSettles(t *testing.T) {
	e, surf, scroll := scrollSnapFixture(t)
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{scroll: {Y: 40}}
	pos := e.Inter.ScrollOffsets[scroll]
	mom := ScrollMomentum{}
	if !e.tryArmScrollSnap(scroll, &pos, &mom) {
		t.Fatal("must arm snap")
	}
	e.Inter.ScrollMomentum = map[*model.Node]ScrollMomentum{scroll: mom}
	e.Inter.ScrollOffsets[scroll] = pos
	// Advance snap frames until settled (applyScrollMomentum needs elapsed time).
	now := time.Now()
	for i := 0; i < 80; i++ {
		now = now.Add(16 * time.Millisecond)
		e.applyScrollMomentum(now)
		e.MarkDirty()
		e.DrawFrame(surf)
		m := e.Inter.ScrollMomentum[scroll]
		if !m.Active && !m.Snap {
			break
		}
	}
	final := e.Inter.ScrollOffsets[scroll].Y
	if final > 1 && final < 99 {
		t.Errorf("snap must settle to a page edge, got Y=%v", final)
	}
}

func TestMaskFadeParse(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	n := &model.Node{Type: "box", Style: map[string]any{
		"maskFade": "bottom", "maskFadeSize": 32.0,
	}}
	s := parseStyle(n, rt)
	if s.MaskFade != "bottom" || s.MaskFadeSize != 32 {
		t.Errorf("maskFade = %q size=%v", s.MaskFade, s.MaskFadeSize)
	}
	n2 := &model.Node{Type: "box", Style: map[string]any{
		"maskImage": "linear-gradient(to top, black, transparent)",
	}}
	s2 := parseStyle(n2, rt)
	if s2.MaskFade != "top" {
		t.Errorf("maskImage to top → maskFade=%q", s2.MaskFade)
	}
}

func TestMaskFadeSoftensEdge(t *testing.T) {
	// Paint a solid black layer with bottom fade; bottom edge alpha drops.
	box := &model.Node{Type: "box", ID: "m",
		Style: map[string]any{
			"width": 40.0, "height": 40.0, "x": 10.0, "y": 10.0,
			"background": "#000000",
			"maskFade":   "bottom", "maskFadeSize": 20.0,
		}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{box}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(80, 80))
	e.DrawFrame(surf)
	// Near top of box (darker) vs near bottom (lighter over white).
	top := surf.Frame().RGBAAt(30, 15)
	bot := surf.Frame().RGBAAt(30, 45)
	topSum := int(top.R) + int(top.G) + int(top.B)
	botSum := int(bot.R) + int(bot.G) + int(bot.B)
	if botSum <= topSum {
		t.Fatalf("bottom fade must lighten bottom edge; top=%v bot=%v", top, bot)
	}
}
