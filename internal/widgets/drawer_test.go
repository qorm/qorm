package widgets

import (
	"image"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/canvas"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// A bound-open drawer mounts as an overlay; a press on the backdrop closes it
// (writes the bound state back to false), while the panel's own content area
// passes presses through to the panel.
func TestDrawerOpenAndBackdropClose(t *testing.T) {
	dr := &model.Node{Type: "drawer", ID: "dr", Props: map[string]any{"open": "{{state.open}}"},
		Children: []*model.Node{{Type: "text", ID: "t", Props: map[string]any{"text": "Content"}}}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{dr}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	rt.State["open"] = true
	e := canvas.NewEngine(rt, canvas.SoftwareRenderer{})
	surf := canvas.NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)
	w, _ := canvas.LookupWidget("drawer")
	drW := w.(*Drawer)

	if !drW.OverlayOpen(dr, rt) {
		t.Fatal("a bound-open drawer must be mounted")
	}
	// The panel is anchored right (min(80%,320) wide on the 400px stage): a
	// press at the far left is the backdrop.
	e.HandlePointer(canvas.PointerInput{Type: canvas.PointerPress, X: 10, Y: 200})
	if rt.State["open"] != false {
		t.Errorf("a backdrop press must close the drawer (write open=false), open = %v", rt.State["open"])
	}
	e.MarkDirty()
	e.DrawFrame(surf)
	if drW.OverlayOpen(dr, rt) {
		t.Error("after closing, the drawer must not be mounted")
	}
}
