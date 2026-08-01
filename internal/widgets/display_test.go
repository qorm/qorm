package widgets

// Engine-level proof for the display widget set (card / spacer / divider /
// icon / link): scenes route through canvas.LookupWidget — measure, layout
// and rasterize end to end, the badge_test.go pattern.

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func displayEngine(t *testing.T, root *model.Node, size image.Point) (*canvas.Engine, *canvas.HeadlessSurface) {
	t.Helper()
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	return e, canvas.NewHeadlessSurface(size)
}

func textNode(id, s string) *model.Node {
	return &model.Node{Type: "text", ID: id, Props: map[string]any{"text": s}}
}

func TestDisplayWidgetsRegistered(t *testing.T) {
	for _, name := range []string{"card", "spacer", "divider", "verticaldivider", "icon", "link"} {
		if _, ok := canvas.LookupWidget(name); !ok {
			t.Errorf("%s must be registered via this package's init", name)
		}
	}
}

// --- card ---

func TestCardMeasuresChildrenPlusPadding(t *testing.T) {
	text := textNode("t", "Hello")
	card := &model.Node{Type: "card", ID: "c", Children: []*model.Node{text}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{card}}
	e, _ := displayEngine(t, root, image.Pt(200, 100))

	textLn := canvas.Measure(text, e.RT, nil, 1)
	cardLn := canvas.Measure(root, e.RT, nil, 1).Children[0]
	if cardLn.Width != textLn.Width+32 || cardLn.Height != textLn.Height+32 {
		t.Errorf("card = %dx%d, want child %dx%d + 16px padding each side",
			cardLn.Width, cardLn.Height, textLn.Width, textLn.Height)
	}

	// Author padding overrides the type default (16): 4px each side.
	card.Style = map[string]any{"padding": float64(4)}
	cardLn = canvas.Measure(root, e.RT, nil, 1).Children[0]
	if cardLn.Width != textLn.Width+8 || cardLn.Height != textLn.Height+8 {
		t.Errorf("card with padding:4 = %dx%d, want child + 8", cardLn.Width, cardLn.Height)
	}

	// No theme card config → the 16px type default still applies (injected
	// by the widget, not the theme).
	delete(e.RT.Theme.Components, "card")
	card.Style = nil
	cardLn = canvas.Measure(root, e.RT, nil, 1).Children[0]
	if cardLn.Width != textLn.Width+32 || cardLn.Height != textLn.Height+32 {
		t.Errorf("themeless card = %dx%d, want child + 32 (16px default)", cardLn.Width, cardLn.Height)
	}
}

func TestCardRendersRoundedSurface(t *testing.T) {
	card := &model.Node{Type: "card", ID: "c", Style: map[string]any{"width": float64(80), "height": float64(60)}}
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{"background": "#F5F5F7"},
		Children: []*model.Node{card}}
	e, surf := displayEngine(t, root, image.Pt(200, 100))
	e.DrawFrame(surf)
	img := surf.Frame()

	if c := img.RGBAAt(40, 30); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("card center = %v, want cardBg white", c)
	}
	// Radius-16 corner: 1px inside the box is outside the rounded body.
	if c := img.RGBAAt(1, 1); c != (color.RGBA{245, 245, 247, 255}) {
		t.Errorf("card corner = %v, want scene grey showing through the rounded cut", c)
	}
}

// --- spacer ---

func TestSpacerFixedSizeTakesSpace(t *testing.T) {
	t1, t2 := textNode("a", "A"), textNode("b", "B")
	sp := &model.Node{Type: "spacer", ID: "sp", Style: map[string]any{"size": float64(20)}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{t1, sp, t2}}
	e, _ := displayEngine(t, root, image.Pt(200, 200))

	th := canvas.Measure(t1, e.RT, nil, 1).Height
	ln := canvas.Measure(root, e.RT, nil, 1)
	spLn := ln.Children[1]
	if spLn.Width != 20 || spLn.Height != 20 {
		t.Errorf("spacer = %dx%d, want 20x20 (style.size)", spLn.Width, spLn.Height)
	}
	if ln.Height != th+20+th {
		t.Errorf("column = %d tall, want text %d + spacer 20 + text %d", ln.Height, th, th)
	}
}

// Documented degradation: without style.size the HTML flex:1 spacer has no
// engine flex channel, so it collapses to 0x0 and draws nothing (see the
// Spacer type doc).
func TestSpacerWithoutSizeCollapses(t *testing.T) {
	sp := &model.Node{Type: "spacer", ID: "sp"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{textNode("a", "A"), sp, textNode("b", "B")}}
	e, surf := displayEngine(t, root, image.Pt(200, 200))

	ln := canvas.Measure(root, e.RT, nil, 1)
	if spLn := ln.Children[1]; spLn.Width != 0 || spLn.Height != 0 {
		t.Errorf("flex spacer = %dx%d, want 0x0 (no flex channel in v1)", spLn.Width, spLn.Height)
	}
	e.DrawFrame(surf) // must render cleanly with a zero-size widget mounted
	if surf.Presents == 0 {
		t.Error("frame did not present")
	}
}

// --- divider ---

func TestDividerHorizontalRendersLine(t *testing.T) {
	div := &model.Node{Type: "divider", ID: "d", Style: map[string]any{"width": float64(100)}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{div}}
	e, surf := displayEngine(t, root, image.Pt(140, 40))

	ln := canvas.Measure(root, e.RT, nil, 1).Children[0]
	if ln.Width != 100 || ln.Height != 17 { // 1px line + 8px margins baked in
		t.Fatalf("divider = %dx%d, want 100x17", ln.Width, ln.Height)
	}
	e.DrawFrame(surf)
	img := surf.Frame()
	sep := color.RGBA{198, 198, 200, 255}
	if c := img.RGBAAt(50, 8); c != sep {
		t.Errorf("divider mid-line = %v, want separator %v", c, sep)
	}
	if c := img.RGBAAt(50, 2); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("above the line = %v, want untouched background", c)
	}
	if c := img.RGBAAt(120, 8); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("past the resolved width = %v, want untouched background (no stretch channel)", c)
	}
}

func TestVerticalDividerAliasRendersVerticalLine(t *testing.T) {
	vd := &model.Node{Type: "verticaldivider", ID: "vd", Style: map[string]any{"height": float64(40)}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{vd}}
	e, surf := displayEngine(t, root, image.Pt(60, 60))

	ln := canvas.Measure(root, e.RT, nil, 1).Children[0]
	if ln.Width != 17 || ln.Height != 40 {
		t.Fatalf("verticaldivider = %dx%d, want 17x40", ln.Width, ln.Height)
	}
	e.DrawFrame(surf)
	img := surf.Frame()
	sep := color.RGBA{198, 198, 200, 255}
	if c := img.RGBAAt(8, 20); c != sep {
		t.Errorf("vertical line = %v, want separator %v", c, sep)
	}
	if c := img.RGBAAt(2, 20); c != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("left of the line = %v, want untouched background", c)
	}
}

// --- icon ---

func TestIconBuiltinsRasterize(t *testing.T) {
	ink := color.RGBA{0, 0, 0, 255}
	for _, name := range []string{"heart", "star", "zap", "check", "x"} {
		body, ok := iconSet[name]
		if !ok {
			t.Fatalf("icon %q missing from the set", name)
		}
		img := rasterIcon(body, 24, 24, ink)
		if b := img.Bounds(); b.Dx() != 24 || b.Dy() != 24 {
			t.Fatalf("%s: bitmap = %v, want 24x24", name, b)
		}
		opaque := 0
		for y := 0; y < 24; y++ {
			for x := 0; x < 24; x++ {
				if img.RGBAAt(x, y).A > 0 {
					opaque++
				}
			}
		}
		if opaque < 20 {
			t.Errorf("%s: only %d covered px — the glyph did not rasterize", name, opaque)
		}
	}
}

func TestIconRendersThroughEngine(t *testing.T) {
	ic := &model.Node{Type: "icon", ID: "i", Props: map[string]any{"icon": "heart", "size": float64(24)},
		Style: map[string]any{"color": "#FF0000"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{ic}}
	e, surf := displayEngine(t, root, image.Pt(80, 60))

	w, ok := canvas.LookupWidget("icon")
	if !ok {
		t.Fatal("icon not registered")
	}
	if mw, mh := w.Measure(ic, e.RT, 1); mw != 24 || mh != 24 {
		t.Fatalf("icon measure = %dx%d, want 24x24 (size prop)", mw, mh)
	}

	e.DrawFrame(surf)
	img := surf.Frame()
	red := 0
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			c := img.RGBAAt(x, y)
			if c.R > 150 && c.G < 100 && c.B < 100 && c.A > 100 {
				red++
			}
		}
	}
	if red == 0 {
		t.Error("no red glyph pixels — style color did not reach the rasterizer")
	}
}

func TestIconUnknownWarnsOnceAndPlaceholders(t *testing.T) {
	var buf bytes.Buffer
	old := iconWarnOut
	iconWarnOut = &buf
	defer func() { iconWarnOut = old }()

	ic := &model.Node{Type: "icon", ID: "i", Props: map[string]any{"icon": "no-such-icon"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{ic}}
	e, surf := displayEngine(t, root, image.Pt(80, 60))
	e.DrawFrame(surf)
	img := surf.Frame()

	// The 22px default box is a grey placeholder (broken-img convention).
	if c := img.RGBAAt(11, 11); c != (color.RGBA{229, 229, 234, 255}) {
		t.Errorf("placeholder center = %v, want grey box", c)
	}
	// Record again directly (the engine skips clean frames): still one warning.
	w, _ := canvas.LookupWidget("icon")
	w.Record(&canvas.LayoutNode{Node: ic, Width: 22, Height: 22}, e.RT, 1)
	if n := strings.Count(buf.String(), "no-such-icon"); n != 1 {
		t.Errorf("unknown icon warned %d times, want exactly one", n)
	}
}

// --- link ---

func TestLinkMeasuresLikeText(t *testing.T) {
	n := &model.Node{Type: "link", ID: "l", Label: "Docs"}
	e, _ := displayEngine(t, &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}, image.Pt(120, 40))
	w, ok := canvas.LookupWidget("link")
	if !ok {
		t.Fatal("link not registered")
	}
	mw, mh := w.Measure(n, e.RT, 1)
	if want := int(canvas.MeasureText("Docs", 14)); mw != want {
		t.Errorf("link width = %d, want MeasureText(Docs,14) = %d", mw, want)
	}
	fs := 14 // the engine text default (no theme component for link)
	if want := int(float64(fs) * 1.2); mh != want {
		t.Errorf("link height = %d, want one text line %d", mh, want)
	}
}

func TestLinkRendersAccentAndUnderline(t *testing.T) {
	n := &model.Node{Type: "link", ID: "l", Label: "Docs"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	e, surf := displayEngine(t, root, image.Pt(120, 40))
	e.DrawFrame(surf)
	img := surf.Frame()

	accent := false // label ink, antialiased by the sfnt engine
	for y := 0; y < 14 && !accent; y++ {
		for x := 0; x < 40; x++ {
			if c := img.RGBAAt(x, y); c.B > 120 && c.R < 120 && c.A > 100 {
				accent = true
				break
			}
		}
	}
	if !accent {
		t.Error("no accent-colored label pixels")
	}
	underline := false // the exact-fill 1px rect on the line box's last row
	for x := 0; x < 40; x++ {
		if c := img.RGBAAt(x, 15); c == (color.RGBA{0, 122, 255, 255}) {
			underline = true
			break
		}
	}
	if !underline {
		t.Error("no accent underline on the bottom row of the link box")
	}
}

func TestLinkOnPressDispatches(t *testing.T) {
	n := &model.Node{Type: "link", ID: "l", Label: "Docs", OnPress: &model.Invoke{Name: "fire"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	e, surf := displayEngine(t, root, image.Pt(120, 40))
	e.RT.App.Actions = map[string]*model.Action{
		"fire": {ID: "fire", Steps: []model.Step{{Type: "state.set", Path: "clicked", Value: "{{ 'yes' }}"}}},
	}
	e.DrawFrame(surf)

	// The link mounts at the column origin, one text line tall.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 5, Y: 8})
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerRelease, X: 5, Y: 8})
	if v := e.RT.State["clicked"]; v != "yes" {
		t.Errorf("link press dispatched clicked=%v, want \"yes\" (generic type-agnostic dispatch)", v)
	}
}
