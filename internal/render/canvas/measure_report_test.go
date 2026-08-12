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
