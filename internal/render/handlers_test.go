package render

import (
	"strconv"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// onclickIndex extracts the handler index N from an onclick="qorm(N)" attribute.
func onclickIndex(html, id string) (int, bool) {
	// find the element with the given id, then its following qorm( call
	i := strings.Index(html, `id="`+id+`"`)
	if i < 0 {
		return 0, false
	}
	rest := html[i:]
	j := strings.Index(rest, `qorm(`)
	if j < 0 {
		return 0, false
	}
	rest = rest[j+len(`qorm(`):]
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(rest[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// TestHandlerIndexAlignment guards the contract that the integer embedded in an
// element's onclick/onchange is the index into Result.Handlers, so the client
// dispatches the right action. Two buttons must map to handlers 0 and 1 in order.
func TestHandlerIndexAlignment(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "button", ID: "b1", Label: "One", OnPress: &model.Invoke{Name: "first"}},
		{Type: "button", ID: "b2", Label: "Two", OnPress: &model.Invoke{Name: "second", Args: map[string]string{"k": "v"}}},
	}}
	res := renderWidget(t, root)
	if len(res.Handlers) != 2 {
		t.Fatalf("want 2 handlers, got %d: %+v", len(res.Handlers), res.Handlers)
	}
	idx1, ok1 := onclickIndex(res.HTML, "b1")
	idx2, ok2 := onclickIndex(res.HTML, "b2")
	if !ok1 || !ok2 {
		t.Fatalf("could not find onclick indices in html:\n%s", res.HTML)
	}
	if res.Handlers[idx1].Name != "first" {
		t.Errorf("b1 onclick=%d resolved to %q, want first", idx1, res.Handlers[idx1].Name)
	}
	if res.Handlers[idx2].Name != "second" || res.Handlers[idx2].Args["k"] != "v" {
		t.Errorf("b2 onclick=%d resolved to %+v, want second with k=v", idx2, res.Handlers[idx2])
	}
}

// TestChangeSyncNoOp guards the documented two-way binding behaviour: a control
// bound to state with no explicit onChange gets the qorm(-1) state-sync handler,
// which is NOT registered in the handler table (it is a client-side sentinel).
func TestChangeSyncNoOp(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "input", ID: "em", Value: "{{ state.email }}"},
		map[string]any{"email": "x"})
	if !strings.Contains(res.HTML, `onchange="qorm(-1)"`) {
		t.Errorf("bound input without onChange should sync via qorm(-1):\n%s", res.HTML)
	}
	if len(res.Handlers) != 0 {
		t.Errorf("qorm(-1) sync must not register a handler, got %+v", res.Handlers)
	}
}

// TestChangeExplicitHandler guards that an explicit onChange wins over the sync
// sentinel and is registered (with the bound data-state path still present).
func TestChangeExplicitHandler(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "input", ID: "em", Value: "{{ state.email }}", OnChange: &model.Invoke{Name: "saveEmail"}},
		map[string]any{"email": "x"})
	if strings.Contains(res.HTML, "qorm(-1)") {
		t.Errorf("explicit onChange should replace the qorm(-1) sync:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, `data-state="email"`) {
		t.Errorf("bound input should still carry data-state:\n%s", res.HTML)
	}
	if len(res.Handlers) != 1 || res.Handlers[0].Name != "saveEmail" {
		t.Errorf("explicit onChange not registered: %+v", res.Handlers)
	}
	if !strings.Contains(res.HTML, `onchange="qorm(0)"`) {
		t.Errorf("explicit onChange should wire qorm(0):\n%s", res.HTML)
	}
}

// TestUnknownWidgetSoftFail guards the self-verify contract: an unrecognised
// widget type renders as a plain container (never breaks the app) but is tagged
// with data-qorm-unknown and reported in Result.Unknown, while its children
// still render.
func TestUnknownWidgetSoftFail(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "colunm", ID: "oops", Children: textKids("still-here")})
	if !strings.Contains(res.HTML, `data-qorm-unknown="colunm"`) {
		t.Errorf("unknown widget should be tagged:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "still-here") {
		t.Errorf("unknown widget children should still render:\n%s", res.HTML)
	}
	if len(res.Unknown) != 1 || res.Unknown[0] != "colunm" {
		t.Errorf("Result.Unknown = %v, want [colunm]", res.Unknown)
	}
}

// TestSceneTagging guards that the scene root is tagged with its id so the client
// can play page transitions, and that RenderScene selects a named scene.
func TestSceneTagging(t *testing.T) {
	app := &model.App{
		Entry: "home",
		Scenes: map[string]*model.Node{
			"home":    {Type: "column", ID: "h", Children: textKids("HOME")},
			"details": {Type: "column", ID: "d", Children: textKids("DETAILS")},
		},
	}
	rt := runtime.New(app)

	home := Render(rt).HTML
	if !strings.Contains(home, `data-scene="entry"`) || !strings.Contains(home, "HOME") {
		t.Errorf("entry render should tag data-scene=entry and show HOME:\n%s", home)
	}

	details := RenderScene(rt, "details").HTML
	if !strings.Contains(details, `data-scene="details"`) || !strings.Contains(details, "DETAILS") {
		t.Errorf("RenderScene(details) should tag data-scene=details:\n%s", details)
	}

	// unknown scene id falls back to the entry scene's tree
	fallback := RenderScene(rt, "nope").HTML
	if !strings.Contains(fallback, "HOME") {
		t.Errorf("unknown scene should fall back to entry:\n%s", fallback)
	}
}

// TestNoSceneToRender guards the empty-app path renders a placeholder rather than
// panicking or emitting nothing.
func TestNoSceneToRender(t *testing.T) {
	app := &model.App{Entry: "main"} // no scenes
	res := Render(runtime.New(app))
	if !strings.Contains(res.HTML, "no scene to render") {
		t.Errorf("empty app should render placeholder:\n%s", res.HTML)
	}
}

// TestRootLandmarkRole guards that the container scene root is given role="main"
// for assistive tech, but a root that already declares a role keeps its own.
func TestRootLandmarkRole(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "text", ID: "x", Text: "hi"})
	// the wrapping column root (id "root") should carry role="main"
	if !strings.Contains(res.HTML, `id="root"`) || !strings.Contains(res.HTML, `role="main"`) {
		t.Errorf("container root should get role=main:\n%s", res.HTML)
	}

	// a root with its own role must not be overwritten
	root := &model.Node{Type: "column", ID: "root", Props: map[string]any{"role": "navigation"}, Children: textKids("x")}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	res = Render(runtime.New(app))
	if !strings.Contains(res.HTML, `role="navigation"`) || strings.Contains(res.HTML, `role="main"`) {
		t.Errorf("root with explicit role should keep it, not gain main:\n%s", res.HTML)
	}
}

// TestRTLRootDirection guards that a right-to-left locale flips the scene root's
// direction (inherited by descendants) and that an LTR locale leaves it unset.
func TestRTLRootDirection(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: textKids("x")}

	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{"locale": "ar"}}}
	if html := Render(runtime.New(app)).HTML; !strings.Contains(html, "direction:rtl") {
		t.Errorf("ar locale should set direction:rtl on root:\n%s", html)
	}

	app = &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: map[string]any{"locale": "en"}}}
	if html := Render(runtime.New(app)).HTML; strings.Contains(html, "direction:rtl") {
		t.Errorf("en locale should not set direction:rtl:\n%s", html)
	}
}

// TestTextEscaping guards that interpolated text and labels are HTML-escaped so
// authored/bound content cannot inject markup.
func TestTextEscaping(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "text", ID: "x", Text: "{{ state.msg }}"},
		map[string]any{"msg": `<script>alert(1)</script>`})
	if strings.Contains(res.HTML, "<script>") {
		t.Errorf("text must be escaped, raw <script> leaked:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "&lt;script&gt;") {
		t.Errorf("text should be HTML-escaped:\n%s", res.HTML)
	}

	res = renderWidget(t, &model.Node{Type: "button", ID: "b", Label: `a"b<c`})
	if strings.Contains(res.HTML, `>a"b<c</button>`) {
		t.Errorf("button label must be escaped:\n%s", res.HTML)
	}
	if !strings.Contains(res.HTML, "a&#34;b&lt;c") {
		t.Errorf("button label should be escaped:\n%s", res.HTML)
	}
}

// TestVisibleProp guards the if / visible / show conditional visibility: a falsy
// condition omits the node entirely, a truthy one renders it, and absence shows.
func TestVisibleProp(t *testing.T) {
	state := map[string]any{"on": true, "off": false}
	for _, key := range []string{"if", "visible", "show"} {
		t.Run(key+"-true", func(t *testing.T) {
			res := renderWidgetState(t,
				&model.Node{Type: "text", ID: "x", Text: "SECRET", Props: map[string]any{key: "{{ state.on }}"}}, state)
			if !strings.Contains(res.HTML, "SECRET") {
				t.Errorf("%s=true should render:\n%s", key, res.HTML)
			}
		})
		t.Run(key+"-false", func(t *testing.T) {
			res := renderWidgetState(t,
				&model.Node{Type: "text", ID: "x", Text: "SECRET", Props: map[string]any{key: "{{ state.off }}"}}, state)
			if strings.Contains(res.HTML, "SECRET") {
				t.Errorf("%s=false should hide:\n%s", key, res.HTML)
			}
		})
	}
}

// TestListItemHandlerScope guards that handlers registered inside a list's
// renderItem capture the per-item scope, so an action dispatched from row N sees
// that row's item (not the last one rendered).
func TestListItemHandlerScope(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "list", ID: "L", Data: "{{ state.items }}",
			Template: &model.Node{Type: "button", ID: "row", Label: "{{ item.name }}", OnPress: &model.Invoke{Name: "pick"}}},
		map[string]any{"items": []any{
			map[string]any{"name": "Alpha"},
			map[string]any{"name": "Beta"},
		}})
	if len(res.Handlers) != 2 {
		t.Fatalf("want one handler per list item, got %d", len(res.Handlers))
	}
	got := []string{}
	for _, h := range res.Handlers {
		item, _ := h.Scope["item"].(map[string]any)
		if item == nil {
			t.Fatalf("handler scope missing item: %+v", h)
		}
		got = append(got, item["name"].(string))
	}
	if got[0] != "Alpha" || got[1] != "Beta" {
		t.Errorf("handler scopes = %v, want [Alpha Beta] in order", got)
	}
}

// TestPagination guards the prev/numbered/next buttons and that each dispatches
// the node's onPress with the correct {page} argument.
func TestPagination(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "pagination", ID: "pg", Props: map[string]any{"page": "{{ state.p }}", "total": float64(3)},
			OnPress: &model.Invoke{Name: "goto"}},
		map[string]any{"p": float64(2)})
	for _, w := range []string{"‹", "›", ">1</button>", ">2</button>", ">3</button>"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("pagination lacks %q:\n%s", w, res.HTML)
		}
	}
	// prev(1) + 3 pages + next(3) = 5 registered page handlers
	pages := map[string]bool{}
	for _, h := range res.Handlers {
		if h.Name == "goto" {
			pages[h.Args["page"]] = true
		}
	}
	for _, want := range []string{"1", "2", "3"} {
		if !pages[want] {
			t.Errorf("pagination should dispatch page=%s; dispatched %v", want, pages)
		}
	}
}

// TestChartRendering guards the inline-SVG chart for the bar and line forms,
// driven by a literal data array.
func TestChartRendering(t *testing.T) {
	data := []any{float64(1), float64(5), float64(3)}

	t.Run("bar", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch", Props: map[string]any{"data": data}})
		if !strings.Contains(res.HTML, "<svg") {
			t.Errorf("chart should render svg:\n%s", res.HTML)
		}
		if n := strings.Count(res.HTML, "<rect"); n != 3 {
			t.Errorf("bar chart should render 3 bars, got %d:\n%s", n, res.HTML)
		}
	})

	t.Run("line", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch2", Props: map[string]any{"data": data, "chartType": "line"}})
		if !strings.Contains(res.HTML, "<polyline") {
			t.Errorf("line chart should render a polyline:\n%s", res.HTML)
		}
	})

	t.Run("area", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "chart", ID: "ch3", Props: map[string]any{"data": data, "chartType": "area"}})
		if !strings.Contains(res.HTML, "<polygon") || !strings.Contains(res.HTML, "<polyline") {
			t.Errorf("area chart should render polygon + polyline:\n%s", res.HTML)
		}
	})

	t.Run("bound-data", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "chart", ID: "ch4", Props: map[string]any{"data": "{{ state.series }}"}},
			map[string]any{"series": []any{float64(2), float64(4)}})
		if n := strings.Count(res.HTML, "<rect"); n != 2 {
			t.Errorf("bound bar chart should render 2 bars, got %d:\n%s", n, res.HTML)
		}
	})
}

// TestDragAttr guards the desktop window-drag region marker.
func TestDragAttr(t *testing.T) {
	res := renderWidget(t, &model.Node{Type: "column", ID: "bar", Props: map[string]any{"drag": true}, Children: textKids("x")})
	if !strings.Contains(res.HTML, "data-qorm-drag") {
		t.Errorf("drag:true should mark the node as a drag region:\n%s", res.HTML)
	}
}
