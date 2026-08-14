package canvas

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/model"
	"github.com/qorm/platform/internal/runtime"
	"github.com/qorm/platform/internal/theme"
)

func TestTilemapBakesOneImageAndCaches(t *testing.T) {
	ResetTilemapCache()
	t.Cleanup(ResetTilemapCache)

	n := &model.Node{Type: "tilemap", ID: "tm",
		Props: map[string]any{
			"rows": []any{"1.1", "..."},
			"cell": 32.0,
			"atlas": map[string]any{
				"1": "assets/ground.png",
			},
		},
		Style: map[string]any{"x": 0.0, "y": 0.0},
	}
	root := &model.Node{Type: "board", ID: "world",
		Style:    map[string]any{"width": 64.0, "height": 64.0, "background": "#5c94fc"},
		Children: []*model.Node{n}}
	app := &model.App{
		Entry:   "main",
		BaseDir: "../../../examples/mario",
		Scenes:  map[string]*model.Node{"main": root},
	}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	s := NewHeadlessSurface(image.Pt(64, 64))
	e.DrawFrame(s)

	imgs, _, _ := GraphImageCount(e.Graph())
	if imgs != 1 {
		t.Fatalf("tilemap graph images = %d, want 1 baked bitmap", imgs)
	}
	first := bakeTilemap(n, rt, 1)
	if first == nil {
		t.Fatal("bake returned nil")
	}
	second := bakeTilemap(n, rt, 1)
	if first != second {
		t.Fatal("unchanged rows must reuse the baked bitmap")
	}

	rt.State["bumpT"] = 1.0 // unused — bump is on node props
	n.Props["rows"] = []any{"111", "..."}
	third := bakeTilemap(n, rt, 1)
	if third == nil || third == first {
		t.Fatal("row change must rebake")
	}
}

func TestTilemapAtlasOrderDoesNotInvalidate(t *testing.T) {
	ResetTilemapCache()
	t.Cleanup(ResetTilemapCache)
	n := &model.Node{Type: "tilemap", ID: "tm",
		Props: map[string]any{
			"rows": []any{"12q"},
			"cell": 32.0,
			"atlas": map[string]any{
				"1": "assets/ground.png",
				"2": "assets/brick.png",
				"q": "assets/question.png",
			},
		},
	}
	app := &model.App{Entry: "main", BaseDir: "../../../examples/mario",
		Scenes: map[string]*model.Node{"main": {Type: "box"}}}
	rt := runtime.New(app)
	a := bakeTilemap(n, rt, 1)
	before := tilemapBakeCount()
	for i := 0; i < 20; i++ {
		b := bakeTilemap(n, rt, 1)
		if b != a {
			t.Fatal("stable rows+atlas must reuse the bake")
		}
	}
	if tilemapBakeCount() != before {
		t.Fatalf("atlas map iteration rebaked %d extra times", tilemapBakeCount()-before)
	}
}

func TestMarioUsesBakedTilemap(t *testing.T) {
	e, surf, _ := marioV2Fixture(t)
	e.DrawFrame(surf)
	var found bool
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || found {
			return
		}
		if n.Type == "tilemap" && n.ID == "tiles" {
			found = true
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, sc := range e.RT.App.Scenes {
		walk(sc)
	}
	if !found {
		t.Fatal("mario scene must declare a tilemap (whole-level bake)")
	}
	imgs, _, _ := GraphImageCount(e.Graph())
	if imgs < 1 {
		t.Fatal("baked tilemap produced no image")
	}
	if imgs > 8 {
		t.Fatalf("mario graph images = %d, tilemap should collapse the level to one bitmap", imgs)
	}
}
