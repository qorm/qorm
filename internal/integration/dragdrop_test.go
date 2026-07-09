package integration

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
)

// TestDragDropExample exercises the examples/dragdrop kanban end to end: the
// scene renders three drop zones full of draggable cards (each carrying its id
// as the drag payload), and dropping a card — the server posts {_dragData} and
// fires the column's action — moves it via state.updateWhere so the shared state
// reflects the new column. This is the integration proof behind the isolated
// Draggable/DragTarget render test.
func TestDragDropExample(t *testing.T) {
	app, err := loader.LoadDir("../../examples/dragdrop")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(app.Diagnostics) > 0 {
		t.Fatalf("diagnostics: %v", app.Diagnostics)
	}
	rt := qrt.New(app)
	html := render.RenderScene(rt, rt.CurrentScene()).HTML
	for _, m := range []string{
		`class="qorm-droptarget" data-qorm-drop="`,   // drop zones carry their handler
		`data-qorm-drag="t1"`, `data-qorm-drag="t3"`, // cards carry their id payload
		">Design the icon<", ">Ship v0.2.2<", // card labels render
	} {
		if !strings.Contains(html, m) {
			t.Errorf("dragdrop render missing %q", m)
		}
	}
	// drop t1 (a To Do card) onto the Doing column
	rt.Dispatch("moveDoing", map[string]any{"_dragData": "t1"})
	got := ""
	for _, it := range rt.State["items"].([]any) {
		if m := it.(map[string]any); m["id"] == "t1" {
			got, _ = m["col"].(string)
		}
	}
	if got != "doing" {
		t.Errorf("after drop, t1.col = %q, want \"doing\"", got)
	}
}
