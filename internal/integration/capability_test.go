package integration

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/capability"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render"
	qrt "github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/server"
)

// TestCapabilityWiring is the headless self-verify harness for capabilities. For
// EVERY entry in the registry it asserts the cross-layer wiring exists — without
// a device — so an AI can verify a new capability itself:
//   - the widget type is handled by render (it does NOT fall to the unknown
//     marker), and
//   - its qormOn<Stem> result callback is defined in the page JS.
//
// It is registry-driven: adding a capability automatically extends this test, so
// a capability that is registered but not wired (or wired but not registered)
// fails here rather than silently shipping a no-op. This is the north star —
// the framework catches its own drift with no human and no hardware.
func TestCapabilityWiring(t *testing.T) {
	kids := make([]*model.Node, 0, len(capability.All))
	for _, c := range capability.All {
		kids = append(kids, &model.Node{Type: c.Widget, ID: c.Widget + "_cap"})
	}
	root := &model.Node{Type: "scaffold", ID: "root", Children: []*model.Node{
		{Type: "column", ID: "body", Children: kids},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := qrt.New(app)
	html := render.Render(rt).HTML
	page := server.Page(rt, html, 0)

	for _, c := range capability.All {
		if strings.Contains(html, `data-qorm-unknown="`+c.Widget+`"`) {
			t.Errorf("capability %q (widget %q): NOT wired in render.go — renders as the unknown-widget marker", c.Stem, c.Widget)
		}
		if c.Callback != "" && !strings.Contains(page, c.Callback) {
			t.Errorf("capability %q: result callback %s() is not defined in the page JS", c.Stem, c.Callback)
		}
	}
}
