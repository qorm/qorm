package canvas

import (
	"sync"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
)

// Widget is the contract between the engine and one UI component type —
// built-in (internal/widgets) or app-defined (a custom component registered
// by the app's Go middle layer). The engine measures and records widgets
// ONLY through this seam; widget code draws only via the draw layer
// (internal/render/draw, aliasing graph), never the raster internals.
//
// v1 contract: a widget is a LEAF. Measure reports its content size;
// Record returns the shape to mount (nil to draw nothing). Generic engine
// features apply without any widget cooperation: style parsing (parseStyle),
// margins/padding/explicit sizes, conditional rendering (if/visible/when),
// disabled suppression, and onPress dispatch (canPress is type-agnostic).
// Children of a widget node flow through the normal container layout but do
// not count toward the widget's own content size.
type Widget interface {
	// Measure reports the widget's content size in physical px at scale.
	Measure(n *model.Node, rt *runtime.Runtime, scale int) (w, h int)
	// Record builds the shape for the laid-out widget (ln carries the
	// resolved box and style), or nil to draw nothing.
	Record(ln *LayoutNode, rt *runtime.Runtime, scale int) graph.Node
}

var (
	widgetsMu sync.RWMutex
	widgets   = map[string]Widget{}
)

// RegisterWidget makes a widget type available to scenes ({"type": name}).
// It is the custom-component seam: an app's Go middle layer calls it (or
// imports internal/widgets, whose init registers the built-ins). Re-
// registering a name replaces it; engine built-in types (text, button, …)
// still win — a registration cannot shadow them in v1.
func RegisterWidget(typ string, w Widget) {
	if typ == "" || w == nil {
		return
	}
	widgetsMu.Lock()
	defer widgetsMu.Unlock()
	widgets[typ] = w
}

// LookupWidget finds a registered widget by scene type.
func LookupWidget(typ string) (Widget, bool) {
	widgetsMu.RLock()
	defer widgetsMu.RUnlock()
	w, ok := widgets[typ]
	return w, ok
}
