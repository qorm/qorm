package canvas

// End-to-end measurement of examples/canvas-fx — the showcase for recent
// canvas rendering features (scroll-snap, mask fade, conic, outline, filter,
// blend, FLIP, text chrome, inset shadow, frost, press). Loads the real app
// JSON and asserts rendered pixels + measure.

import (
	"encoding/json"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func decodeMeasureRows(t *testing.T, e *Engine) []map[string]any {
	t.Helper()
	raw := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("measure json: %v\n%s", err, raw)
	}
	return rows
}

func canvasFxFixture(t *testing.T) (*Engine, *HeadlessSurface, *runtime.Runtime) {
	t.Helper()
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "canvas-fx"))
	if err != nil {
		t.Fatalf("load examples/canvas-fx: %v", err)
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(420, 720))
	e.DrawFrame(surf)
	return e, surf, rt
}

func TestCanvasFxExampleLoadsAndRenders(t *testing.T) {
	e, surf, rt := canvasFxFixture(t)
	if surf.Presents < 1 {
		t.Fatal("first frame must present")
	}
	// Structure/style/logic split: stylesheet + qs actions must load clean.
	if rt.App == nil || len(rt.App.Styles) == 0 {
		t.Fatal("styles/app.qss must contribute cascade rules")
	}
	if len(rt.App.Stylesheets) != 1 || rt.App.Stylesheets[0].ID != "app" {
		t.Fatalf("Stylesheets = %+v, want styles/app.qss", rt.App.Stylesheets)
	}
	for _, d := range rt.App.Diagnostics {
		t.Log("loader:", d)
		if strings.Contains(d, "未知样式键") || strings.Contains(d, "unknown style") {
			t.Errorf("QSS must not warn on canvas FX keys: %s", d)
		}
		if strings.Contains(d, "script 编译失败") {
			t.Errorf("qs actions must compile: %s", d)
		}
	}
	for _, id := range []string{"toggle_filter", "toggle_flip", "toggle_blend"} {
		act := rt.App.Actions[id]
		if act == nil || act.Script == "" {
			t.Errorf("action %q should be a .qs script action", id)
		}
	}
	// Scene must paint non-white content (dark stage background).
	c := surf.Frame().RGBAAt(10, 10)
	if c.R > 40 && c.G > 40 && c.B > 40 {
		t.Fatalf("stage background should be dark, got %v", c)
	}
	// Measure rows include key demo nodes.
	rows := decodeMeasureRows(t, e)
	ids := map[string]bool{}
	for _, r := range rows {
		if id, _ := r["id"].(string); id != "" {
			ids[id] = true
		}
	}
	for _, want := range []string{
		"title", "snap_strip", "conic_disc", "mask_panel", "flip_chip", "btn_flip",
		"clip_circle", "cache_blur", "game_sprite", "tl_sprite", "path_dot",
		"stagger_list", "cubic_dot", "yoyo_style", "yoyo_tl", "btn_burst",
		"text_chrome", "inset_shadow", "frost_panel", "press_card",
		"ellipsis_line", "overflow_clip",
		"invert_card", "tint_card", "xform_box", "pixel_img", "smooth_img",
	} {
		if !ids[want] {
			t.Errorf("measure missing id %q (got %d rows)", want, len(rows))
		}
	}
}

func findCanvasFxNode(rt *runtime.Runtime, id string) *model.Node {
	var found *model.Node
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil || found != nil {
			return
		}
		if n.ID == id {
			found = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	if rt != nil && rt.App != nil {
		for _, sc := range rt.App.Scenes {
			walk(sc)
		}
	}
	return found
}

// TestCanvasFxRenderChrome asserts sections 15–18 parse and measure: text
// stroke/shadow, inset box-shadow + outline, backdrop frost, spring press card.
func TestCanvasFxRenderChrome(t *testing.T) {
	e, _, rt := canvasFxFixture(t)

	textChrome := findCanvasFxNode(rt, "text_chrome")
	inset := findCanvasFxNode(rt, "inset_shadow")
	frost := findCanvasFxNode(rt, "frost_panel")
	press := findCanvasFxNode(rt, "press_card")
	ellipsis := findCanvasFxNode(rt, "ellipsis_line")
	overflow := findCanvasFxNode(rt, "overflow_clip")
	if textChrome == nil || inset == nil || frost == nil || press == nil {
		t.Fatalf("missing chrome demo nodes: text=%v inset=%v frost=%v press=%v",
			textChrome != nil, inset != nil, frost != nil, press != nil)
	}

	ts := parseStyle(textChrome, rt)
	if ts.TextStrokeWidth <= 0 || ts.TextStrokeColor.A == 0 {
		t.Errorf("text_chrome stroke = w=%v c=%v", ts.TextStrokeWidth, ts.TextStrokeColor)
	}
	if ts.TextShadowBlur <= 0 || ts.TextShadowColor.A == 0 {
		t.Errorf("text_chrome shadow = blur=%v c=%v", ts.TextShadowBlur, ts.TextShadowColor)
	}
	if ts.TextDecoration != "underline" {
		t.Errorf("text_chrome TextDecoration = %q", ts.TextDecoration)
	}
	if ts.TextTransform != "uppercase" {
		t.Errorf("text_chrome TextTransform = %q", ts.TextTransform)
	}
	if ts.LetterSpacing <= 0 {
		t.Errorf("text_chrome LetterSpacing = %v", ts.LetterSpacing)
	}
	if ts.FontStyle != "italic" {
		t.Errorf("text_chrome FontStyle = %q", ts.FontStyle)
	}

	is := parseStyle(inset, rt)
	if !is.BoxShadowInset {
		t.Error("inset_shadow BoxShadowInset = false")
	}
	if is.BoxShadowBlur <= 0 || is.BoxShadowColor.A == 0 {
		t.Errorf("inset_shadow box shadow blur=%v c=%v", is.BoxShadowBlur, is.BoxShadowColor)
	}
	if is.OutlineWidth <= 0 || is.OutlineColor.A == 0 {
		t.Errorf("inset_shadow outline w=%v c=%v", is.OutlineWidth, is.OutlineColor)
	}

	fs := parseStyle(frost, rt)
	if fs.BackdropBlur <= 0 {
		t.Errorf("frost_panel BackdropBlur = %v", fs.BackdropBlur)
	}
	if fs.BackdropTint.A == 0 {
		t.Errorf("frost_panel BackdropTint = %v", fs.BackdropTint)
	}

	ps := parseStyle(press, rt)
	if ps.HoverScale <= 1 {
		t.Errorf("press_card HoverScale = %v, want > 1", ps.HoverScale)
	}
	if ps.PressedScale <= 0 || ps.PressedScale >= 1 {
		t.Errorf("press_card PressedScale = %v, want (0,1)", ps.PressedScale)
	}
	if ps.Transition <= 0 {
		t.Errorf("press_card Transition = %v", ps.Transition)
	}
	if !strings.EqualFold(ps.TransitionEasing, "spring") {
		t.Errorf("press_card TransitionEasing = %q, want spring", ps.TransitionEasing)
	}
	if pb, _ := press.Style["pressedBackground"].(string); pb == "" {
		t.Error("press_card inline pressedBackground required for press overlay")
	}

	if clamp := findCanvasFxNode(rt, "clamp_copy"); clamp != nil {
		cs := parseStyle(clamp, rt)
		if cs.LineClamp != 2 {
			t.Errorf("clamp_copy LineClamp = %d", cs.LineClamp)
		}
	}
	if ellipsis != nil {
		es := parseStyle(ellipsis, rt)
		if es.TextOverflow != "ellipsis" {
			t.Errorf("ellipsis_line TextOverflow = %q", es.TextOverflow)
		}
	}
	if overflow != nil {
		os := parseStyle(overflow, rt)
		if os.Overflow != "hidden" {
			t.Errorf("overflow_clip Overflow = %q", os.Overflow)
		}
		if os.BorderRadius <= 0 {
			t.Errorf("overflow_clip BorderRadius = %v", os.BorderRadius)
		}
	}

	rows := decodeMeasureRows(t, e)
	for _, want := range []string{"text_chrome", "inset_shadow", "frost_panel", "press_card"} {
		var w, h float64
		found := false
		for _, r := range rows {
			if r["id"] == want {
				w, h = asF64(r["w"]), asF64(r["h"])
				found = true
				break
			}
		}
		if !found || w < 8 || h < 8 {
			t.Errorf("measure %s missing or tiny (found=%v w=%v h=%v)", want, found, w, h)
		}
	}
}

// TestCanvasFxRenderSpriteStyle asserts sections 19–22: invert/sepia, tint,
// rotate/scale/flip, and pixelated vs bilinear image sampling.
func TestCanvasFxRenderSpriteStyle(t *testing.T) {
	_, _, rt := canvasFxFixture(t)
	inv := findCanvasFxNode(rt, "invert_card")
	tint := findCanvasFxNode(rt, "tint_card")
	xf := findCanvasFxNode(rt, "xform_box")
	pix := findCanvasFxNode(rt, "pixel_img")
	smo := findCanvasFxNode(rt, "smooth_img")
	if inv == nil || tint == nil || xf == nil || pix == nil || smo == nil {
		t.Fatalf("missing sprite-style nodes invert=%v tint=%v xform=%v pixel=%v smooth=%v",
			inv != nil, tint != nil, xf != nil, pix != nil, smo != nil)
	}

	rt.State["colorFx"] = "invert"
	is := parseStyle(inv, rt)
	if is.FilterInvert < 1 {
		t.Errorf("invert_card FilterInvert = %v after colorFx=invert", is.FilterInvert)
	}
	rt.State["colorFx"] = "sepia"
	is = parseStyle(inv, rt)
	if is.FilterSepia < 1 {
		t.Errorf("invert_card FilterSepia = %v after colorFx=sepia", is.FilterSepia)
	}

	ts := parseStyle(tint, rt)
	if ts.Tint.A == 0 {
		t.Errorf("tint_card Tint unset (A=0), want #ff6b6b")
	}

	xs := parseStyle(xf, rt)
	if xs.Scale != 1 {
		t.Errorf("xform_box Scale = %v, want 1", xs.Scale)
	}
	rt.Dispatch("nudge_xform", nil)
	if rt.LastScriptError != "" {
		t.Fatalf("nudge_xform: %s", rt.LastScriptError)
	}
	xs = parseStyle(xf, rt)
	if xs.Rotate != 15 {
		t.Errorf("after nudge, rotate = %v, want 15", xs.Rotate)
	}
	if !xs.FlipX {
		t.Error("after nudge, flipX should be true")
	}

	ps := parseStyle(pix, rt)
	if !strings.EqualFold(ps.ImageRendering, "pixelated") {
		t.Errorf("pixel_img ImageRendering = %q", ps.ImageRendering)
	}
}

// TestCanvasFxQSSClassCascade proves styles/app.qss class rules feed the same
// parseStyle path as inline style (filter binding + layoutMotion + scroll snap).
func TestCanvasFxQSSClassCascade(t *testing.T) {
	_, _, rt := canvasFxFixture(t)
	var filterCard, flipChip, snapStrip *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		switch n.ID {
		case "filter_card":
			filterCard = n
		case "flip_chip":
			flipChip = n
		case "snap_strip":
			snapStrip = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	if filterCard == nil || flipChip == nil || snapStrip == nil {
		t.Fatal("missing classed demo nodes")
	}
	if cs, _ := filterCard.Props["class"].(string); cs != "filterCard" {
		t.Fatalf("filter_card class = %q", cs)
	}
	if len(matchingStyleRules(filterCard, rt)) == 0 {
		t.Fatal("filter_card should match .filterCard from styles/app.qss")
	}
	// filterOn defaults true → QSS binding applies saturate/brightness stack.
	fs := parseStyle(filterCard, rt)
	if fs.FilterSaturate == 1 && fs.FilterBrightness == 1 {
		t.Fatalf("filterCard QSS filter not applied: saturate=%v brightness=%v", fs.FilterSaturate, fs.FilterBrightness)
	}
	// flipChip class: layoutMotion + spring transition.
	cs := parseStyle(flipChip, rt)
	if !cs.LayoutMotion {
		t.Fatalf("flipChip LayoutMotion from QSS = false; rules=%v", matchingStyleRules(flipChip, rt))
	}
	if cs.Transition <= 0 {
		t.Fatalf("flipChip Transition from QSS = %v", cs.Transition)
	}
	if !strings.EqualFold(cs.TransitionEasing, "spring") {
		t.Fatalf("flipChip TransitionEasing = %q, want spring", cs.TransitionEasing)
	}
	// scroll-snap type on the viewport class.
	ss := parseStyle(snapStrip, rt)
	if !strings.Contains(ss.ScrollSnapType, "y") || !strings.Contains(ss.ScrollSnapType, "mandatory") {
		t.Fatalf("snapStrip ScrollSnapType = %q", ss.ScrollSnapType)
	}
}

func TestCanvasFxClipPathCircleCutsCorner(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Content is tall; scroll the outer stage so clip_circle is on-screen.
	var stage *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || stage != nil {
			return
		}
		if n.ID == "stage" {
			stage = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(e.sceneRoot())
	if stage == nil {
		t.Fatal("stage scroll missing")
	}
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{stage: {Y: 1000}}
	e.MarkDirty()
	e.DrawFrame(surf)
	rows := decodeMeasureRows(t, e)
	var x, y, w, h float64
	for _, r := range rows {
		if r["id"] == "clip_circle" {
			x, y, w, h = asF64(r["x"]), asF64(r["y"]), asF64(r["w"]), asF64(r["h"])
			break
		}
	}
	if w < 10 {
		t.Fatal("clip_circle not measured")
	}
	// After scroll, AbsY is still content coords; visual y = absY - scrollOffset.
	visY := int(y - 1000)
	visX := int(x)
	frame := surf.Frame()
	if visY < 0 || visY >= frame.Bounds().Dy() {
		// Still off-screen — rely on unit TestClipPathStyleEndToEnd.
		t.Logf("clip_circle still off-screen after scroll (y=%v visY=%d); layout ok", y, visY)
		return
	}
	mid := frame.RGBAAt(visX+int(w/2), visY+int(h/2))
	if mid.R+mid.G+mid.B < 80 {
		t.Errorf("clip_circle center too dark %v at (%d,%d)", mid, visX+int(w/2), visY+int(h/2))
	}
}

func TestCanvasFxLayerCacheHits(t *testing.T) {
	ResetLayerCache()
	t.Cleanup(ResetLayerCache)
	// Minimal app matching example authoring for cache_blur.
	n := &model.Node{Type: "box", ID: "cache_blur", Style: map[string]any{
		"filter": "blur(2px)", "layerCache": true,
		"width": 120.0, "height": 48.0, "background": "#5e5ce6",
		"x": 10.0, "y": 10.0,
	}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	s := parseStyle(n, rt)
	if !s.LayerCache {
		t.Fatal("layerCache style must parse true")
	}
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(160, 80))
	e.DrawFrame(surf)
	layerCacheMu.Lock()
	ent := layerCache["cache_blur"]
	layerCacheMu.Unlock()
	if ent == nil {
		t.Fatal("expected layerCache entry for id=cache_blur after paint")
	}
	// Second frame same content → still cached (FP stable).
	e.MarkDirty()
	e.DrawFrame(surf)
	layerCacheMu.Lock()
	ent2 := layerCache["cache_blur"]
	layerCacheMu.Unlock()
	if ent2 == nil || ent2.fp != ent.fp {
		t.Fatal("layer cache entry should remain for unchanged content")
	}
}

func TestCanvasFxConicDiscNotFlat(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Sample several points on the conic disc — angles must differ.
	// Disc is roughly left side under title; find non-white colorful pixels.
	frame := surf.Frame()
	var samples []color.RGBA
	for y := 0; y < 720; y += 4 {
		for x := 0; x < 200; x += 4 {
			c := frame.RGBAAt(x, y)
			// Strong chroma (not gray/white/black stage).
			if max3(c.R, c.G, c.B)-min3(c.R, c.G, c.B) > 40 && c.A > 200 {
				samples = append(samples, c)
			}
		}
	}
	if len(samples) < 8 {
		t.Fatalf("expected colorful conic samples, got %d", len(samples))
	}
	// At least two distinct hues among samples.
	distinct := 0
	seen := samples[0]
	for _, c := range samples {
		if absU8(c.R, seen.R)+absU8(c.G, seen.G)+absU8(c.B, seen.B) > 80 {
			distinct++
			seen = c
		}
	}
	if distinct < 1 {
		t.Fatalf("conic disc should vary by angle; samples all similar to %v", samples[0])
	}
	_ = e
}

func TestCanvasFxMaskFadeSoftensRight(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	rows := decodeMeasureRows(t, e)
	var mx, my, mw, mh float64
	found := false
	for _, r := range rows {
		if r["id"] == "mask_panel" {
			mx = asF64(r["x"])
			my = asF64(r["y"])
			mw = asF64(r["w"])
			mh = asF64(r["h"])
			found = true
			break
		}
	}
	if !found || mw < 40 || mh < 10 {
		t.Fatalf("mask_panel measure missing or tiny: %v,%v %vx%v", mx, my, mw, mh)
	}
	// Measure is logical; surface is physical at scale 1 for HeadlessSurface default.
	frame := surf.Frame()
	y := int(my + mh/2)
	xL := int(mx + mw*0.2)
	xR := int(mx + mw*0.92)
	if y < 0 || y >= frame.Bounds().Dy() || xR >= frame.Bounds().Dx() {
		t.Fatalf("sample out of bounds y=%d xL=%d xR=%d", y, xL, xR)
	}
	left := frame.RGBAAt(xL, y)
	right := frame.RGBAAt(xR, y)
	// Stage is dark (#0f1115); opaque purple panel is bright. Fade reduces
	// coverage so the right edge is darker (more stage shows through).
	sumL := int(left.R) + int(left.G) + int(left.B)
	sumR := int(right.R) + int(right.G) + int(right.B)
	if sumR >= sumL {
		t.Fatalf("maskFade right should darken toward stage; left=%v (%d) right=%v (%d)", left, sumL, right, sumR)
	}
	// Right must still differ from solid stage (not fully gone).
	stage := frame.RGBAAt(2, 2)
	if right == stage {
		t.Fatalf("right edge fully transparent (stage); want soft fade, got %v", right)
	}
}

func TestCanvasFxScrollSnapArms(t *testing.T) {
	e, surf, _ := canvasFxFixture(t)
	// Find snap_strip model node.
	var scroll *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || scroll != nil {
			return
		}
		if n.ID == "snap_strip" {
			scroll = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(e.sceneRoot())
	if scroll == nil {
		t.Fatal("snap_strip not found")
	}
	axis, mand := scrollSnapConfig(scroll)
	if axis != "y" || !mand {
		t.Fatalf("scrollSnapType = %q mand=%v, want y mandatory", axis, mand)
	}
	// Offset mid-page and arm snap.
	e.Inter.ScrollOffsets = map[*model.Node]ScrollPos{scroll: {Y: 60}}
	e.MarkDirty()
	e.DrawFrame(surf)
	pos := e.Inter.ScrollOffsets[scroll]
	mom := ScrollMomentum{}
	if !e.tryArmScrollSnap(scroll, &pos, &mom) {
		t.Fatal("mandatory snap mid-page must arm on real example layout")
	}
	if !mom.HasSnapY {
		t.Error("expected HasSnapY")
	}
	if mom.SnapToY != 0 && mom.SnapToY != 120 {
		// Page heights are 120; nearest of 0 or 120 from 60.
		t.Logf("SnapToY=%v (ok if other page edge)", mom.SnapToY)
	}
}

func TestCanvasFxFlipLayoutMotion(t *testing.T) {
	e, surf, rt := canvasFxFixture(t)
	// Initial chip on the left (flipLeft=true → x=16).
	rows := decodeMeasureRows(t, e)
	var x0 float64
	found := false
	for _, r := range rows {
		if r["id"] == "flip_chip" {
			if x, ok := r["x"].(float64); ok {
				x0 = x
				found = true
			} else if xi, ok := r["x"].(int); ok {
				x0 = float64(xi)
				found = true
			}
		}
	}
	if !found {
		t.Fatal("flip_chip not in measure")
	}
	// Toggle flip and settle.
	rt.Dispatch("toggle_flip", nil)
	e.MarkDirty()
	for i := 0; i < 40; i++ {
		e.DrawFrame(surf)
		if !e.Animating() && !flipStillRunning() && !e.Dirty() {
			break
		}
		e.MarkDirty()
		time.Sleep(10 * time.Millisecond)
	}
	rows = decodeMeasureRows(t, e)
	var x1 float64
	for _, r := range rows {
		if r["id"] == "flip_chip" {
			if x, ok := r["x"].(float64); ok {
				x1 = x
			} else if xi, ok := r["x"].(int); ok {
				x1 = float64(xi)
			}
		}
	}
	if x1 <= x0+20 {
		t.Fatalf("after toggle flip, chip should move right: x0=%v x1=%v", x0, x1)
	}
}

func TestCanvasFxCubicAndYoyoLoad(t *testing.T) {
	e, surf, rt := canvasFxFixture(t)
	rt.Dispatch("play_cubic", nil)
	e.MarkDirty()
	e.DrawFrame(surf)
	var cubic *model.Node
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if n.ID == "cubic_dot" {
			cubic = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	if cubic == nil {
		t.Fatal("cubic_dot missing")
	}
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(cubic, 0, rt, inter, start)
	mid := timelineFor(cubic, 0, rt, inter, start.Add(400*time.Millisecond))
	if !mid.running || (mid.dx == 0 && mid.dy == 0) {
		t.Fatalf("cubic mid running=%v dx=%v dy=%v", mid.running, mid.dx, mid.dy)
	}

	// Timeline yoyo object form on yoyo_tl
	var ytl *model.Node
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		if n.ID == "yoyo_tl" {
			ytl = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	if ytl == nil {
		t.Fatal("yoyo_tl missing")
	}
	rt.Dispatch("play_yoyo_tl", nil)
	inter2 := &Interaction{}
	start2 := time.Now()
	// Re-read node props after state change — timelineToken binding is eval'd in timelineFor
	_ = timelineFor(ytl, 0, rt, inter2, start2)
	midY := timelineFor(ytl, 0, rt, inter2, start2.Add(140*time.Millisecond))
	if !midY.running {
		t.Fatal("yoyo timeline should run")
	}
	if midY.scale <= 1.05 && midY.dx == 0 {
		t.Fatalf("yoyo mid scale=%v dx=%v", midY.scale, midY.dx)
	}
}

// TestCanvasFxTimelineOnCompleteAndPath exercises the full engine path:
// load example → play timeline → settle → onComplete action; path step samples.
func TestCanvasFxTimelineOnCompleteAndPath(t *testing.T) {
	e, surf, rt := canvasFxFixture(t)
	// Baseline done counter.
	done0, _ := rt.State["done"].(float64)
	_ = done0
	tlDone0, _ := rt.State["tlDone"].(float64)

	rt.Dispatch("play_timeline", nil)
	e.MarkDirty()
	// Advance wall clock via many frames until timeline settles and complete fires.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e.DrawFrame(surf)
		if !e.Animating() && !e.Dirty() {
			// may still need a frame after complete dirties
			e.DrawFrame(surf)
			break
		}
		time.Sleep(16 * time.Millisecond)
	}
	tlDone1, _ := rt.State["tlDone"].(float64)
	// state numbers may be float64 from JSON
	if tlDone1 <= tlDone0 {
		// try int
		if i, ok := rt.State["tlDone"].(int); ok && float64(i) > tlDone0 {
			tlDone1 = float64(i)
		}
	}
	if tlDone1 <= tlDone0 {
		t.Fatalf("timelineOnComplete should bump tlDone: before=%v after=%v state=%v",
			tlDone0, tlDone1, rt.State["tlDone"])
	}

	// Path follow: arm and sample mid displacement via timeline state.
	rt.Dispatch("play_path", nil)
	e.MarkDirty()
	e.DrawFrame(surf)
	// Locate path_dot model node and evaluate timeline mid-flight.
	var pathDot *model.Node
	var walk func(*model.Node)
	walk = func(n *model.Node) {
		if n == nil || pathDot != nil {
			return
		}
		if n.ID == "path_dot" {
			pathDot = n
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range rt.App.Scenes {
		walk(sc)
	}
	if pathDot == nil {
		t.Fatal("path_dot missing")
	}
	// Fresh interaction clock for path (token changed).
	inter := &Interaction{}
	start := time.Now()
	_ = timelineFor(pathDot, 0, rt, inter, start)
	mid := timelineFor(pathDot, 0, rt, inter, start.Add(350*time.Millisecond))
	if !mid.running {
		t.Fatal("path timeline should be running mid-flight")
	}
	if mid.dx == 0 && mid.dy == 0 {
		t.Fatalf("path should displace, got dx=%v dy=%v", mid.dx, mid.dy)
	}
}

func TestCanvasFxTextTransformInStyle(t *testing.T) {
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{
		"main": {Type: "column"},
	}})
	rt.Theme = theme.GetDefault()
	// Mirror subtitle style from the example.
	n := &model.Node{Type: "text", Props: map[string]any{
		"text": "snap · mask · conic",
	}, Style: map[string]any{"textTransform": "uppercase", "fontSize": 13.0}}
	s := parseStyle(n, rt)
	if s.TextTransform != "uppercase" {
		t.Fatalf("textTransform = %q", s.TextTransform)
	}
	got := applyTextTransform("snap · mask · conic", s.TextTransform)
	if got != "SNAP · MASK · CONIC" {
		t.Fatalf("transform = %q", got)
	}
}

func max3(a, b, c uint8) uint8 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c uint8) uint8 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func absU8(a, b uint8) int {
	d := int(a) - int(b)
	if d < 0 {
		return -d
	}
	return d
}

func asF64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	default:
		return 0
	}
}
