package canvas

import (
	"image"
	"image/color"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/render/graph"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// inputFixture builds a headless engine around one centered input bound to
// state.name, mirroring engineFixture for buttons.
func inputFixture(t *testing.T) (*Engine, *HeadlessSurface, *model.Node) {
	t.Helper()
	in := &model.Node{Type: "input", ID: "in1", Value: "{{state.name}}", Placeholder: "Your name"}
	root := &model.Node{
		Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	surf := NewHeadlessSurface(image.Pt(400, 400))
	return NewEngine(rt, SoftwareRenderer{}), surf, in
}

func inputGroup(t *testing.T, e *Engine, in *model.Node) *graph.Group {
	t.Helper()
	g, ok := e.findGroupByModel(in).(*graph.Group)
	if !ok {
		t.Fatal("input group not found in rendered graph")
	}
	return g
}

func clickNode(t *testing.T, e *Engine, n *model.Node) {
	t.Helper()
	cx, cy := buttonCenter(t, e, n)
	e.HandlePointer(PointerInput{Type: PointerPress, X: float64(cx), Y: float64(cy)})
	e.HandlePointer(PointerInput{Type: PointerRelease, X: float64(cx), Y: float64(cy)})
}

// typeRunes feeds printable text through the host's Rune channel.
func typeRunes(e *Engine, s string) {
	for _, r := range s {
		e.HandleKey(KeyInput{Key: "rune", Rune: r, Down: true})
	}
}

func firstRect(g *graph.Group) *graph.Rect {
	for _, c := range g.Children {
		if r, ok := c.(*graph.Rect); ok {
			return r
		}
	}
	return nil
}

func firstText(g *graph.Group) *graph.Text {
	for _, c := range g.Children {
		if t, ok := c.(*graph.Text); ok {
			return t
		}
	}
	return nil
}

// findCaret locates the edit caret: the NoHit rect with a fill and no stroke
// (the focus ring is the other NoHit rect — it has a stroke, no fill).
func findCaret(g *graph.Group) *graph.Rect {
	for _, c := range g.Children {
		r, ok := c.(*graph.Rect)
		if ok && r.Base().NoHit && r.Fill.A > 0 && r.StrokeWidth == 0 {
			return r
		}
	}
	return nil
}

func findRing(g *graph.Group) *graph.Rect {
	for _, c := range g.Children {
		r, ok := c.(*graph.Rect)
		if ok && r.Base().NoHit && r.StrokeWidth == 2 {
			return r
		}
	}
	return nil
}

// An empty input keeps a usable default size: min content width plus the
// theme padding (12), one text line tall (fs 16).
func TestInputMeasuresDefaultSize(t *testing.T) {
	rt := rtWithDefaultTheme(t)
	in := &model.Node{Type: "input", ID: "i", Placeholder: "Your name"}
	ln := Measure(in, rt, nil, 1)
	if want := minInputWidth + 24; ln.Width != want {
		t.Errorf("empty input width = %d, want min %d", ln.Width, want)
	}
	if want := 19 + 24; ln.Height != want { // 19 = int(16 * 1.2)
		t.Errorf("input height = %d, want one text line + padding %d", ln.Height, want)
	}
	if !ln.Placeholder || ln.Text != "Your name" {
		t.Errorf("empty input must show its placeholder, got text=%q placeholder=%v", ln.Text, ln.Placeholder)
	}
}

// The input paints with the theme's inputBg fill / inputBorder stroke; with
// no value the placeholder shows in the secondary color, and a bound value
// replaces it in the text color. Verified down to the pixels.
func TestInputRendersPlaceholderAndValue(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	g := inputGroup(t, e, in)

	bg := firstRect(g)
	if bg == nil {
		t.Fatal("input must paint a background rect")
	}
	if want := (color.RGBA{232, 232, 237, 255}); bg.Fill != want {
		t.Errorf("input background = %v, want theme inputBg %v", bg.Fill, want)
	}
	if want := (color.RGBA{198, 198, 200, 255}); bg.Stroke != want {
		t.Errorf("input border = %v, want theme inputBorder %v", bg.Stroke, want)
	}

	txt := firstText(g)
	if txt == nil || txt.Content != "Your name" {
		t.Fatalf("empty input must render its placeholder, got %+v", txt)
	}
	if want := (color.RGBA{134, 134, 139, 255}); txt.Fill != want {
		t.Errorf("placeholder color = %v, want secondary text color %v", txt.Fill, want)
	}

	bb := g.GetBBox()
	if got := surf.Frame().RGBAAt(int(bb.MinX)+4, int((bb.MinY+bb.MaxY)/2)); got != bg.Fill {
		t.Errorf("pixel inside the box = %v, want the inputBg fill %v", got, bg.Fill)
	}

	// A bound value replaces the placeholder in the regular text color.
	e.RT.State["name"] = "Ada"
	e.MarkDirty()
	e.DrawFrame(surf)
	txt = firstText(inputGroup(t, e, in))
	if txt == nil || txt.Content != "Ada" {
		t.Fatalf("bound value must render, got %+v", txt)
	}
	if want := (color.RGBA{29, 29, 31, 255}); txt.Fill != want {
		t.Errorf("value color = %v, want theme text color %v", txt.Fill, want)
	}
}

// Clicking an input focuses it (no focus ring — pointer semantics) and opens
// the edit session; clicking blank space blurs and closes it.
func TestInputClickFocusesAndBlankBlurs(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)

	clickNode(t, e, in)
	if e.Inter.Focused != in {
		t.Fatalf("click must focus the input, focused=%v", e.Inter.Focused)
	}
	if e.Inter.FocusVisible {
		t.Error("pointer-driven focus must not show the keyboard ring")
	}
	s := e.Inter.Input
	if s == nil || s.Node != in {
		t.Fatal("click must open the edit session")
	}
	if len(s.Runes) != 0 || s.Cursor != 0 {
		t.Errorf("fresh session = %q@%d, want empty buffer, cursor 0", string(s.Runes), s.Cursor)
	}

	e.HandlePointer(PointerInput{Type: PointerPress, X: 5, Y: 395})
	if e.Inter.Focused != nil {
		t.Error("blank press must blur the input (HTML parity)")
	}
	if e.Inter.Input != nil {
		t.Error("blank press must close the edit session")
	}
}

// A session on a populated binding starts from the current value with the
// cursor at the end, like clicking into an HTML field.
func TestInputEditSessionStartsFromBoundValue(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.RT.State["name"] = "Ada"
	e.DrawFrame(surf)

	clickNode(t, e, in)
	s := e.Inter.Input
	if s == nil {
		t.Fatal("click must open the edit session")
	}
	if got := string(s.Runes); got != "Ada" {
		t.Errorf("session buffer = %q, want the bound value %q", got, "Ada")
	}
	if s.Cursor != 3 {
		t.Errorf("cursor = %d, want at the end (3)", s.Cursor)
	}
	typeRunes(e, "!")
	if got := e.RT.State["name"]; got != "Ada!" {
		t.Errorf("state.name = %v, want %q", got, "Ada!")
	}
}

// Every keystroke writes the buffer back to the bound state path and flags a
// redraw; the next frame renders the new text.
func TestInputTypingWritesBackState(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)

	typeRunes(e, "ab")
	s := e.Inter.Input
	if got := string(s.Runes); got != "ab" || s.Cursor != 2 {
		t.Fatalf("buffer = %q@%d, want \"ab\"@2", got, s.Cursor)
	}
	if got := e.RT.State["name"]; got != "ab" {
		t.Errorf("state.name = %v, want %q (two-way binding write-back)", got, "ab")
	}
	if !e.Dirty() {
		t.Error("an edit must request a redraw")
	}
	e.DrawFrame(surf)
	if txt := firstText(inputGroup(t, e, in)); txt == nil || txt.Content != "ab" {
		t.Fatalf("frame after the edit must render the buffer, got %+v", txt)
	}
}

// Hosts that do not fill KeyInput.Rune yet still type ASCII via the
// normalized key names; the Rune channel wins when both are present.
func TestInputNamedKeyFallbackTypes(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)

	e.HandleKey(KeyInput{Key: "a", Down: true})
	e.HandleKey(KeyInput{Key: "b", Shift: true, Down: true})
	e.HandleKey(KeyInput{Key: "1", Down: true})
	e.HandleKey(KeyInput{Key: "space", Down: true})
	e.HandleKey(KeyInput{Key: "z", Rune: 'q', Down: true})
	if got := e.RT.State["name"]; got != "aB1 q" {
		t.Errorf("state.name = %v, want %q", got, "aB1 q")
	}
}

// Backspace ("delete" on macOS) removes the rune before the cursor; emptying
// the field brings the placeholder back, never the stale bound value.
func TestInputBackspace(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)

	typeRunes(e, "ab")
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := e.RT.State["name"]; got != "a" {
		t.Errorf("after one backspace state.name = %v, want %q", got, "a")
	}
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := e.RT.State["name"]; got != "" {
		t.Errorf("empty buffer must write back an empty string, got %v", got)
	}
	// At the start: a no-op, not a panic.
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := string(e.Inter.Input.Runes); got != "" {
		t.Errorf("buffer = %q, want still empty", got)
	}

	e.DrawFrame(surf)
	if txt := firstText(inputGroup(t, e, in)); txt == nil || txt.Content != "Your name" {
		t.Errorf("emptied field must show the placeholder again, got %+v", txt)
	}
}

// Left/right move the insertion point; edits land at the cursor, ends clamp.
func TestInputCursorMovement(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)

	typeRunes(e, "abc")
	e.HandleKey(KeyInput{Key: "left", Down: true})
	if c := e.Inter.Input.Cursor; c != 2 {
		t.Fatalf("cursor = %d, want 2", c)
	}
	typeRunes(e, "X")
	if got := e.RT.State["name"]; got != "abXc" {
		t.Fatalf("insert at cursor: state.name = %v, want %q", got, "abXc")
	}
	e.HandleKey(KeyInput{Key: "left", Down: true})
	e.HandleKey(KeyInput{Key: "left", Down: true})
	e.HandleKey(KeyInput{Key: "right", Down: true})
	e.HandleKey(KeyInput{Key: "delete", Down: true})
	if got := e.RT.State["name"]; got != "aXc" {
		t.Errorf("backspace at cursor: state.name = %v, want %q", got, "aXc")
	}
	// Ends clamp: left at 0 and right at len are no-ops.
	for i := 0; i < 5; i++ {
		e.HandleKey(KeyInput{Key: "left", Down: true})
	}
	if c := e.Inter.Input.Cursor; c != 0 {
		t.Errorf("cursor after clamping left = %d, want 0", c)
	}
	for i := 0; i < 5; i++ {
		e.HandleKey(KeyInput{Key: "right", Down: true})
	}
	if c := e.Inter.Input.Cursor; c != 3 {
		t.Errorf("cursor after clamping right = %d, want 3", c)
	}
}

// The caret sits at padding + the width of the text before the cursor, and
// disappears with the session.
func TestInputCaretFollowsCursor(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "ab")
	e.HandleKey(KeyInput{Key: "left", Down: true}) // cursor 1
	e.DrawFrame(surf)

	caret := findCaret(inputGroup(t, e, in))
	if caret == nil {
		t.Fatal("an editing input must paint the caret")
	}
	// Theme: padding 12, fontSize 16 → "a" is 9.6px wide (truncated to 9).
	if want := float64(12 + int(MeasureText("a", 16))); caret.X != want {
		t.Errorf("caret x = %v, want padding + prefix width %v", caret.X, want)
	}
	if caret.Width != 1 {
		t.Errorf("caret width = %v, want 1 device px at scale 1", caret.Width)
	}
	if caret.Height != 19 { // 19 = int(16 * 1.2)
		t.Errorf("caret height = %v, want one text line %v", caret.Height, 19)
	}

	e.HandleKey(KeyInput{Key: "escape", Down: true})
	e.DrawFrame(surf)
	if findCaret(inputGroup(t, e, in)) != nil {
		t.Error("caret must disappear when the session ends")
	}
}

// A declared onChange dispatches per edit with the new {value}; the bound
// state write-back still happens in parallel (HTML qorm() semantics).
func TestInputOnChangeDispatchesWithValue(t *testing.T) {
	e, surf, in := inputFixture(t)
	in.OnChange = &model.Invoke{Name: "changed"}
	e.RT.App.Actions = map[string]*model.Action{
		"changed": {ID: "changed", Steps: []model.Step{{Type: "state.set", Path: "seen", Value: "{{ value }}"}}},
	}
	e.DrawFrame(surf)
	clickNode(t, e, in)

	typeRunes(e, "xy")
	if got := e.RT.State["seen"]; got != "xy" {
		t.Errorf("onChange value = %v, want %q", got, "xy")
	}
	if got := e.RT.State["name"]; got != "xy" {
		t.Errorf("state.name = %v, want %q (write-back parallel to onChange)", got, "xy")
	}
}

// Keyboard focus opens the edit session too, and shows both the focus ring
// (:focus-visible) and the caret; Escape blurs and closes the session.
func TestInputTabFocusShowsRingAndCaret(t *testing.T) {
	e, surf, in := inputFixture(t)
	e.DrawFrame(surf)

	e.HandleKey(KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != in || !e.Inter.FocusVisible {
		t.Fatalf("tab must focus the input with a visible ring (focused=%v visible=%v)", e.Inter.Focused, e.Inter.FocusVisible)
	}
	if e.Inter.Input == nil {
		t.Fatal("keyboard focus must open the edit session")
	}
	e.DrawFrame(surf)
	g := inputGroup(t, e, in)
	if findRing(g) == nil {
		t.Error("keyboard focus must draw the focus ring")
	}
	if findCaret(g) == nil {
		t.Error("a keyboard-focused input must draw the caret")
	}

	e.HandleKey(KeyInput{Key: "escape", Down: true})
	if e.Inter.Focused != nil || e.Inter.Input != nil {
		t.Error("escape must blur and close the edit session")
	}
}

// Inputs join the tab order by type; disabled ones leave it.
func TestInputFocusableMembership(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "input", ID: "i1"},
		{Type: "button", ID: "b1"},
		{Type: "input", ID: "i2", Style: map[string]any{"disabled": true}},
		{Type: "text", ID: "t1"},
	}}
	got := ids(Focusables(root, nil))
	want := []string{"i1", "b1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Focusables = %v, want %v", got, want)
	}
}

// A disabled input is not clickable, not in the tab order and not editable.
func TestInputDisabledSkipsFocusAndEditing(t *testing.T) {
	e, surf, in := inputFixture(t)
	in.Style = map[string]any{"disabled": true}
	e.DrawFrame(surf)

	clickNode(t, e, in)
	if e.Inter.Focused != nil {
		t.Error("disabled input must not take focus")
	}
	if e.Inter.Input != nil {
		t.Error("disabled input must not open an edit session")
	}
	e.HandleKey(KeyInput{Key: "tab", Down: true})
	if e.Inter.Focused != nil {
		t.Error("disabled input must leave the tab order")
	}
	typeRunes(e, "zz")
	if v := e.RT.State["name"]; v != nil {
		t.Errorf("disabled input edited state (name=%v), want untouched", v)
	}
}

// When a condition flip hides the focused input, the session dies and further
// keys no longer edit.
func TestInputHiddenByConditionStopsEditing(t *testing.T) {
	e, surf, in := inputFixture(t)
	in.Props = map[string]any{"if": "{{state.show}}"}
	e.RT.State["show"] = true
	e.DrawFrame(surf)
	clickNode(t, e, in)
	typeRunes(e, "ab")
	if got := e.RT.State["name"]; got != "ab" {
		t.Fatalf("state.name = %v, want %q", got, "ab")
	}

	e.RT.State["show"] = false
	e.MarkDirty()
	e.DrawFrame(surf)
	if g := e.findGroupByModel(in); g != nil {
		t.Fatal("a conditionally hidden input must not render")
	}
	e.HandleKey(KeyInput{Key: "rune", Rune: 'c', Down: true})
	if got := e.RT.State["name"]; got != "ab" {
		t.Errorf("hidden input must not edit: state.name = %v, want %q", got, "ab")
	}
	if e.Inter.Input != nil {
		t.Error("the edit session must end when its node leaves the tree")
	}
}

// The two-way binding write-back honors the runtime's read-only computed
// namespace (SetStatePath): edits proceed locally, the derived value stays
// published from its declaration.
func TestInputComputedNamespaceRefusesWriteBack(t *testing.T) {
	in := &model.Node{Type: "input", ID: "in1", Value: "{{state.computed.name}}"}
	root := &model.Node{
		Type: "column", ID: "root",
		Layout:   map[string]any{"align": "center", "justify": "center"},
		Children: []*model.Node{in},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	app.Computed = map[string]string{"name": "{{ 'derived' }}"}
	rt := runtime.New(app)
	rt.Theme = theme.GetDefault()
	e := NewEngine(rt, SoftwareRenderer{})
	surf := NewHeadlessSurface(image.Pt(400, 400))
	e.DrawFrame(surf)

	if txt := firstText(inputGroup(t, e, in)); txt == nil || txt.Content != "derived" {
		t.Fatalf("the derived value must render, got %+v", txt)
	}
	clickNode(t, e, in)
	typeRunes(e, "x")
	if got := string(e.Inter.Input.Runes); got != "derivedx" {
		t.Errorf("local edit must proceed, buffer = %q, want %q", got, "derivedx")
	}
	m, ok := rt.State["computed"].(map[string]any)
	if !ok || m["name"] != "derived" {
		t.Errorf("computed namespace overwritten: %#v, want name stayed \"derived\"", rt.State["computed"])
	}
}
