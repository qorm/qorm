package canvas

// JSON-component instantiation tests (components.go): props evaluate in the
// instance scope, slots fill from instance children (named + fallback), ids
// suffix per instance, and the uikit shapes (card of kv rows) measure
// through.

import (
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

func compApp() *model.App {
	return &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		Components: map[string]*model.Node{
			"metric": {
				Type: "card", ID: "mc",
				Children: []*model.Node{
					{Type: "text", ID: "l", Props: map[string]any{"text": "{{prop.label}}"}},
					{Type: "text", ID: "v", Props: map[string]any{"text": "{{prop.value}}"}},
				},
			},
			"panel": {
				Type: "card", ID: "pc",
				Children: []*model.Node{
					{Type: "text", ID: "t", Props: map[string]any{"text": "{{prop.title}}"}},
					{Type: "slot", ID: "s"},
				},
			},
		},
	}
}

func TestComponentPropsResolveInInstanceScope(t *testing.T) {
	rt := runtime.New(compApp())
	rt.Theme = theme.GetDefault()
	rt.State["price"] = "$9"
	inst := &model.Node{Type: "metric", ID: "m1", Props: map[string]any{"label": "Rev", "value": "{{state.price}}"}}
	ln := Measure(inst, rt, nil, 1)
	if ln == nil {
		t.Fatal("instance measured nil")
	}
	// The clone root is the template card with the instance suffix.
	if ln.Node.Type != "card" || ln.Node.ID != "mc_m1" {
		t.Fatalf("clone root = %s %q, want card mc_m1", ln.Node.Type, ln.Node.ID)
	}
	if got := ln.Children[1].Text; got != "$9" {
		t.Errorf("prop.value = %q, want the instance-scope-evaluated $9", got)
	}
}

func TestComponentSlotFillAndFallback(t *testing.T) {
	rt := runtime.New(compApp())
	rt.Theme = theme.GetDefault()
	inst := &model.Node{Type: "panel", ID: "p1",
		Props: map[string]any{"title": "Account"},
		Children: []*model.Node{
			{Type: "text", ID: "row1", Props: map[string]any{"text": "filled"}},
		}}
	ln := Measure(inst, rt, nil, 1)
	// panel template: [title text, slot] — the slot's fill splices in place.
	if len(ln.Children) != 2 {
		t.Fatalf("panel children = %d, want title+slot-fill", len(ln.Children))
	}
	fill := ln.Children[1]
	if fill.Node.ID != "row1" || fill.Text != "filled" {
		t.Fatalf("slot fill = %v %q, want the row1 text spliced in place", fill.Node.ID, fill.Text)
	}

	// No instance children: the slot's own children render as fallback.
	rt2 := runtime.New(compApp())
	rt2.Theme = theme.GetDefault()
	comp := *compApp().Components["panel"]
	comp.Children[1].Children = []*model.Node{{Type: "text", ID: "fb", Props: map[string]any{"text": "empty"}}}
	rt2.App.Components["panel"] = &comp
	inst2 := &model.Node{Type: "panel", ID: "p2", Props: map[string]any{"title": "Empty"}}
	ln2 := Measure(inst2, rt2, nil, 1)
	fb := ln2.Children[1]
	if fb.Node.ID != "fb" || fb.Text != "empty" {
		t.Fatalf("slot fallback = %v %q, want the slot's own fb text", fb.Node.ID, fb.Text)
	}
}

func TestComponentDepthCap(t *testing.T) {
	app := compApp()
	// Self-referential component: must stop at the cap, not recurse forever.
	app.Components["loop"] = &model.Node{Type: "loop", ID: "lp"}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	ln := Measure(&model.Node{Type: "loop", ID: "x"}, rt, nil, 1)
	if ln == nil {
		t.Fatal("self-referential component measured nil (the cap must render the raw node)")
	}
}

// A JSON component can carry an <image> widget in its template; the
// instantiated clone measures + records an image graph node, sourced from
// the app-relative src through the canvas engine's image loader (the same
// path an inline image takes). This pins down the contract: a component
// template is allowed to mix text + image + slot + whatever, and rendering
// walks the cloned subtree exactly as a hand-written scene would.
func TestComponentTemplateWithImageRenders(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, dir, "icon.png", solidRGBA(4, 4, color.RGBA{0x77, 0x88, 0x99, 0xff}))

	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": {Type: "column", ID: "root"}},
		BaseDir: dir,
		Components: map[string]*model.Node{
			"avatar": {
				Type: "row", ID: "av",
				Children: []*model.Node{
					// The image: a 24x24 icon, src bound to the
					// app-relative asset path. Width/height pinned so
					// the test isn't sensitive to the intrinsic size.
					{Type: "image", ID: "img",
						Props: map[string]any{"src": "icon.png"},
						Style:  map[string]any{"width": 24, "height": 24}},
					{Type: "text", ID: "label",
						Props: map[string]any{"text": "{{prop.name}}"}},
				},
			},
		},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	inst := &model.Node{
		Type: "avatar", ID: "u1",
		Props: map[string]any{"name": "Ada"},
	}
	ln := Measure(inst, rt, nil, 1)
	if ln == nil {
		t.Fatal("instance measured nil")
	}
	if ln.Node.Type != "row" || ln.Node.ID != "av_u1" {
		t.Fatalf("clone root = %s %q, want row av_u1", ln.Node.Type, ln.Node.ID)
	}
	// The image should be the first child (an image graph node, not the
	// template's own image widget — the test only checks it survived the
	// instantiation).
	if got := ln.Children[0].Node.Type; got != "image" {
		t.Errorf("avatar children[0] type = %q, want image", got)
	}
	// The text prop was instance-scope-evaluated.
	if got := ln.Children[1].Text; got != "Ada" {
		t.Errorf("avatar children[1] text = %q, want Ada", got)
	}
	// Now drive a full record pass: the image widget should resolve
	// through the loader and produce a non-nil bitmap graph node. We
	// don't assert specific pixel values — the loader pipeline (path
	// resolve, decode, cache) is what we want to keep working.
	rec := RecordImage(ln.Children[0].Node, rt, 24, 24, 0, nil)
	if rec == nil {
		t.Fatal("RecordImage returned nil for component-instantiated image")
	}
	if img, ok := rec.(*graph.Image); !ok {
		t.Errorf("RecordImage type = %T, want *graph.Image", rec)
	} else if img.Bitmap == nil {
		t.Error("RecordImage bitmap = nil, want a decoded RGBA")
	} else if img.Bitmap.Bounds().Empty() {
		t.Error("RecordImage bitmap bounds = empty, want a 24x24 RGBA")
	}
}
