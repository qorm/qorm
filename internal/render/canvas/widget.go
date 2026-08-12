package canvas

import (
	"image"
	"sort"
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
//
// Measure's vars carry the list-instance scope ({{item.x}} etc.) when the
// widget sits inside a repeat template — nil otherwise. A widget that
// measures its own subtree (card, tabs) must thread it through
// MeasureScoped or its children's bindings evaluate empty.
type Widget interface {
	// Measure reports the widget's content size in physical px at scale.
	Measure(n *model.Node, rt *runtime.Runtime, vars map[string]any, scale int) (w, h int)
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

// RegisteredWidgetNames returns a sorted snapshot of types currently in the
// widget registry (built-ins from internal/widgets plus any app-defined
// registrations). For tests and diagnostics — not a hot path.
func RegisteredWidgetNames() []string {
	widgetsMu.RLock()
	defer widgetsMu.RUnlock()
	names := make([]string, 0, len(widgets))
	for k := range widgets {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// InteractiveWidget is an OPTIONAL extension for widgets that handle their
// own pointer events (toggle, drag, tab switching). The engine routes events
// for the widget's nodes BEFORE its generic press/hover handling; while a
// widget node is pressed it captures the whole stream until release (drag
// semantics — the widget sets inter.Pressed itself on PointerPress to take
// capture). Returning redraw=true requests a frame. A consumed event never
// reaches generic hover/press dispatch: the widget owns its nodes' input
// (v1: no keyboard routing yet).
type InteractiveWidget interface {
	Widget
	// frame is the widget node's rendered bounds in physical pixels — the
	// widget maps event coordinates into its own layout (button regions,
	// drag tracks) without stashing geometry in Record.
	HandlePointer(n *model.Node, rt *runtime.Runtime, p PointerInput, inter *Interaction, frame image.Rectangle) (redraw bool)
}

// SinksInter returns the interaction forwarded through the sinks, or nil when
// sinks is nil — a ChildLayoutWidget's public Record path calls
// PerformLayoutWithSinks with nil sinks (layout-only callers, widget tests),
// so the panel layout must not dereference sinks blindly.
func SinksInter(sinks *LayoutSinks) *Interaction {
	if sinks == nil {
		return nil
	}
	return sinks.Inter
}

// DropTargetWidget is an optional interface an InteractiveWidget implements
// to declare itself a drag-and-drop drop zone (dragtarget/droptarget): while a
// cross-panel drag is in flight, the engine routes a release to the nearest
// drop-target ancestor even when an inner interactive widget is closer — a
// drop zone must not be swallowed by its own children.
type DropTargetWidget interface {
	Widget
	DropTarget()
}

// FocusHookWidget is an optional interface an InteractiveWidget may implement
// to learn when KEYBOARD focus lands on it — the Tab/Shift-Tab path, which
// never routes through HandlePointer. A widget that caches the engine's
// Interaction for its Record (the textarea's live edit session + scroll
// offset) needs this hook or a Tab-focused instance renders without its
// session. Pointer focus already reaches the widget via HandlePointer.
type FocusHookWidget interface {
	Widget
	OnFocused(n *model.Node, inter *Interaction)
}

// OverlayWidget marks a widget whose popup should paint above its siblings
// while it is open (for example, a select menu). The engine appends the
// returned overlay node after normal layout, so the popup draws and hit-tests
// on top without a global overlay manager.
type OverlayWidget interface {
	Widget
	OverlayOpen(n *model.Node, rt *runtime.Runtime) bool
	OverlayRecord(ln *LayoutNode, rt *runtime.Runtime, scale int, origin image.Point) graph.Node
}

// LayoutSinks carries the two side channels a frame-level layout allocates:
// the repeat-instance sidecar (list.go) and the popup overlays (OverlayWidget).
// Container widgets that lay out children themselves receive it and forward
// it into PerformLayoutWithSinks, so widgets NESTED inside their panels (a
// select inside a tab) still register overlays and item identities with the
// frame. Widget code only forwards the pointer; the engine owns the contents.
type LayoutSinks struct {
	items    map[graph.Node]itemInstance
	overlays *[]graph.Node
	// Inter is the frame's interaction state, forwarded to a ChildLayoutWidget
	// that lays out panel children itself (tabs, scaffold, animatedopacity,
	// …) so hover/focus reach those children — they would otherwise be laid
	// out with nil interaction and their rings/hover styles would never show.
	Inter *Interaction
}

// ChildLayoutWidget is an OPTIONAL extension for container widgets that drive
// their own child layout through PerformLayout (tabs' active panel,
// animatedcontainer's opacity group). The engine calls RecordWithSinks instead
// of Record, handing over the frame's sinks; a Record that calls the plain
// PerformLayout drops them, which silently breaks overlays (and repeat-item
// identity) for everything nested in the panel.
type ChildLayoutWidget interface {
	Widget
	RecordWithSinks(ln *LayoutNode, rt *runtime.Runtime, scale int, sinks *LayoutSinks) graph.Node
}

// InlineWidget marks a widget that is inline-level in the CSS sense
// (avatar, badge, icon, switch, …): a flex container must NOT cross-stretch
// it (HTML renders those inline-flex, so they keep their content width even
// when the parent stretches block-level siblings).
type InlineWidget interface {
	Widget
	Inline()
}

// AnimatedWidget is an OPTIONAL extension for widgets that animate
// continuously (an indeterminate spinner never settles). While any of its
// nodes is mounted in the current scene the engine keeps the frame loop
// alive and calls Record every frame — the widget advances its own clock in
// Measure/Record.
type AnimatedWidget interface {
	Widget
	Animating() bool
}

// KeyWidget is an OPTIONAL extension for widgets that handle keyboard input
// (games, rich editors). The engine routes key events to it while one of its
// nodes holds focus — a press on the widget focuses it (pointer semantics:
// no focus ring), Escape blurs back out to the generic path. Returning
// consumed=true keeps the event from the generic handlers (tab/return/
// escape); redraw=true requests a frame.
type KeyWidget interface {
	Widget
	HandleKey(n *model.Node, rt *runtime.Runtime, k KeyInput, inter *Interaction) (consumed, redraw bool)
}
