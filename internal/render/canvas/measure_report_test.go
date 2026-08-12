package canvas

import (
	"encoding/json"
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func TestCollectMeasureReportsIDs(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "hello", Text: "Hello canvas"},
		{Type: "button", ID: "cta", Label: "Go"},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 300))
	e.DrawFrame(surf)

	raw := e.CollectMeasure()
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("CollectMeasure JSON: %v (%s)", err, raw)
	}
	byID := map[string]map[string]any{}
	for _, r := range rows {
		id, _ := r["id"].(string)
		byID[id] = r
	}
	for _, id := range []string{"root", "hello", "cta"} {
		r, ok := byID[id]
		if !ok {
			t.Fatalf("missing measured id %q in %s", id, raw)
		}
		if r["visible"] != true {
			t.Errorf("%s visible = %v, want true", id, r["visible"])
		}
		if r["w"].(float64) <= 0 || r["h"].(float64) <= 0 {
			t.Errorf("%s box w/h = %v/%v", id, r["w"], r["h"])
		}
	}
	if got := byID["hello"]["text"]; got != "Hello canvas" {
		t.Errorf("hello text = %v", got)
	}
}

func TestMeasureSceneHeadless(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "text", ID: "t", Text: "x"},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	raw := MeasureScene(rt, 400, 200, 1)
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 {
		t.Fatalf("MeasureScene: %v %s", err, raw)
	}
}

func TestCollectMeasureLogicalHiDPI(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Style: map[string]any{
		"width": 200.0, "height": 100.0, "background": "#ff0000", "padding": 8.0,
	}, Children: []*model.Node{
		{Type: "text", ID: "hello", Text: "Hi", Style: map[string]any{"fontSize": 16.0, "color": "#00ff00"}},
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	// Physical 800x400 at scale 2 → logical 400x200 stage.
	surf := NewHeadlessSurface(image.Pt(800, 400))
	// RenderInto uses size in physical px and scale.
	e.RenderInto(image.Pt(800, 400), 2, surf.Frame())

	phys := e.CollectMeasureOpts(MeasureOpts{Logical: false})
	logi := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var pr, lr []map[string]any
	if err := json.Unmarshal(phys, &pr); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(logi, &lr); err != nil {
		t.Fatal(err)
	}
	by := func(rows []map[string]any) map[string]map[string]any {
		m := map[string]map[string]any{}
		for _, r := range rows {
			m[r["id"].(string)] = r
		}
		return m
	}
	pb, lb := by(pr), by(lr)
	// Physical width ~2× logical for the same node.
	if pb["root"]["w"].(float64) < lb["root"]["w"].(float64)*1.5 {
		t.Fatalf("physical w=%v should be ~2x logical w=%v", pb["root"]["w"], lb["root"]["w"])
	}
	// Style fields present.
	if fs, _ := lb["hello"]["fontSize"].(string); fs == "" {
		t.Error("logical measure must include fontSize")
	}
	if bg, _ := lb["root"]["background"].(string); bg == "" || bg == "rgba(0, 0, 0, 0)" {
		t.Errorf("root background = %q, want red-ish", bg)
	}
	if st, ok := lb["__stage"]; !ok || st["w"].(float64) != 400 {
		t.Errorf("logical stage w = %v, want 400", st)
	}
}

func TestCollectMeasureEnrichedStyles(t *testing.T) {
	root := &model.Node{Type: "column", ID: "box", Style: map[string]any{
		"width": 120.0, "height": 60.0, "background": "#112233",
		"borderRadius": 8.0, "padding": 10.0,
	}}
	rt := runtime.New(&model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}})
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 300))
	e.DrawFrame(surf)
	raw := e.CollectMeasureOpts(MeasureOpts{Logical: true})
	var rows []map[string]any
	_ = json.Unmarshal(raw, &rows)
	var box map[string]any
	for _, r := range rows {
		if r["id"] == "box" {
			box = r
		}
	}
	if box == nil {
		t.Fatal("missing box row")
	}
	for _, k := range []string{"background", "padding", "borderRadius", "opacity"} {
		if box[k] == nil || box[k] == "" {
			t.Errorf("missing style field %q in %v", k, box)
		}
	}
}
