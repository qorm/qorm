package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// renderApp renders an app whose entry scene root holds the given children, with
// the supplied component definitions. Used to exercise component instantiation.
func renderApp(t *testing.T, components map[string]*model.Node, root *model.Node) Result {
	t.Helper()
	app := &model.App{
		Entry:      "main",
		Scenes:     map[string]*model.Node{"main": root},
		Components: components,
	}
	return Render(runtime.New(app))
}

// TestComponentPropScope guards that an instance's props/text/label/value become
// {{prop.x}} inside the component template, evaluated per instance.
func TestComponentPropScope(t *testing.T) {
	comps := map[string]*model.Node{
		"Field": {Type: "text", ID: "f", Text: "{{ prop.label }}={{ prop.value }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Field", ID: "f1", Label: "Name", Value: "Al"},
		{Type: "Field", ID: "f2", Label: "Age", Value: "30"},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"Name=Al", "Age=30"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component prop scope did not resolve per instance, lacks %q:\n%s", w, res.HTML)
		}
	}
	if strings.Contains(res.HTML, "{{ prop.label }}") {
		t.Errorf("unresolved binding leaked:\n%s", res.HTML)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("component instance should not be unknown: %v", res.Unknown)
	}
}

// TestComponentPropsMap guards that explicit props (n.Props) are exposed as
// {{prop.x}} alongside the text/label/value shorthand.
func TestComponentPropsMap(t *testing.T) {
	comps := map[string]*model.Node{
		"Badge2": {Type: "text", ID: "b", Text: "{{ prop.kind }}:{{ prop.text }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Badge2", ID: "b1", Text: "HELLO", Props: map[string]any{"kind": "info"}},
	}}
	res := renderApp(t, comps, root)
	if !strings.Contains(res.HTML, "info:HELLO") {
		t.Errorf("component props map + text not resolved:\n%s", res.HTML)
	}
}

// TestComponentSlot guards that an instance's children fill a {type:slot} in the
// component template, per instance.
func TestComponentSlot(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel": {Type: "card", ID: "panel", Children: []*model.Node{
			{Type: "text", ID: "title", Text: "{{ prop.title }}"},
			{Type: "slot"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel", ID: "p1", Props: map[string]any{"title": "First"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "WORLD"}}},
		{Type: "Panel", ID: "p2", Props: map[string]any{"title": "Second"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "AGAIN"}}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"First", "Second", "WORLD", "AGAIN"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component slot/title not resolved, lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestComponentIdUniqueness guards that ids inside a component template are
// suffixed per instance, so two uses of the same component never collide on
// document.getElementById.
func TestComponentIdUniqueness(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel": {Type: "card", ID: "panel", Children: []*model.Node{
			{Type: "text", ID: "title", Text: "{{ prop.title }}"},
			{Type: "slot"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel", ID: "p1", Props: map[string]any{"title": "A"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "x"}}},
		{Type: "Panel", ID: "p2", Props: map[string]any{"title": "B"},
			Children: []*model.Node{{Type: "text", ID: "body", Text: "y"}}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{`id="title_p1"`, `id="title_p2"`, `id="body_p1"`, `id="body_p2"`} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("component ids should be unique per instance, lacks %q:\n%s", w, res.HTML)
		}
	}
	// the bare (unsuffixed) template id must not appear, or lookups would collide
	if strings.Contains(res.HTML, `id="title"`) || strings.Contains(res.HTML, `id="body"`) {
		t.Errorf("unsuffixed template id leaked:\n%s", res.HTML)
	}
}

// stateApp builds an app with components, an entry scene, initial state and
// actions — for exercising the dynamic component model (props bound to state,
// callback props dispatching real actions).
func stateApp(components map[string]*model.Node, root *model.Node, initial map[string]any, actions map[string]*model.Action) *model.App {
	return &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		Components:  components,
		Actions:     actions,
		GlobalState: model.GlobalState{Initial: initial},
	}
}

// handlerByName finds the first registered handler with the given (resolved)
// action name.
func handlerByName(res Result, name string) (Handler, bool) {
	for _, h := range res.Handlers {
		if h.Name == name {
			return h, true
		}
	}
	return Handler{}, false
}

// TestComponentPropStateBinding guards that a prop value carrying a state
// binding is evaluated in the instance's scope before injection: the template
// renders the live state value, never the literal "{{state.x}}" text.
func TestComponentPropStateBinding(t *testing.T) {
	comps := map[string]*model.Node{
		"Echo": {Type: "text", ID: "e", Text: "ECHO_{{ prop.msg }}_END"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Echo", ID: "e1", Props: map[string]any{"msg": "{{state.msg}}"}},
	}}
	rt := runtime.New(stateApp(comps, root, map[string]any{"msg": "hello"}, nil))
	res := Render(rt)
	if !strings.Contains(res.HTML, "ECHO_hello_END") {
		t.Errorf("state-bound prop did not evaluate in instance scope:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "{{state.msg}}") {
		t.Errorf("literal binding leaked into output:\n%s", res.HTML)
	}
}

// TestComponentPropItemBinding guards that a component instantiated inside a
// list renderItem can take props bound to the per-item scope ({{item.x}}).
func TestComponentPropItemBinding(t *testing.T) {
	comps := map[string]*model.Node{
		"Row": {Type: "text", ID: "r", Text: "[{{ prop.name }}]"},
	}
	root := &model.Node{Type: "list", ID: "lst", Data: "{{ state.rows }}",
		Template: &model.Node{Type: "Row", ID: "row", Props: map[string]any{"name": "{{item.name}}"}}}
	rt := runtime.New(stateApp(comps, root, map[string]any{
		"rows": []any{map[string]any{"name": "Ada"}, map[string]any{"name": "Linus"}},
	}, nil))
	res := Render(rt)
	for _, w := range []string{"[Ada]", "[Linus]"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("item-bound prop not resolved per item, lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestComponentPropTypePreservation guards EvalBinding's typed path through
// props: a whole-string binding keeps its bool/number/list type (so template
// expressions can compute with it), and non-string JSON literals pass through
// typed and untouched.
func TestComponentPropTypePreservation(t *testing.T) {
	comps := map[string]*model.Node{
		"Calc": {Type: "column", ID: "c", Children: []*model.Node{
			// number prop: arithmetic works only on a real number
			{Type: "text", ID: "sum", Text: "SUM={{ prop.n + 1 }}"},
			// bool prop drives visibility with no special-casing in visible()
			{Type: "text", ID: "open", Text: "OPEN", Props: map[string]any{"if": "{{ prop.open }}"}},
			// list prop feeds a list's data binding
			{Type: "list", ID: "inner", Data: "{{ prop.rows }}",
				Template: &model.Node{Type: "text", ID: "it", Text: "<{{ item.t }}>"}},
			// JSON literals on the instance keep their types too
			{Type: "text", ID: "lit", Text: "LIT={{ prop.litNum + 1 }}/{{ prop.litFlag }}"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Calc", ID: "c1", Props: map[string]any{
			"n":       "{{state.n}}",
			"open":    "{{state.open}}",
			"rows":    "{{state.rows}}",
			"litNum":  float64(9),
			"litFlag": true,
		}},
	}}
	rt := runtime.New(stateApp(comps, root, map[string]any{
		"n": float64(41), "open": true,
		"rows": []any{map[string]any{"t": "a"}, map[string]any{"t": "b"}},
	}, nil))
	res := Render(rt)
	for _, w := range []string{"SUM=42", "OPEN", "&lt;a&gt;", "&lt;b&gt;", "LIT=10/true"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("typed prop lost its type, lacks %q:\n%s", w, res.HTML)
		}
	}
	// flip the bool: the template's if={{prop.open}} must hide the node
	rt.State["open"] = false
	if html := Render(rt).HTML; strings.Contains(html, "OPEN") {
		t.Errorf("bool prop false should hide the if-guarded node:\n%s", html)
	}
}

// TestComponentCallbackProp guards the callback-prop chain: the template's
// invoke name {{prop.onConfirm}} resolves to the instance-supplied action name
// at registration time, so the handler table carries the FINAL name and a
// dispatch through it runs the real action.
func TestComponentCallbackProp(t *testing.T) {
	comps := map[string]*model.Node{
		"Confirm": {Type: "column", ID: "d", Children: []*model.Node{
			{Type: "button", ID: "ok", Label: "OK",
				OnPress: &model.Invoke{Name: "{{prop.onConfirm}}", Args: map[string]string{}}},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Confirm", ID: "c1", Props: map[string]any{"onConfirm": "saveItem"}},
	}}
	actions := map[string]*model.Action{
		"saveItem": {ID: "saveItem", Steps: []model.Step{
			{Type: "state.set", Path: "saved", Value: "{{ true }}"},
		}},
	}
	rt := runtime.New(stateApp(comps, root, map[string]any{"saved": false}, actions))
	res := Render(rt)
	h, ok := handlerByName(res, "saveItem")
	if !ok {
		t.Fatalf("callback prop did not resolve to the final action name, handlers: %+v", res.Handlers)
	}
	rt.Dispatch(h.Name, nil) // what the server does with the registered handler
	if rt.State["saved"] != true {
		t.Errorf("dispatching the resolved callback did not run saveItem, state: %v", rt.State)
	}
}

// TestComponentNamedSlots guards named-slot distribution: each named slot takes
// only the instance children attributed to it (slot:"header"), the default slot
// takes the unattributed rest, and nothing renders twice.
func TestComponentNamedSlots(t *testing.T) {
	comps := map[string]*model.Node{
		"Frame": {Type: "column", ID: "f", Children: []*model.Node{
			{Type: "slot", ID: "sh", Props: map[string]any{"name": "header"}},
			{Type: "text", ID: "mid", Text: "MID"},
			{Type: "slot", ID: "sd"},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Frame", ID: "f1", Children: []*model.Node{
			{Type: "text", ID: "h", Text: "HEAD", Props: map[string]any{"slot": "header"}},
			{Type: "text", ID: "b", Text: "BODY"},
		}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"HEAD", "MID", "BODY"} {
		if c := strings.Count(res.HTML, w); c != 1 {
			t.Errorf("named-slot content %q should render exactly once, got %d:\n%s", w, c, res.HTML)
		}
	}
	// distribution order: header slot before MID, default slot after MID
	hi, mi, bi := strings.Index(res.HTML, "HEAD"), strings.Index(res.HTML, "MID"), strings.Index(res.HTML, "BODY")
	if !(hi < mi && mi < bi) {
		t.Errorf("slot distribution out of order (HEAD=%d MID=%d BODY=%d):\n%s", hi, mi, bi, res.HTML)
	}
}

// TestComponentSlotFallback guards that a slot's own children render as its
// default content when the instance supplies nothing for it — and are replaced
// when it does.
func TestComponentSlotFallback(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel3": {Type: "column", ID: "p", Children: []*model.Node{
			{Type: "slot", ID: "sf", Props: map[string]any{"name": "footer"},
				Children: []*model.Node{{Type: "text", ID: "fb", Text: "DEFAULT_FOOTER"}}},
		}},
	}
	// no footer content supplied: fallback renders
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel3", ID: "p1"},
	}}
	res := renderApp(t, comps, root)
	if !strings.Contains(res.HTML, "DEFAULT_FOOTER") {
		t.Errorf("empty slot should render its fallback children:\n%s", res.HTML)
	}
	// footer content supplied: fallback replaced
	root2 := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Panel3", ID: "p1", Children: []*model.Node{
			{Type: "text", ID: "f", Text: "REAL_FOOTER", Props: map[string]any{"slot": "footer"}},
		}},
	}}
	res2 := renderApp(t, comps, root2)
	if strings.Contains(res2.HTML, "DEFAULT_FOOTER") || !strings.Contains(res2.HTML, "REAL_FOOTER") {
		t.Errorf("supplied slot content should replace the fallback:\n%s", res2.HTML)
	}
}

// TestComponentUnnamedSlotBackCompat guards the original single-slot contract:
// with one unnamed slot and unattributed children, ALL instance children fill
// it, exactly as before named slots existed.
func TestComponentUnnamedSlotBackCompat(t *testing.T) {
	comps := map[string]*model.Node{
		"Wrap2": {Type: "column", ID: "w", Children: []*model.Node{{Type: "slot", ID: "s"}}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Wrap2", ID: "w1", Children: []*model.Node{
			{Type: "text", ID: "a", Text: "ONE"},
			{Type: "text", ID: "b", Text: "TWO"},
		}},
	}}
	res := renderApp(t, comps, root)
	for _, w := range []string{"ONE", "TWO"} {
		if c := strings.Count(res.HTML, w); c != 1 {
			t.Errorf("unnamed slot should render every child once, %q appeared %d times:\n%s", w, c, res.HTML)
		}
	}
}

// TestComponentNestedPropsObject guards the spec-style nested "props":{...}
// object on an instance: equivalent to top-level keys, evaluated in the
// instance scope, and winning over a conflicting top-level key.
func TestComponentNestedPropsObject(t *testing.T) {
	comps := map[string]*model.Node{
		"Info": {Type: "text", ID: "i", Text: "{{ prop.title }}|{{ prop.extra }}"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Info", ID: "i1", Props: map[string]any{
			"title": "TOP",
			"props": map[string]any{"title": "NESTED", "extra": "{{state.msg}}"},
		}},
	}}
	rt := runtime.New(stateApp(comps, root, map[string]any{"msg": "hi"}, nil))
	res := Render(rt)
	if !strings.Contains(res.HTML, "NESTED|hi") {
		t.Errorf("nested props object should win over top-level keys and evaluate bindings:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "TOP|") {
		t.Errorf("top-level key should have been overridden by the props object:\n%s", res.HTML)
	}
}

// TestComponentScenarioExpandableCard is the probe app's first stuck scenario:
// a card whose body visibility is a prop bound to state and whose toggle
// button is a callback prop. The full loop — render closed, dispatch the
// resolved callback, re-render open — must work.
func TestComponentScenarioExpandableCard(t *testing.T) {
	comps := map[string]*model.Node{
		"ExpandCard": {Type: "card", ID: "ec", Children: []*model.Node{
			{Type: "button", ID: "tog", Label: "{{ prop.title }}",
				OnPress: &model.Invoke{Name: "{{prop.onToggle}}", Args: map[string]string{}}},
			{Type: "column", ID: "body", Props: map[string]any{"if": "{{ prop.open }}"},
				Children: []*model.Node{{Type: "text", ID: "det", Text: "DETAILS"}}},
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "ExpandCard", ID: "c1", Props: map[string]any{
			"title": "More", "open": "{{state.open}}", "onToggle": "toggleCard",
		}},
	}}
	actions := map[string]*model.Action{
		"toggleCard": {ID: "toggleCard", Steps: []model.Step{
			{Type: "state.set", Path: "open", Value: "{{ state.open == false }}"},
		}},
	}
	rt := runtime.New(stateApp(comps, root, map[string]any{"open": false}, actions))
	res := Render(rt)
	if strings.Contains(res.HTML, "DETAILS") {
		t.Fatalf("card should start collapsed (prop.open=false):\n%s", res.HTML)
	}
	h, ok := handlerByName(res, "toggleCard")
	if !ok {
		t.Fatalf("toggle callback not resolved, handlers: %+v", res.Handlers)
	}
	rt.Dispatch(h.Name, nil)
	if html := Render(rt).HTML; !strings.Contains(html, "DETAILS") {
		t.Errorf("after dispatching the toggle callback the card should expand:\n%s", html)
	}
}

// TestComponentScenarioCallbackDialog is the probe app's second stuck scenario:
// a reusable dialog whose confirm button is a callback prop; pressing it must
// dispatch the instance-supplied action for real.
func TestComponentScenarioCallbackDialog(t *testing.T) {
	comps := map[string]*model.Node{
		"ConfirmDialog": {Type: "card", ID: "dlg", Props: map[string]any{"if": "{{ prop.open }}"},
			Children: []*model.Node{
				{Type: "text", ID: "msg", Text: "{{ prop.message }}"},
				{Type: "button", ID: "ok", Label: "OK",
					OnPress: &model.Invoke{Name: "{{prop.onConfirm}}", Args: map[string]string{}}},
			}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "ConfirmDialog", ID: "d1", Props: map[string]any{
			"open": "{{state.showDlg}}", "message": "Save changes?", "onConfirm": "saveItem",
		}},
	}}
	actions := map[string]*model.Action{
		"saveItem": {ID: "saveItem", Steps: []model.Step{
			{Type: "state.set", Path: "saved", Value: "{{ true }}"},
			{Type: "state.set", Path: "showDlg", Value: "{{ false }}"},
		}},
	}
	rt := runtime.New(stateApp(comps, root, map[string]any{"showDlg": true, "saved": false}, actions))
	res := Render(rt)
	if !strings.Contains(res.HTML, "Save changes?") {
		t.Fatalf("dialog should be open (prop.open bound to state true):\n%s", res.HTML)
	}
	h, ok := handlerByName(res, "saveItem")
	if !ok {
		t.Fatalf("confirm callback not resolved, handlers: %+v", res.Handlers)
	}
	rt.Dispatch(h.Name, nil)
	if rt.State["saved"] != true {
		t.Errorf("confirm should have run saveItem, state: %v", rt.State)
	}
	if html := Render(rt).HTML; strings.Contains(html, "Save changes?") {
		t.Errorf("dialog should close after confirm (showDlg=false):\n%s", html)
	}
}

// TestComponentScenarioDynamicDataCard is the probe app's third stuck scenario:
// a card component driven per-item inside a data-bound list, with typed props
// the template computes with.
func TestComponentScenarioDynamicDataCard(t *testing.T) {
	comps := map[string]*model.Node{
		"InfoCard": {Type: "card", ID: "ic", Children: []*model.Node{
			{Type: "text", ID: "t", Text: "{{ prop.title }}:{{ prop.count * 2 }}"},
		}},
	}
	root := &model.Node{Type: "list", ID: "cards", Data: "{{ state.cards }}",
		Template: &model.Node{Type: "InfoCard", ID: "card", Props: map[string]any{
			"title": "{{item.title}}", "count": "{{item.n}}",
		}}}
	rt := runtime.New(stateApp(comps, root, map[string]any{
		"cards": []any{
			map[string]any{"title": "CPU", "n": float64(3)},
			map[string]any{"title": "RAM", "n": float64(8)},
		},
	}, nil))
	res := Render(rt)
	for _, w := range []string{"CPU:6", "RAM:16"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("dynamic data card lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestComponentRecursionGuard guards that a self-referencing component terminates
// (the depth cap turns the runaway instance into an unknown node) rather than
// recursing forever.
func TestComponentRecursionGuard(t *testing.T) {
	// "Loop" instantiates itself as its only child.
	comps := map[string]*model.Node{
		"Loop": {Type: "column", ID: "loop", Children: []*model.Node{{Type: "Loop", ID: "inner"}}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Loop", ID: "top"},
	}}
	res := renderApp(t, comps, root) // must return, not hang/panic
	if len(res.Unknown) == 0 {
		t.Errorf("self-referencing component should bottom out as unknown at the depth cap")
	}
}

// schemaApp builds an app whose components carry declared schemas, for
// exercising the parts of the declaration the RENDERER honours (defaults and
// the spec's ref instance form).
func schemaApp(components map[string]*model.Node, schemas map[string]*model.ComponentSchema, root *model.Node, initial map[string]any) *model.App {
	return &model.App{
		Entry:            "main",
		Scenes:           map[string]*model.Node{"main": root},
		Components:       components,
		ComponentSchemas: schemas,
		GlobalState:      model.GlobalState{Initial: initial},
	}
}

// TestComponentPropDefaults guards the declared-default filling: a prop the
// instance omits renders as its default, a prop the instance supplies always
// wins (even via the nested props object or a state binding), and a declaration
// without a default injects nothing.
func TestComponentPropDefaults(t *testing.T) {
	comps := map[string]*model.Node{
		"Chip": {Type: "text", ID: "c", Text: "[{{ prop.label }}|{{ prop.count }}|{{ prop.tone }}]"},
	}
	schemas := map[string]*model.ComponentSchema{
		"Chip": {Props: map[string]model.PropSpec{
			"label": {Type: "string", Default: "DEFLABEL"},
			"count": {Type: "number", Default: float64(7)},
			"tone":  {Type: "string"}, // no default: nothing injected
		}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "Chip", ID: "c1"},
		{Type: "Chip", ID: "c2", Props: map[string]any{"label": "GIVEN", "count": float64(1)}},
		{Type: "Chip", ID: "c3", Props: map[string]any{"props": map[string]any{"label": "NESTED"}}},
		{Type: "Chip", ID: "c4", Props: map[string]any{"label": "{{ state.msg }}"}},
	}}
	res := Render(runtime.New(schemaApp(comps, schemas, root, map[string]any{"msg": "BOUND"})))
	for _, w := range []string{"[DEFLABEL|7|]", "[GIVEN|1|]", "[NESTED|7|]", "[BOUND|7|]"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("prop defaults not applied as declared, lacks %q:\n%s", w, res.HTML)
		}
	}
}

// TestComponentDefaultsIgnoredWithoutSchema is the back-compat guard: with no
// declaration the renderer injects nothing, so an undeclared prop stays empty.
func TestComponentDefaultsIgnoredWithoutSchema(t *testing.T) {
	comps := map[string]*model.Node{"Chip": {Type: "text", ID: "c", Text: "[{{ prop.label }}]"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{{Type: "Chip", ID: "c1"}}}
	res := renderApp(t, comps, root)
	if !strings.Contains(res.HTML, "[]") {
		t.Errorf("an undeclared component must inject no defaults:\n%s", res.HTML)
	}
}

// TestComponentRefInstance covers the spec's explicit instance form:
// {"type":"component","ref":"panel"} instantiates the component (canonical
// component:// prefix and bindings included), and an unresolvable ref still
// renders the plain `component` container it always has.
func TestComponentRefInstance(t *testing.T) {
	comps := map[string]*model.Node{
		"Panel": {Type: "card", ID: "p", Children: []*model.Node{
			{Type: "text", ID: "pt", Text: "T:{{ prop.title }}"},
			{Type: "slot", ID: "ps"},
		}},
	}
	schemas := map[string]*model.ComponentSchema{
		"Panel": {Props: map[string]model.PropSpec{"title": {Type: "string", Default: "DEF"}}},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "component", ID: "r1", Props: map[string]any{"ref": "Panel", "title": "A"}},
		{Type: "component", ID: "r2", Props: map[string]any{"ref": "component://Panel"}},
		{Type: "component", ID: "r3", Props: map[string]any{"ref": "{{ state.which }}"},
			Children: []*model.Node{{Type: "text", ID: "kid", Text: "SLOTTED"}}},
		{Type: "component", ID: "r4", Props: map[string]any{"ref": "NoSuch"}},
		{Type: "component", ID: "r5", Children: []*model.Node{{Type: "text", ID: "plain", Text: "PLAIN"}}},
	}}
	res := Render(runtime.New(schemaApp(comps, schemas, root, map[string]any{"which": "Panel"})))
	for _, w := range []string{"T:A", "T:DEF", "SLOTTED", "PLAIN"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("ref instance form lacks %q:\n%s", w, res.HTML)
		}
	}
	// r3's ref resolved through the binding, so its child filled the slot inside
	// the component (id suffixed per instance); r5 stayed a plain container.
	if !strings.Contains(res.HTML, `id="kid_r3"`) {
		t.Errorf("a bound ref must instantiate the component:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `id="plain"`) {
		t.Errorf("a ref-less component node must stay a plain container:\n%s", res.HTML)
	}
	if len(res.Unknown) != 0 {
		t.Errorf("no node should degrade to unknown: %v", res.Unknown)
	}
}
