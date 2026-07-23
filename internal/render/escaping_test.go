package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
)

// xss is the injection payload for TEXT contexts. The distinctive QORMXSS marker
// lets the assertions tell an injected tag from the renderer's own legitimate
// inline <script>setTimeout(...)> wiring.
const xss = `<script>QORMXSS</script>`

// xssAttr is a quote-breakout payload for ATTRIBUTE contexts: it tries to close
// the surrounding attribute and open a new tag.
const xssAttr = `"><script>QORMXSS</script>`

// assertXSSEscaped verifies a text-context payload was HTML-escaped: the raw tag
// must be gone and its escaped form present.
func assertXSSEscaped(t *testing.T, html, ctx string) {
	t.Helper()
	if strings.Contains(html, "<script>QORMXSS") {
		t.Errorf("%s: raw <script> leaked (text not escaped):\n%s", ctx, html)
	}
	if !strings.Contains(html, "&lt;script&gt;QORMXSS") {
		t.Errorf("%s: payload should appear in escaped form:\n%s", ctx, html)
	}
}

// assertAttrSafe verifies an attribute-context payload cannot break out: the
// double quote is entity-encoded (so no new attribute/tag can start) and no raw
// script tag survives.
func assertAttrSafe(t *testing.T, html, ctx string) {
	t.Helper()
	if strings.Contains(html, "<script>QORMXSS") {
		t.Errorf("%s: attribute breakout leaked a raw script tag:\n%s", ctx, html)
	}
	if !strings.Contains(html, "&#34;") {
		t.Errorf("%s: double-quote not escaped — attribute breakout possible:\n%s", ctx, html)
	}
}

// TestEscapingTextContent sweeps the payload through every text/label content
// path the renderer touches and asserts each one HTML-escapes it. Any widget
// that drops html.EscapeString leaks the raw tag and fails here.
func TestEscapingTextContent(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
	}{
		{"text", &model.Node{Type: "text", ID: "e", Text: xss}},
		{"selectabletext", &model.Node{Type: "selectabletext", ID: "e", Text: xss}},
		{"button", &model.Node{Type: "button", ID: "e", Label: xss}},
		{"link", &model.Node{Type: "link", ID: "e", Label: xss}},
		{"fab", &model.Node{Type: "fab", ID: "e", Label: xss}},
		{"tag", &model.Node{Type: "tag", ID: "e", Label: xss}},
		{"badge", &model.Node{Type: "badge", ID: "e", Label: xss}},
		{"appbar-title", &model.Node{Type: "appbar", ID: "e", Label: xss}},
		{"largetitle", &model.Node{Type: "largetitle", ID: "e", Label: xss, Props: map[string]any{"subtitle": xss}}},
		{"alert", &model.Node{Type: "alert", ID: "e", Props: map[string]any{"title": xss}, Text: xss}},
		{"stat", &model.Node{Type: "stat", ID: "e", Props: map[string]any{"value": xss, "label": xss, "delta": xss}}},
		{"breadcrumb", &model.Node{Type: "breadcrumb", ID: "e", Props: map[string]any{"items": []any{xss}}}},
		{"steps", &model.Node{Type: "steps", ID: "e", Props: map[string]any{"steps": []any{xss}}}},
		{"timeline", &model.Node{Type: "timeline", ID: "e", Props: map[string]any{"items": []any{map[string]any{"title": xss, "text": xss}}}}},
		{"descriptions", &model.Node{Type: "descriptions", ID: "e", Props: map[string]any{"items": []any{map[string]any{"label": xss, "value": xss}}}}},
		{"empty", &model.Node{Type: "empty", ID: "e", Props: map[string]any{"title": xss}, Text: xss}},
		{"listtile", &model.Node{Type: "listtile", ID: "e", Label: xss, Props: map[string]any{"subtitle": xss, "trailing": xss}}},
		{"field", &model.Node{Type: "field", ID: "e", Props: map[string]any{"label": xss, "error": xss, "help": xss}}},
		{"textformfield", &model.Node{Type: "textformfield", ID: "e", Props: map[string]any{"label": xss, "error": xss, "helper": xss, "prefix": xss, "suffix": xss}}},
		{"menu", &model.Node{Type: "menu", ID: "e", Label: xss, Props: map[string]any{"items": []any{map[string]any{"label": xss}}}}},
		{"chip", &model.Node{Type: "chip", ID: "e", Label: xss}},
		{"segmented", &model.Node{Type: "segmented", ID: "e", Props: map[string]any{"options": []any{map[string]any{"value": xss, "label": xss}}}}},
		{"dropdownbutton", &model.Node{Type: "dropdownbutton", ID: "e", Props: map[string]any{"options": []any{map[string]any{"value": "v", "label": xss}}}}},
		{"searchbar", &model.Node{Type: "searchbar", ID: "e", Props: map[string]any{"hint": xss, "items": []any{map[string]any{"label": xss, "detail": xss}}}}},
		{"richtext", &model.Node{Type: "richtext", ID: "e", Props: map[string]any{"spans": []any{map[string]any{"text": xss}}}}},
		{"bottomnav", &model.Node{Type: "bottomnav", ID: "e", Props: map[string]any{"items": []any{map[string]any{"value": "v", "label": xss}}}}},
		{"navigationrail", &model.Node{Type: "navigationrail", ID: "e", Props: map[string]any{"items": []any{map[string]any{"value": "v", "label": xss}}}}},
		{"navigationdrawer", &model.Node{Type: "navigationdrawer", ID: "e", Props: map[string]any{"items": []any{map[string]any{"value": "v", "label": xss}}}}},
		{"expansiontile", &model.Node{Type: "expansiontile", ID: "e", Label: xss, Children: textKids("b")}},
		{"accordion", &model.Node{Type: "accordion", ID: "e", Children: []*model.Node{{Type: "column", ID: "s", Props: map[string]any{"title": xss}, Children: textKids("b")}}}},
		{"tabs", &model.Node{Type: "tabs", ID: "e", Props: map[string]any{"tabs": []any{xss}}, Children: textKids("p")}},
		{"table-cell-and-header", &model.Node{Type: "table", ID: "e", Props: map[string]any{
			"columns": []any{map[string]any{"value": "k", "label": xss}},
			"data":    []any{map[string]any{"k": xss}}}}},
		{"datatable-cell-and-header", &model.Node{Type: "datatable", ID: "e", Props: map[string]any{
			"columns": []any{map[string]any{"value": "k", "label": xss}},
			"data":    []any{map[string]any{"k": xss}}}}},
		{"contextmenu-items", &model.Node{Type: "contextmenu", ID: "e", Props: map[string]any{"items": []any{map[string]any{"id": "i", "title": xss}}}, Children: textKids("t")}},
		{"treemap-label", &model.Node{Type: "tree", ID: "e", Props: map[string]any{"data": []any{map[string]any{"label": xss}}}}},
		{"listsection-header-footer", &model.Node{Type: "listsection", ID: "e", Props: map[string]any{"header": xss, "footer": xss}, Children: textKids("r")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			assertXSSEscaped(t, res.HTML, tc.name)
		})
	}
}

// TestEscapingAttributeBreakout sweeps the quote-breakout payload through the
// attribute paths (href/src/value/placeholder + the a11y attributes) and asserts
// the quote is entity-encoded so the value cannot escape its attribute.
func TestEscapingAttributeBreakout(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
	}{
		{"link-href", &model.Node{Type: "link", ID: "e", Label: "x", Props: map[string]any{"href": xssAttr}}},
		{"image-src-alt", &model.Node{Type: "image", ID: "e", Props: map[string]any{"src": xssAttr, "alt": xssAttr}}},
		{"video-src", &model.Node{Type: "video", ID: "e", Props: map[string]any{"src": xssAttr}}},
		{"avatar-src", &model.Node{Type: "avatar", ID: "e", Props: map[string]any{"src": xssAttr}}},
		{"input-value-placeholder", &model.Node{Type: "input", ID: "e", Value: xssAttr, Placeholder: xssAttr}},
		{"textarea-placeholder", &model.Node{Type: "textarea", ID: "e", Placeholder: xssAttr, Value: xssAttr}},
		{"select-option", &model.Node{Type: "select", ID: "e", Props: map[string]any{"options": []any{map[string]any{"value": xssAttr, "label": xssAttr}}}}},
		{"a11y-attrs", &model.Node{Type: "text", ID: "e", Text: "x", Props: map[string]any{"role": xssAttr, "ariaLabel": xssAttr, "title": xssAttr, "tooltip": xssAttr}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			assertAttrSafe(t, res.HTML, tc.name)
		})
	}
}

// TestEscapingBoundState confirms the primary adversarial path — untrusted data
// arriving through state bindings — is escaped on representative widgets, and
// that an injected binding-looking string in a literal prop is NOT re-evaluated.
func TestEscapingBoundState(t *testing.T) {
	state := map[string]any{"v": `<script>QORMXSS</script>`}

	t.Run("bound-text", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{Type: "text", ID: "e", Text: "{{ state.v }}"}, state)
		assertXSSEscaped(t, res.HTML, "bound-text")
	})
	t.Run("bound-table-cell", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{Type: "table", ID: "e", Props: map[string]any{
			"columns": []any{map[string]any{"value": "k", "label": "K"}},
			"data":    "{{ state.rows }}"}},
			map[string]any{"rows": []any{map[string]any{"k": `<script>QORMXSS</script>`}}})
		assertXSSEscaped(t, res.HTML, "bound-table-cell")
	})
	t.Run("bound-list-item", func(t *testing.T) {
		res := renderWidgetState(t, &model.Node{Type: "list", ID: "e", Data: "{{ state.rows }}",
			Template: &model.Node{Type: "text", ID: "r", Text: "{{ item.k }}"}},
			map[string]any{"rows": []any{map[string]any{"k": `<script>QORMXSS</script>`}}})
		assertXSSEscaped(t, res.HTML, "bound-list-item")
	})
}

// TestEscapingFixedBugs regression-guards TWO HTML-injection defects that were
// fixed in round 5 (this test previously documented the vulnerable behaviour
// and was written to flip red once fixed — it now asserts the CORRECTED
// behaviour and must stay green).
//
// Root cause (shared): these paths built an HTML attribute with Go's %q, whose
// backslash quoting is meaningless to an HTML parser. A double quote in the
// value was emitted as \" — the HTML parser treats the backslash as a literal
// character and the quote still TERMINATES the attribute, so "><script>…
// broke out. The fix entity-encodes the value (html.EscapeString for notify's
// data-title/data-body; attrID — html.EscapeString — at every id-attribute
// emission site), so the quote can no longer escape its attribute.
func TestEscapingFixedBugs(t *testing.T) {
	t.Run("notify-data-title-body-escaped", func(t *testing.T) {
		// render_feedback.go notify(): data-title/data-body are html.EscapeString'd
		// so an untrusted notification title/body cannot inject markup.
		res := renderWidget(t, &model.Node{Type: "notify", ID: "e", Placeholder: xssAttr, Text: xssAttr})
		assertAttrSafe(t, res.HTML, "notify-data-title-body")
	})

	t.Run("id-attribute-escaped", func(t *testing.T) {
		// Every widget emits its node id through attrID (html.EscapeString), so an
		// adversarial node id cannot break out of the id attribute. text is the
		// minimal emitter; the fix is systemic across the widget switch.
		res := renderWidget(t, &model.Node{Type: "text", ID: xssAttr, Text: "hi"})
		assertAttrSafe(t, res.HTML, "id-attribute")
	})

	t.Run("id-attribute-escaped-across-widgets", func(t *testing.T) {
		// The id fix must hold at EVERY emission site — the plain n.ID form, the
		// r.nid(n) form (list-item-unique ids), the derived id="%s-out" form and
		// the unknown-widget fallback. Sweep representative emitters of each.
		// (Widgets that also inline the raw id into a <script> getElementById are
		// excluded: the script context is a separate concern, and %q is correct
		// there. These nodes carry no handlers, so none emit a script.)
		nodes := []*model.Node{
			{Type: "button", ID: xssAttr, Label: "x"},
			{Type: "image", ID: xssAttr, Props: map[string]any{"src": "p.png"}},
			{Type: "slider", ID: xssAttr},
			{Type: "modal", ID: xssAttr, Props: map[string]any{"open": "true"}},
			{Type: "chart", ID: xssAttr, Props: map[string]any{"data": []any{1.0, 2.0}}},
			{Type: "datatable", ID: xssAttr, Props: map[string]any{"columns": []any{"a"}}},
			{Type: "notify", ID: xssAttr},                                                     // id + id="%s-out"
			{Type: "gesture", ID: xssAttr, Children: textKids("c")},                           // r.nid(n)
			{Type: "dismissible", ID: xssAttr, Children: textKids("c")},                       // r.nid(n)
			{Type: "autocomplete", ID: xssAttr, Props: map[string]any{"options": []any{"o"}}}, // datalist id + list ref
			{Type: "colunm", ID: xssAttr},                                                     // unknown-widget fallback
		}
		for _, n := range nodes {
			assertAttrSafe(t, renderWidget(t, n).HTML, n.Type+"-id")
		}
	})

	t.Run("safe-id-unchanged-and-handlers-unaffected", func(t *testing.T) {
		// attrID is a no-op for ordinary ids (no <, >, &, quotes), so exact-HTML
		// consumers are unaffected; the handler table keys on the registration
		// index in onclick="qorm(N)", never on the id, so it is untouched too.
		res := renderWidget(t, &model.Node{Type: "button", ID: "go-1", Label: "Go",
			OnPress: &model.Invoke{Name: "act"}})
		if !strings.Contains(res.HTML, `<button id="go-1"`) {
			t.Errorf("safe id must render verbatim (no escaping):\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, "&#") {
			t.Errorf("safe id must not be entity-encoded:\n%s", res.HTML)
		}
		if !strings.Contains(res.HTML, `onclick="qorm(0)"`) || len(res.Handlers) != 1 || res.Handlers[0].Name != "act" {
			t.Errorf("handler table must be unaffected by id escaping: %+v\n%s", res.Handlers, res.HTML)
		}
	})
}
