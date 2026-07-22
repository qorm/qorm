package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// renderWidget renders a single widget as the only child of a plain column root
// and returns the Result. Wrapping in a container root keeps the widget under
// test away from the scene-root tag (which the renderer annotates with
// data-scene / role), so fragment assertions see the widget's own markup.
func renderWidget(t *testing.T, n *model.Node) Result {
	t.Helper()
	return renderWidgetState(t, n, nil)
}

func renderWidgetState(t *testing.T, n *model.Node, state map[string]any) Result {
	t.Helper()
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		GlobalState: model.GlobalState{Initial: state},
	}
	return Render(runtime.New(app))
}

// TestWidgetRendering drives a wide slice of the built-in widget vocabulary
// through the renderer and asserts each emits its characteristic markup/CSS.
// Every assertion would fail if the widget regressed to an unknown node or lost
// a load-bearing attribute.
func TestWidgetRendering(t *testing.T) {
	kids := func(ts ...string) []*model.Node {
		out := make([]*model.Node, 0, len(ts))
		for i, s := range ts {
			out = append(out, &model.Node{Type: "text", ID: "k" + string(rune('a'+i)), Text: s})
		}
		return out
	}

	cases := []struct {
		name    string
		node    *model.Node
		want    []string
		wantNot []string
	}{
		{
			name: "text",
			node: &model.Node{Type: "text", ID: "t", Text: "Hello"},
			want: []string{`id="t"`, "Hello", "font-size:15px;"},
		},
		{
			name: "button-default",
			node: &model.Node{Type: "button", ID: "b", Label: "Go"},
			want: []string{`id="b"`, `class="qorm-tap"`, "background:var(--accent)", ">Go</button>"},
		},
		{
			name: "button-text-variant",
			node: &model.Node{Type: "button", ID: "b2", Label: "T", Props: map[string]any{"variant": "text"}},
			want: []string{"background:transparent", "color:var(--accent)"},
		},
		{
			name: "button-outlined-variant",
			node: &model.Node{Type: "button", ID: "b3", Label: "O", Props: map[string]any{"variant": "outlined"}},
			want: []string{"border:1px solid var(--accent)"},
		},
		{
			name: "button-elevated-variant",
			node: &model.Node{Type: "button", ID: "b4", Label: "E", Props: map[string]any{"variant": "elevated"}},
			want: []string{"background:var(--surface)", "box-shadow:0 2px 5px"},
		},
		{
			name: "button-icon-variant",
			node: &model.Node{Type: "button", ID: "b5", Label: "I", Props: map[string]any{"variant": "icon"}},
			want: []string{"border-radius:50%", "width:40px"},
		},
		{
			name: "link-default-href",
			node: &model.Node{Type: "link", ID: "l", Label: "Open"},
			want: []string{`<a id="l"`, `href="javascript:void(0)"`, ">Open</a>"},
		},
		{
			name: "link-explicit-href",
			node: &model.Node{Type: "link", ID: "l2", Label: "X", Props: map[string]any{"href": "https://example.com"}},
			want: []string{`href="https://example.com"`},
		},
		{
			name: "grid-columns",
			node: &model.Node{Type: "grid", ID: "g", Props: map[string]any{"columns": float64(3)}, Children: kids("a")},
			want: []string{"display:grid", "grid-template-columns:repeat(3,1fr)"},
		},
		{
			name: "scroll-horizontal",
			node: &model.Node{Type: "scroll", ID: "sc", Props: map[string]any{"orientation": "horizontal"}, Children: kids("a")},
			want: []string{"flex-direction:row", "overflow-x:auto"},
		},
		{
			name: "scroll-vertical",
			node: &model.Node{Type: "scroll", ID: "sc2", Children: kids("a")},
			want: []string{"overflow-y:auto"},
		},
		{
			name: "card",
			node: &model.Node{Type: "card", ID: "cd", Children: kids("a")},
			want: []string{"background:var(--surface)", "border-radius:14px"},
		},
		{
			name: "row-direction",
			node: &model.Node{Type: "row", ID: "rw", Children: kids("a")},
			want: []string{"flex-direction:row"},
		},
		{
			name: "divider-horizontal",
			node: &model.Node{Type: "divider", ID: "dv"},
			want: []string{"height:1px;width:100%;background:var(--sep)"},
		},
		{
			name: "divider-vertical",
			node: &model.Node{Type: "verticaldivider", ID: "dvv"},
			want: []string{"width:1px;align-self:stretch"},
		},
		{
			name: "spacer",
			node: &model.Node{Type: "spacer", ID: "sp"},
			want: []string{"flex:1 1 auto"},
		},
		{
			name: "image",
			node: &model.Node{Type: "image", ID: "im", Props: map[string]any{"src": "pic.png", "fit": "contain", "alt": "A pic"}},
			want: []string{`<img id="im"`, `src="pic.png"`, "object-fit:contain", `alt="A pic"`},
		},
		{
			name: "avatar-src",
			node: &model.Node{Type: "avatar", ID: "av", Props: map[string]any{"src": "me.png"}},
			want: []string{`<img id="av"`, `src="me.png"`, "border-radius:50%", "object-fit:cover"},
		},
		{
			name: "avatar-initials",
			node: &model.Node{Type: "avatar", ID: "av2", Props: map[string]any{"name": "John Doe"}},
			want: []string{">JO</div>"},
		},
		{
			name: "icon-known",
			node: &model.Node{Type: "icon", ID: "ic", Props: map[string]any{"icon": "check"}},
			want: []string{`id="ic"`, "<svg"},
		},
		{
			name: "icon-unknown-fallback",
			node: &model.Node{Type: "icon", ID: "ic2", Props: map[string]any{"icon": "notarealicon"}},
			want: []string{">notarealicon</span>"},
		},
		{
			name: "badge-plain",
			node: &model.Node{Type: "badge", ID: "bd", Label: "NEW"},
			want: []string{"border-radius:999px", ">NEW</span>"},
		},
		{
			name: "badge-over-child",
			node: &model.Node{Type: "badge", ID: "bd2", Label: "5", Children: kids("x")},
			want: []string{"position:relative", "background:#ef4444", ">5</span>"},
		},
		{
			name: "spinner",
			node: &model.Node{Type: "spinner", ID: "sn"},
			want: []string{`class="qorm-spin"`, "border-top-color:var(--accent)", `role="status"`},
		},
		{
			name: "video",
			node: &model.Node{Type: "video", ID: "vd", Props: map[string]any{"src": "clip.mp4"}},
			want: []string{`<video id="vd"`, `src="clip.mp4"`, "controls"},
		},
		{
			name: "skeleton",
			node: &model.Node{Type: "skeleton", ID: "sk"},
			want: []string{`class="qorm-skel"`, `aria-hidden="true"`},
		},
		{
			name: "tag",
			node: &model.Node{Type: "tag", ID: "tg", Label: "beta"},
			want: []string{`id="tg"`, "border-radius:999px", ">beta"},
		},
		{
			name: "selectabletext",
			node: &model.Node{Type: "selectabletext", ID: "st", Text: "copy me"},
			want: []string{"user-select:text", "copy me"},
		},
		{
			name: "aspectratio",
			node: &model.Node{Type: "aspectratio", ID: "ar", Props: map[string]any{"ratio": float64(16)}, Children: kids("a")},
			want: []string{"aspect-ratio:16"},
		},
		{
			name: "wrap",
			node: &model.Node{Type: "wrap", ID: "wp", Children: kids("a")},
			want: []string{"flex-wrap:wrap", "column-gap:8px"},
		},
		{
			name: "form",
			node: &model.Node{Type: "form", ID: "fm", Children: kids("a")},
			want: []string{`<form id="fm"`, `onsubmit="return false"`},
		},
		{
			name: "ignorepointer",
			node: &model.Node{Type: "ignorepointer", ID: "ip", Children: kids("a")},
			want: []string{"display:contents", "pointer-events:none"},
		},
		{
			name: "backbutton",
			node: &model.Node{Type: "backbutton", ID: "bb"},
			want: []string{`onclick="history.back()"`, `aria-label="Back"`, "<svg"},
		},
		{
			name: "closebutton",
			node: &model.Node{Type: "closebutton", ID: "cb"},
			want: []string{`onclick="history.back()"`, `aria-label="Close"`},
		},
		{
			name: "fab-default",
			node: &model.Node{Type: "fab", ID: "fb"},
			want: []string{`class="qorm-tap"`, ">+</button>", "border-radius:50%"},
		},
		{
			name: "bottomappbar",
			node: &model.Node{Type: "bottomappbar", ID: "bab", Children: kids("a")},
			want: []string{`role="toolbar"`, "border-top:.5px solid var(--sep)"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("html lacks %q:\n%s", w, res.HTML)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(res.HTML, w) {
					t.Errorf("html should not contain %q:\n%s", w, res.HTML)
				}
			}
			if len(res.Unknown) != 0 {
				t.Errorf("known widget %q reported as unknown: %v", tc.node.Type, res.Unknown)
			}
		})
	}
}

// TestInputWidgets covers the form controls, including the two-way state binding
// that emits the qorm(-1) sync handler and the data-state path attribute.
func TestInputWidgets(t *testing.T) {
	t.Run("input-text", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "input", ID: "nm", Placeholder: "Name"})
		for _, w := range []string{`<input id="nm"`, `type="text"`, `placeholder="Name"`, "outline:none"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("input html lacks %q:\n%s", w, res.HTML)
			}
		}
		// unbound input: no qorm(-1), no data-state
		if strings.Contains(res.HTML, "qorm(-1)") || strings.Contains(res.HTML, "data-state") {
			t.Errorf("unbound input should not sync state:\n%s", res.HTML)
		}
	})

	t.Run("input-password-by-id", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "input", ID: "userPassword"})
		if !strings.Contains(res.HTML, `type="password"`) {
			t.Errorf("id containing 'password' should render type=password:\n%s", res.HTML)
		}
	})

	t.Run("input-bound-syncs", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "input", ID: "em", Value: "{{ state.email }}"},
			map[string]any{"email": "a@b.co"})
		for _, w := range []string{`value="a@b.co"`, `data-state="email"`, `onchange="qorm(-1)"`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("bound input lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("textarea-rows", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textarea", ID: "ta", Props: map[string]any{"rows": float64(8)}})
		if !strings.Contains(res.HTML, `rows="8"`) || !strings.Contains(res.HTML, "<textarea") {
			t.Errorf("textarea lacks rows=8:\n%s", res.HTML)
		}
	})

	t.Run("select-selected-option", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "select", ID: "sel", Value: "{{ state.c }}", Props: map[string]any{
				"options": []any{
					map[string]any{"value": "a", "label": "Apple"},
					map[string]any{"value": "b", "label": "Banana"},
				},
			}},
			map[string]any{"c": "b"})
		for _, w := range []string{`<select id="sel"`, `<option value="a">Apple</option>`, `value="b" selected`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("select lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("checkbox-checked-bound", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "checkbox", ID: "ck", Label: "Agree", Value: "{{ state.on }}"},
			map[string]any{"on": true})
		for _, w := range []string{`type="checkbox"`, " checked", `data-state="on"`, `onchange="qorm(-1)"`, "Agree"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("checkbox lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("switch-renders-toggle", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "switch", ID: "sw", Label: "Wi-Fi"})
		if !strings.Contains(res.HTML, `class="qorm-switch"`) || !strings.Contains(res.HTML, "Wi-Fi") {
			t.Errorf("switch lacks toggle markup:\n%s", res.HTML)
		}
	})

	t.Run("radio-checked-option", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "radio", ID: "rd", Value: "{{ state.choice }}", Props: map[string]any{
				"options": []any{
					map[string]any{"value": "x", "label": "Ex"},
					map[string]any{"value": "y", "label": "Why"},
				},
			}},
			map[string]any{"choice": "y"})
		for _, w := range []string{`type="radio" name="rd"`, `value="y" checked`, `data-state="choice"`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("radio lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("slider-pct", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "slider", ID: "sl", Value: "{{ state.vol }}"},
			map[string]any{"vol": float64(50)})
		for _, w := range []string{`class="qorm-slider"`, `type="range"`, `min="0"`, `max="100"`, `value="50"`, "--pct:50%"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("slider lacks %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestProgressFractions guards the documented 0..1-as-fraction behaviour of the
// progress widget, alongside the 0..100 percentage form and clamping.
func TestProgressFractions(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"percent", "50", "width:50%"},
		{"fraction", "0.5", "width:50%"},
		{"clamp-high", "250", "width:100%"},
		{"clamp-low", "-10", "width:0%"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "progress", ID: "pr", Value: tc.value})
			if !strings.Contains(res.HTML, tc.want) || !strings.Contains(res.HTML, `role="progressbar"`) {
				t.Errorf("progress(%s) lacks %q:\n%s", tc.value, tc.want, res.HTML)
			}
		})
	}
}

// textKids builds n text children with distinct ids and the given labels.
func textKids(labels ...string) []*model.Node {
	out := make([]*model.Node, 0, len(labels))
	for i, s := range labels {
		out = append(out, &model.Node{Type: "text", ID: "kid" + string(rune('a'+i)), Text: s})
	}
	return out
}

// TestDisplayWidgets covers the data-presentation widgets (tables, steppers,
// trees, metrics, ...). Literal data props exercise the same code paths as
// bound arrays without needing state.
func TestDisplayWidgets(t *testing.T) {
	cols := []any{
		map[string]any{"value": "name", "label": "Name"},
		map[string]any{"value": "age", "label": "Age"},
	}
	rows := []any{
		map[string]any{"name": "Alice", "age": "30"},
		map[string]any{"name": "Bob", "age": "40"},
	}

	cases := []struct {
		name    string
		node    *model.Node
		want    []string
		wantNot []string
	}{
		{
			name: "tabs",
			node: &model.Node{Type: "tabs", ID: "tb", Props: map[string]any{"tabs": []any{"One", "Two"}}, Children: textKids("P1", "P2")},
			want: []string{`qorm-tabbar`, `qorm-tab-active`, `data-tab="1"`, `data-panel="0" style="display:block`, `display:none`},
		},
		{
			name: "table",
			node: &model.Node{Type: "table", ID: "tbl", Props: map[string]any{"columns": cols, "data": rows}},
			want: []string{`class="qorm-table"`, `<th>Name</th>`, `<td>Alice</td>`, `<td>40</td>`},
		},
		{
			name: "datatable-selectable",
			node: &model.Node{Type: "datatable", ID: "dt", Props: map[string]any{"columns": cols, "data": rows, "selectable": true}},
			want: []string{`class="qorm-datatable"`, `qdt-check`, `<td>Alice</td>`},
		},
		{
			name: "accordion",
			node: &model.Node{Type: "accordion", ID: "ac", Children: []*model.Node{
				{Type: "column", ID: "s1", Props: map[string]any{"title": "Sec1"}, Children: textKids("body1")},
				{Type: "column", ID: "s2", Props: map[string]any{"title": "Sec2"}, Children: textKids("body2")},
			}},
			want: []string{`qorm-acc`, `qormAcc(this)`, `Sec1`, `qorm-acc-panel`, `display:block`, `display:none`},
		},
		{
			name: "rating",
			node: &model.Node{Type: "rating", ID: "rt", Props: map[string]any{"value": "3"}},
			want: []string{`aria-label="3 of 5"`, `<svg`},
		},
		{
			name: "steps",
			node: &model.Node{Type: "steps", ID: "st", Props: map[string]any{"steps": []any{"Cart", "Pay", "Done"}, "active": "1"}},
			want: []string{"Cart", "Pay", "Done"},
		},
		{
			name: "breadcrumb",
			node: &model.Node{Type: "breadcrumb", ID: "bc", Props: map[string]any{"items": []any{"Home", "Docs", "API"}}},
			want: []string{"Home", "Docs", "API", "color:var(--sep)"},
		},
		{
			name: "tree",
			node: &model.Node{Type: "tree", ID: "tr", Props: map[string]any{"data": []any{
				map[string]any{"label": "Root", "children": []any{map[string]any{"label": "Leaf"}}},
			}}},
			want: []string{`qorm-tree`, `qorm-tree-n`, `qorm-tree-sum`, `qorm-tree-leaf`, "Root", "Leaf"},
		},
		{
			name: "timeline",
			node: &model.Node{Type: "timeline", ID: "tl", Props: map[string]any{"items": []any{
				map[string]any{"title": "T1", "text": "D1"},
				map[string]any{"title": "T2", "text": "D2"},
			}}},
			want: []string{"T1", "D1", "T2"},
		},
		{
			name: "stat",
			node: &model.Node{Type: "stat", ID: "mt", Props: map[string]any{"value": "123", "label": "Users", "delta": "+5", "deltaType": "up"}},
			want: []string{"123", "Users", "+5", "#16a34a"},
		},
		{
			name: "empty",
			node: &model.Node{Type: "empty", ID: "em", Props: map[string]any{"title": "Nothing"}, Text: "No items"},
			want: []string{"Nothing", "No items", "<svg"},
		},
		{
			name: "descriptions",
			node: &model.Node{Type: "descriptions", ID: "ds", Props: map[string]any{"items": []any{
				map[string]any{"label": "Name", "value": "Al"},
				map[string]any{"label": "Age", "value": "30"},
			}}},
			want: []string{"display:grid", "Name", "Al", "30"},
		},
		{
			name: "materialstepper",
			node: &model.Node{Type: "materialstepper", ID: "ms", Props: map[string]any{"steps": []any{"One", "Two"}, "active": "1"},
				Children: textKids("CONTENT-A", "CONTENT-B")},
			want:    []string{"One", "Two", "CONTENT-B"},
			wantNot: []string{"CONTENT-A"},
		},
		{
			name: "listsection",
			node: &model.Node{Type: "listsection", ID: "ls", Props: map[string]any{"header": "Settings", "footer": "Note"}, Children: textKids("row")},
			want: []string{"Settings", "Note", "background:var(--surface)"},
		},
		{
			name: "expansiontile",
			node: &model.Node{Type: "expansiontile", ID: "ex", Label: "More", Props: map[string]any{"initiallyExpanded": "true"}, Children: textKids("hidden")},
			want: []string{"<details", " open", "<summary", "More"},
		},
		{
			name: "listtile",
			node: &model.Node{Type: "listtile", ID: "lt", Label: "Title", Props: map[string]any{"subtitle": "Sub", "leading": "star"},
				OnPress: &model.Invoke{Name: "open"}},
			want: []string{"Title", "Sub", "<svg", `onclick="qorm(`, "›"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("html lacks %q:\n%s", w, res.HTML)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(res.HTML, w) {
					t.Errorf("html should not contain %q:\n%s", w, res.HTML)
				}
			}
			if len(res.Unknown) != 0 {
				t.Errorf("known widget %q reported as unknown: %v", tc.node.Type, res.Unknown)
			}
		})
	}
}

// TestListRendersTemplate checks a bound list renders its renderItem once per
// element with the `item` scope, and virtualization wraps each row when set.
func TestListRendersTemplate(t *testing.T) {
	items := map[string]any{"items": []any{
		map[string]any{"name": "Alpha"},
		map[string]any{"name": "Beta"},
		map[string]any{"name": "Gamma"},
	}}
	res := renderWidgetState(t,
		&model.Node{Type: "list", ID: "L", Data: "{{ state.items }}",
			Template: &model.Node{Type: "text", ID: "row", Text: "{{ item.name }}"}},
		items)
	for _, w := range []string{"Alpha", "Beta", "Gamma"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("list did not render item %q:\n%s", w, res.HTML)
		}
	}

	// virtualize wraps each item in a content-visibility row
	res = renderWidgetState(t,
		&model.Node{Type: "list", ID: "L2", Data: "{{ state.items }}", Props: map[string]any{"virtualize": true},
			Template: &model.Node{Type: "text", ID: "row", Text: "{{ item.name }}"}},
		items)
	if n := strings.Count(res.HTML, "content-visibility:auto"); n != 3 {
		t.Errorf("virtualized list should wrap 3 rows, got %d:\n%s", n, res.HTML)
	}
}

// TestOverlayWidgets covers the open/closed gating of overlays and their
// characteristic chrome (dialogs, sheets, snackbars, menus, indicators).
func TestOverlayWidgets(t *testing.T) {
	openState := map[string]any{"show": true}
	closedState := map[string]any{"show": false}

	t.Run("modal-open", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "modal", ID: "md", Props: map[string]any{"open": "{{ state.show }}", "title": "Hi"}, Children: textKids("body")},
			openState)
		for _, w := range []string{`role="dialog"`, `aria-modal="true"`, "Hi", `data-dismiss-h=`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("open modal lacks %q:\n%s", w, res.HTML)
			}
		}
		// the default dismiss handler must be registered against the bound path
		found := false
		for _, h := range res.Handlers {
			if h.Name == runtime.BuiltinDismiss && h.Args["path"] == "show" {
				found = true
			}
		}
		if !found {
			t.Errorf("open modal did not register __dismiss for path 'show': %+v", res.Handlers)
		}
	})

	t.Run("modal-closed", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "modal", ID: "md", Props: map[string]any{"open": "{{ state.show }}"}, Children: textKids("body")},
			closedState)
		if strings.Contains(res.HTML, `role="dialog"`) {
			t.Errorf("closed modal should render nothing:\n%s", res.HTML)
		}
	})

	t.Run("snackbar-open", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "snackbar", ID: "sb", Props: map[string]any{"open": "{{ state.show }}", "action": "Undo"}, Text: "Saved",
				OnPress: &model.Invoke{Name: "undo"}},
			openState)
		for _, w := range []string{`role="status"`, "Saved", "Undo"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("open snackbar lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("snackbar-closed", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "snackbar", ID: "sb", Props: map[string]any{"open": "{{ state.show }}"}, Text: "Saved"},
			closedState)
		if strings.Contains(res.HTML, `role="status"`) {
			t.Errorf("closed snackbar should render nothing:\n%s", res.HTML)
		}
	})

	t.Run("alert-success", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alert", ID: "al", Props: map[string]any{"variant": "success", "title": "Done"}, Text: "Saved"})
		for _, w := range []string{`role="alert"`, "Done", "Saved", "var(--success)"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("alert lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("alert-error", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alert", ID: "al2", Props: map[string]any{"variant": "error"}, Text: "Boom"})
		if !strings.Contains(res.HTML, "var(--danger)") {
			t.Errorf("error alert should use danger color:\n%s", res.HTML)
		}
	})

	t.Run("circular-determinate", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "circularprogress", ID: "cp", Props: map[string]any{"value": "0.5"}})
		if !strings.Contains(res.HTML, "stroke-dashoffset") || !strings.Contains(res.HTML, "<svg") {
			t.Errorf("determinate circular progress lacks arc:\n%s", res.HTML)
		}
	})

	t.Run("circular-indeterminate", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "circularprogress", ID: "cp2"})
		if !strings.Contains(res.HTML, "animateTransform") {
			t.Errorf("indeterminate circular progress should spin:\n%s", res.HTML)
		}
	})

	t.Run("activity-indicator", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "activityindicator", ID: "ai"})
		if !strings.Contains(res.HTML, `qorm-activity`) {
			t.Errorf("activity indicator lacks class:\n%s", res.HTML)
		}
		if n := strings.Count(res.HTML, "<rect"); n != 8 {
			t.Errorf("activity indicator should have 8 spokes, got %d", n)
		}
	})

	t.Run("menu-items", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "menu", ID: "mn", Label: "Options", Props: map[string]any{
			"items": []any{
				map[string]any{"label": "Edit", "icon": "copy", "onPress": map[string]any{"name": "edit"}},
				map[string]any{"label": "Del", "disabled": true},
			},
		}})
		for _, w := range []string{`qorm-menu`, "Options", "Edit", "Del", "opacity:.45"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("menu lacks %q:\n%s", w, res.HTML)
			}
		}
		found := false
		for _, h := range res.Handlers {
			if h.Name == "edit" {
				found = true
			}
		}
		if !found {
			t.Errorf("menu item onPress 'edit' not registered: %+v", res.Handlers)
		}
	})

	t.Run("alertdialog", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "alertdialog", ID: "ad", Props: map[string]any{
			"open":    "true",
			"title":   "Sure?",
			"message": "Really",
			"actions": []any{
				map[string]any{"label": "Cancel", "style": "cancel"},
				map[string]any{"label": "OK", "onPress": map[string]any{"name": "ok"}},
			},
		}})
		for _, w := range []string{"Sure?", "Really", "Cancel", "OK"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("alertdialog lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("actionsheet", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "actionsheet", ID: "as", Props: map[string]any{
			"open":    "true",
			"title":   "Pick",
			"actions": []any{map[string]any{"label": "A"}, map[string]any{"label": "B", "style": "destructive"}},
			"cancel":  []any{map[string]any{"label": "Cancel"}},
		}})
		for _, w := range []string{`qorm-sheet`, "Pick", "A", "B", "Cancel", "var(--danger)"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("actionsheet lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("drawer-open", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "drawer", ID: "dr", Props: map[string]any{"open": "{{ state.show }}"}, Children: textKids("panel")},
			openState)
		if !strings.Contains(res.HTML, `role="dialog"`) || !strings.Contains(res.HTML, "panel") {
			t.Errorf("open drawer lacks panel:\n%s", res.HTML)
		}
	})

	t.Run("drawer-closed", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "drawer", ID: "dr", Props: map[string]any{"open": "{{ state.show }}"}, Children: textKids("panel")},
			closedState)
		if strings.Contains(res.HTML, `role="dialog"`) {
			t.Errorf("closed drawer should render nothing:\n%s", res.HTML)
		}
	})
}

// TestCapabilityWidgets covers the device-capability widgets (the hwList /
// hwAdjust family plus camera/notify/etc): each emits its bridge-wiring class
// and onclick helper so the client JS can find and drive it.
func TestCapabilityWidgets(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
		want []string
	}{
		{"bluetooth", &model.Node{Type: "bluetooth", ID: "bt"}, []string{`qorm-bluetooth`, `qormBluetooth(this)`, "Scan Bluetooth"}},
		{"wifi", &model.Node{Type: "wifi", ID: "wf"}, []string{`qorm-wifi`, `qormWifi(this)`}},
		{"battery", &model.Node{Type: "battery", ID: "ba"}, []string{`qorm-battery`, `qormBattery(this)`}},
		{"volume", &model.Node{Type: "volume", ID: "vo"}, []string{`qorm-volume`, `qormVol(this,-1)`, `qormVol(this,1)`}},
		{"brightness", &model.Node{Type: "brightness", ID: "br"}, []string{`qorm-brightness`, `qormBright(this,-1)`}},
		// Defaults prefix the built-in SVG icon (not emoji) to the label.
		{"camera", &model.Node{Type: "camera", ID: "cm"}, []string{`qorm-camera`, `qorm-cam-preview`, `qormCamera(this)`, "Take Photo", iconSVG("camera", 18)}},
		{"location", &model.Node{Type: "location", ID: "lo"}, []string{`qorm-location`, `qormGeo(this)`, "Get Location", iconSVG("location", 18)}},
		{"sensors", &model.Node{Type: "sensors", ID: "se"}, []string{`qorm-motion`, `qormMotion(this)`, "Enable Motion", iconSVG("compass", 18)}},
		{"recorder", &model.Node{Type: "recorder", ID: "re"}, []string{`qorm-recorder`, `qormRec(this)`, "Record", iconSVG("mic", 18)}},
		{"biometric", &model.Node{Type: "biometric", ID: "bi"}, []string{`qorm-biometric`, `qormBio(this)`}},
		{"notify", &model.Node{Type: "notify", ID: "no"}, []string{`qorm-notify`, `qormNotify(this)`, "Send Notification", iconSVG("bell", 18), `data-body="Hello from your QORM app"`}},
		{"dockbadge", &model.Node{Type: "dockbadge", ID: "db"}, []string{`qorm-dockbadge`, `qormBadge(this,-1)`, `qormBadge(this,1)`}},
		{"loginitem", &model.Node{Type: "loginitem", ID: "li"}, []string{`qorm-loginitem`, `qormLoginItem(this)`}},
		{"screens", &model.Node{Type: "screens", ID: "scr"}, []string{`qorm-screens`}},
		{"clipboard", &model.Node{Type: "clipboard", ID: "cl"}, []string{`qorm-clipboard`, `qormClipboard(this)`}},
		{"torch", &model.Node{Type: "torch", ID: "to"}, []string{`qorm-torch`, `qormTorch(this)`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("html lacks %q:\n%s", w, res.HTML)
				}
			}
			if len(res.Unknown) != 0 {
				t.Errorf("capability widget %q reported as unknown: %v", tc.node.Type, res.Unknown)
			}
		})
	}

	// House rule: no emoji anywhere. The hardware-widget defaults embed the
	// built-in SVG icon set instead — pin that the default output of each
	// widget that once carried an emoji default is emoji-free.
	t.Run("defaults-no-emoji", func(t *testing.T) {
		for _, typ := range []string{"location", "sensors", "recorder", "notify", "camera"} {
			res := renderWidget(t, &model.Node{Type: typ, ID: "hw-" + typ})
			for _, c := range res.HTML {
				if (c >= 0x1F000 && c <= 0x1FAFF) || (c >= 0x2600 && c <= 0x27BF) {
					t.Errorf("%s default output contains emoji %q:\n%s", typ, string(c), res.HTML)
				}
			}
		}
	})

	// An app-authored label replaces the icon+text default with plain escaped
	// text (no injected icon).
	t.Run("custom-label-plain-text", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "camera", ID: "cmx", Props: map[string]any{"label": "Snap it"}})
		if !strings.Contains(res.HTML, "Snap it") {
			t.Errorf("camera lacks custom label:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, iconSVG("camera", 18)) || strings.Contains(res.HTML, "Take Photo") {
			t.Errorf("custom label should not carry the default icon/text:\n%s", res.HTML)
		}
	})

	t.Run("picker-selects-current", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "picker", ID: "pk", Value: "{{ state.sz }}", Props: map[string]any{
				"options": []any{
					map[string]any{"value": "S", "label": "Small"},
					map[string]any{"value": "L", "label": "Large"},
				},
			}},
			map[string]any{"sz": "L"})
		for _, w := range []string{"Small", "Large", "font-weight:600", "height:180px"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("picker lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("datepicker-wheels", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "datepicker", ID: "dp", Value: "{{ state.d }}"},
			map[string]any{"d": "2026-07-15"})
		for _, w := range []string{"Jan", "2026", "height:180px"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("datepicker lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("timepicker-wheels", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "timepicker", ID: "tp", Value: "{{ state.t }}"},
			map[string]any{"t": "09:30"})
		for _, w := range []string{"00", "23", "height:180px"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("timepicker lacks %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestLayoutWidgets covers the structural/navigation widgets.
func TestLayoutWidgets(t *testing.T) {
	navItems := []any{
		map[string]any{"value": "h", "label": "Home", "icon": "home"},
		map[string]any{"value": "s", "label": "Search", "icon": "search"},
	}

	t.Run("scaffold-arranges-children", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "scaffold", ID: "sf", Children: []*model.Node{
			{Type: "appbar", ID: "ab", Label: "Top"},
			{Type: "text", ID: "body", Text: "BODY"},
			{Type: "fab", ID: "add", Label: "Add"},
			{Type: "bottomnav", ID: "bn", Value: "{{ state.tab }}", Props: map[string]any{"items": navItems}},
		}})
		for _, w := range []string{`qorm-body`, "Top", "BODY", "Add", `qorm-bottomnav`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("scaffold lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("bottomnav-active-item", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "bottomnav", ID: "bn", Value: "{{ state.tab }}", Props: map[string]any{"items": navItems},
				OnChange: &model.Invoke{Name: "setTab"}},
			map[string]any{"tab": "h"})
		for _, w := range []string{`qorm-bottomnav`, `qorm-navitem`, "Home", "Search", "color:var(--accent)"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("bottomnav lacks %q:\n%s", w, res.HTML)
			}
		}
		if len(res.Handlers) != 2 {
			t.Errorf("bottomnav should register one handler per item, got %d", len(res.Handlers))
		}
	})

	t.Run("navigationrail", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "navigationrail", ID: "nr", Value: "{{ state.tab }}", Props: map[string]any{"items": navItems}},
			map[string]any{"tab": "s"})
		for _, w := range []string{"Home", "Search", "border-right:.5px solid var(--sep)"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("navigationrail lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("navigationdrawer", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "navigationdrawer", ID: "nd", Value: "{{ state.tab }}", Props: map[string]any{"items": navItems}},
			map[string]any{"tab": "h"})
		for _, w := range []string{`role="navigation"`, "Home", "Search"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("navigationdrawer lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("appbar-title", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "appbar", ID: "ab", Label: "Title"})
		for _, w := range []string{"Title", "min-width:44px"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("appbar lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("largetitle", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "largetitle", ID: "ltt", Label: "Big", Props: map[string]any{"subtitle": "Sub"}})
		for _, w := range []string{"Big", "Sub", "font-size:34px"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("largetitle lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("carousel-snap", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "carousel", ID: "cr", Children: textKids("p1", "p2")})
		for _, w := range []string{"scroll-snap-type:x mandatory", "scroll-snap-align:start"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("carousel lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("pageview", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "pageview", ID: "pv", Children: textKids("p1", "p2")})
		for _, w := range []string{`qorm-pageview`, "flex:0 0 100%"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("pageview lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("gridview-template", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "gridview", ID: "gv2", Data: "{{ state.items }}",
				Props:    map[string]any{"crossAxisCount": float64(3)},
				Template: &model.Node{Type: "text", ID: "cell", Text: "{{ item.t }}"}},
			map[string]any{"items": []any{map[string]any{"t": "X"}, map[string]any{"t": "Y"}}})
		for _, w := range []string{"grid-template-columns:repeat(3,1fr)", "X", "Y"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("gridview lacks %q:\n%s", w, res.HTML)
			}
		}
		// without a template it degrades to a plain flex container — never a grid,
		// never unknown — while still rendering its children. Note: asserting
		// display:flex on the whole document would be vacuous (the wrapping column
		// root emits it too), so key off the gridview's own id, the absence of
		// display:grid, and a clean Unknown list.
		res = renderWidget(t, &model.Node{Type: "gridview", ID: "gv3", Children: textKids("z")})
		if !strings.Contains(res.HTML, `id="gv3"`) {
			t.Errorf("template-less gridview did not render its element:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "display:grid") {
			t.Errorf("template-less gridview should not render as a grid:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "z") {
			t.Errorf("template-less gridview should still render its children:\n%s", res.HTML)
		}
		if len(res.Unknown) != 0 {
			t.Errorf("template-less gridview reported as unknown: %v", res.Unknown)
		}
	})

	t.Run("limitedbox", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "limitedbox", ID: "lb", Props: map[string]any{"maxWidth": float64(200)}, Children: textKids("x")})
		if !strings.Contains(res.HTML, "max-width:200px") {
			t.Errorf("limitedbox lacks max-width:\n%s", res.HTML)
		}
	})

	t.Run("indexedstack-shows-index", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "indexedstack", ID: "is", Props: map[string]any{"index": "1"},
			Children: textKids("FIRST", "SECOND")})
		if !strings.Contains(res.HTML, "FIRST") || !strings.Contains(res.HTML, "SECOND") {
			t.Errorf("indexedstack should mount all children:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, "display:none") {
			t.Errorf("indexedstack should hide non-active child:\n%s", res.HTML)
		}
	})

	t.Run("offstage-default-hidden", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "offstage", ID: "os", Children: textKids("kept")})
		if !strings.Contains(res.HTML, "display:none") || !strings.Contains(res.HTML, "kept") {
			t.Errorf("offstage should keep but hide child:\n%s", res.HTML)
		}
	})

	t.Run("richtext-spans", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "richtext", ID: "rx", Props: map[string]any{"spans": []any{
			map[string]any{"text": "Hello", "color": "red", "fontSize": float64(20)},
			map[string]any{"text": "World", "italic": true},
		}}})
		for _, w := range []string{"Hello", "World", "color:red", "font-style:italic"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("richtext lacks %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestGestureWidgets covers the interaction wrappers and their client helpers.
func TestGestureWidgets(t *testing.T) {
	t.Run("gesturedetector-all-taps", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "gesturedetector", ID: "gd",
			OnPress:  &model.Invoke{Name: "tap"},
			Props:    map[string]any{"onDoubleTap": map[string]any{"name": "dbl"}, "onLongPress": map[string]any{"name": "long"}},
			Children: textKids("x")})
		for _, w := range []string{`onclick="qorm(`, `ondblclick="qorm(`, `qormLong(`, "cursor:pointer"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("gesturedetector lacks %q:\n%s", w, res.HTML)
			}
		}
		if len(res.Handlers) != 3 {
			t.Errorf("gesturedetector should register tap/dbl/long, got %d", len(res.Handlers))
		}
	})

	t.Run("dismissible", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dismissible", ID: "dm",
			Props:    map[string]any{"onDismissed": map[string]any{"name": "gone"}},
			Children: textKids("row")})
		for _, w := range []string{`qorm-dismiss-content`, `qormSwipe(`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("dismissible lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("draggable", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "draggable", ID: "dg", Props: map[string]any{"data": "payload"}, Children: textKids("x")})
		for _, w := range []string{`qorm-draggable`, `data-qorm-drag="payload"`, `qormDragInit()`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("draggable lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("dragtarget", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "dragtarget", ID: "dt", Props: map[string]any{"onDrop": map[string]any{"name": "drop"}}, Children: textKids("x")})
		for _, w := range []string{`qorm-droptarget`, `data-qorm-drop=`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("dragtarget lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("switchlisttile", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "switchlisttile", ID: "slt", Label: "Enable", Value: "{{ state.on }}"},
			map[string]any{"on": true})
		for _, w := range []string{`qorm-switch`, "Enable", " checked"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("switchlisttile lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("checkboxlisttile", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "checkboxlisttile", ID: "clt", Label: "Agree"})
		if !strings.Contains(res.HTML, `type="checkbox"`) || !strings.Contains(res.HTML, "Agree") {
			t.Errorf("checkboxlisttile lacks checkbox:\n%s", res.HTML)
		}
	})

	t.Run("contextmenu-items", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "contextmenu", ID: "cx",
			Props:    map[string]any{"items": []any{map[string]any{"id": "e", "title": "Edit", "icon": "copy"}}},
			Children: textKids("target")})
		for _, w := range []string{`qorm-ctxmenu`, "Edit", "target"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("contextmenu lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("contextmenu-actions", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "contextmenu", ID: "cx2",
			Props:    map[string]any{"actions": []any{map[string]any{"label": "Copy", "onPress": map[string]any{"name": "cp"}}}},
			Children: textKids("target")})
		for _, w := range []string{`qorm-ctx`, `qormCtx(`, "Copy"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("contextmenu(actions) lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("refreshindicator", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "refreshindicator", ID: "ri",
			Props:    map[string]any{"onRefresh": map[string]any{"name": "reload"}},
			Children: textKids("content")})
		for _, w := range []string{`qorm-refresh-spin`, `qormRefresh(`, "content"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("refreshindicator lacks %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestRichInputWidgets covers the composite input widgets.
func TestRichInputWidgets(t *testing.T) {
	opts := []any{
		map[string]any{"value": "a", "label": "Apple"},
		map[string]any{"value": "b", "label": "Banana"},
	}

	t.Run("segmented-single", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "segmented", ID: "sg", Value: "{{ state.c }}", Props: map[string]any{"options": opts}},
			map[string]any{"c": "b"})
		for _, w := range []string{`qorm-seg`, `role="radiogroup"`, "Apple", "Banana", `value="b" checked`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("segmented lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("segmented-multi", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "segmented", ID: "sgm", Value: "{{ state.sel }}", Props: map[string]any{"options": opts, "multiple": true},
				OnChange: &model.Invoke{Name: "toggle"}},
			map[string]any{"sel": []any{"a"}})
		for _, w := range []string{`role="group"`, `aria-pressed="true"`, `aria-pressed="false"`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("segmented(multi) lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("chip-selected", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "filterchip", ID: "ch", Label: "Tag", Props: map[string]any{"selected": "true"}})
		for _, w := range []string{"Tag", "background:var(--accent)", "<svg"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("selected filterchip lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("rangeslider", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "rangeslider", ID: "rs", Props: map[string]any{"low": "{{ state.lo }}", "high": "{{ state.hi }}"}},
			map[string]any{"lo": float64(20), "hi": float64(80)})
		for _, w := range []string{`qorm-range-lo`, `qorm-range-hi`, `value="20"`, `value="80"`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("rangeslider lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("dropdownbutton", func(t *testing.T) {
		res := renderWidgetState(t,
			&model.Node{Type: "dropdownbutton", ID: "dd", Value: "{{ state.c }}", Props: map[string]any{"options": opts}},
			map[string]any{"c": "a"})
		for _, w := range []string{`qorm-menu`, `qormMenu(this)`, "Apple", "Banana"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("dropdownbutton lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("autocomplete", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "autocomplete", ID: "acp", Props: map[string]any{"options": opts}})
		for _, w := range []string{`list="acp-ac"`, `<datalist`, `<option value="Apple">`} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("autocomplete lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("searchbar", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "searchbar", ID: "sb2", Props: map[string]any{
			"hint":  "Find",
			"items": []any{map[string]any{"label": "Result1", "detail": "d1"}},
		}})
		for _, w := range []string{`qorm-search`, "Find", "Result1", "d1"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("searchbar lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("textformfield-error", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "textformfield", ID: "tf", Props: map[string]any{
			"label": "Email", "error": "Required", "maxLength": float64(10)}, Value: "abc"})
		for _, w := range []string{"Email", "Required", "#ef4444", "3/10"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("textformfield lacks %q:\n%s", w, res.HTML)
			}
		}
	})

	t.Run("field-required-help", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "field", ID: "fd", Props: map[string]any{
			"label": "Name", "required": true, "help": "Your name"}, Children: textKids("x")})
		for _, w := range []string{"Name", "*", "Your name"} {
			if !strings.Contains(res.HTML, w) {
				t.Errorf("field lacks %q:\n%s", w, res.HTML)
			}
		}
	})
}

// TestAnimationWidgets covers the motion wrappers and their keyframe mapping.
func TestAnimationWidgets(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
		want []string
	}{
		{"motion-pop", &model.Node{Type: "motion", ID: "mo", Props: map[string]any{"animation": "pop"}, Children: textKids("x")}, []string{"animation:qa-pop"}},
		{"motion-default-type", &model.Node{Type: "fadetransition", ID: "ft", Children: textKids("x")}, []string{"animation:qa-fade"}},
		{"animatedcontainer", &model.Node{Type: "animatedcontainer", ID: "anc", Children: textKids("x")}, []string{"transition:all 300ms"}},
		{"animatedopacity", &model.Node{Type: "animatedopacity", ID: "ano", Props: map[string]any{"opacity": "0.5"}, Children: textKids("x")}, []string{"opacity:0.5", "transition:opacity"}},
		{"transform", &model.Node{Type: "transform", ID: "tfm", Props: map[string]any{"rotate": float64(45), "scale": float64(2)}, Children: textKids("x")}, []string{"transform:rotate(45deg) scale(2)"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("html lacks %q:\n%s", w, res.HTML)
				}
			}
		})
	}
}
