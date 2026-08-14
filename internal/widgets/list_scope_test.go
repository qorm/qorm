package widgets

// A card inside a list template must measure with the item scope: its
// background has to cover the EVALUATED label, not the empty binding
// (pre-fix the card collapsed to padding width while the text overflowed).

import (
	"image"
	"testing"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/op"
	"github.com/qorm/platform/internal/render/canvas"
	"github.com/qorm/platform/internal/render/graph"
	"github.com/qorm/platform/internal/runtime"
)

func TestCardInListMeasuresWithItemScope(t *testing.T) {
	label := "a fairly long item label"
	app := loader.FromDocs([]map[string]any{
		{"type": "app", "id": "t", "entry": "main",
			"globalState": map[string]any{"initial": map[string]any{"items": []any{map[string]any{"label": label}}}}},
		{"type": "scene", "id": "main", "root": map[string]any{"type": "column", "id": "root",
			"children": []any{
				map[string]any{"type": "list", "id": "l", "data": "{{state.items}}",
					"renderItem": map[string]any{"type": "card", "id": "c",
						"children": []any{map[string]any{"type": "text", "text": "{{item.label}}"}}}},
			}}},
	})
	rt := runtime.New(app)
	ops := &op.Ops{}
	g, _ := canvas.Layout(ops, app.EntryRoot(), image.Pt(400, 400), rt, nil, 1)

	var card graph.Node
	var walk func(n graph.Node)
	walk = func(n graph.Node) {
		if n == nil {
			return
		}
		if m := n.Base().Model; m != nil && m.Type == "card" {
			card = n
			return
		}
		if gr, ok := n.(*graph.Group); ok {
			for _, c := range gr.Children {
				walk(c)
			}
		}
	}
	walk(g)
	if card == nil {
		t.Fatal("card not found in the rendered graph")
	}
	cardW := card.GetBBox().MaxX - card.GetBBox().MinX
	textW := canvas.MeasureText(label, 14)
	if cardW < textW {
		t.Errorf("card width %v < evaluated label width %v — scope did not reach Measure", cardW, textW)
	}
}
