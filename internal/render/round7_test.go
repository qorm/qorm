package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

// xssScriptID is a node id that tries to terminate the inline <script> block
// that embeds it: the HTML parser closes a <script> on the literal bytes
// "</script" no matter how the JS string is quoted, so an unescaped id breaks
// out of the wiring script and injects a second one. In an agent-native app
// node ids are author-set, so this is a live vector, not a theoretical one.
const xssScriptID = `foo</script><script>alert(1)</script>`

// jsEscapeLT is the six-character JS escape jsStringID rewrites "<" to inside
// a <script> body: a backslash followed by "u003c". Built from rune(92) so the
// test embeds no backslash literal of its own.
var jsEscapeLT = string(rune(92)) + "u003c"

// assertScriptIDSafe verifies an adversarial id embedded in an inline wiring
// <script> (via getElementById) cannot break out: no raw "</script><script>"
// pair may survive, the injected script must be gone, the id must appear in
// its close-tag-neutralised form ("<" rewritten as the JS escape jsEscapeLT),
// and the widget's own script block must be the ONLY <script> in the output
// (i.e. nothing opened early).
func assertScriptIDSafe(t *testing.T, html, ctx string) {
	t.Helper()
	if strings.Contains(html, `</script><script>`) || strings.Contains(html, `<script>alert(1)</script>`) {
		t.Errorf("%s: id broke out of the inline <script> (raw close-tag survived):\n%s", ctx, html)
	}
	if !strings.Contains(html, `getElementById("foo`+jsEscapeLT+`/script>`) {
		t.Errorf("%s: id inside the <script> must have its close-tag sequence escaped (%s):\n%s", ctx, jsEscapeLT, html)
	}
	if got := strings.Count(html, "<script>"); got != 1 {
		t.Errorf("%s: want exactly 1 <script> block (the widget's own wiring), got %d:\n%s", ctx, got, html)
	}
}

// TestEscapingScriptCloseTagBreakout sweeps a close-tag id through every
// inline-script wiring site — the six getElementById emitters (long-press,
// dismissible, swipe-actions, context menu, refresh indicator, reorderable
// list) — and asserts none lets the id terminate the <script> early. Round 6
// fixed the id ATTRIBUTE (attrID) but left these raw: %q is correct for the
// JS string yet the HTML parser still ends the script on "</script".
func TestEscapingScriptCloseTagBreakout(t *testing.T) {
	invoke := func(name string) map[string]any { return map[string]any{"name": name} }
	cases := []struct {
		name  string
		node  *model.Node
		state map[string]any
	}{
		{"gesture-longpress", &model.Node{Type: "gesture", ID: xssScriptID, Props: map[string]any{"onLongPress": invoke("lp")}, Children: textKids("c")}, nil},
		{"dismissible", &model.Node{Type: "dismissible", ID: xssScriptID, OnPress: &model.Invoke{Name: "act"}, Children: textKids("c")}, nil},
		{"swipeactions", &model.Node{Type: "swipeactions", ID: xssScriptID, Children: textKids("c")}, nil},
		{"contextmenu", &model.Node{Type: "contextmenu", ID: xssScriptID, Children: textKids("c")}, nil},
		{"refreshindicator", &model.Node{Type: "refreshindicator", ID: xssScriptID, Props: map[string]any{"onRefresh": invoke("rf")}, Children: textKids("c")}, nil},
		{"list-reorder", &model.Node{Type: "list", ID: xssScriptID, Data: "{{ state.rows }}",
			Props:    map[string]any{"reorderable": true, "onReorder": invoke("mv")},
			Template: &model.Node{Type: "text", ID: "r", Text: "{{ item }}"}},
			map[string]any{"rows": []any{"a", "b"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidgetState(t, tc.node, tc.state)
			assertScriptIDSafe(t, res.HTML, tc.name)
			// The id ATTRIBUTE must keep its round-6 entity escaping — the two
			// contexts are escaped differently (entities for the attribute,
			// the JS jsEscapeLT escape inside the script) yet decode to the
			// same id, so the getElementById lookup still matches element.id
			// at run time.
			if !strings.Contains(res.HTML, `id="foo&lt;/script&gt;`) {
				t.Errorf("%s: id attribute must stay entity-escaped:\n%s", tc.name, res.HTML)
			}
		})
	}

	t.Run("normal-id-wires-verbatim", func(t *testing.T) {
		// jsStringID is a no-op for ordinary ids (no "<"), so safe ids wire
		// exactly as before — the client's getElementById keeps working.
		res := renderWidget(t, &model.Node{Type: "swipeactions", ID: "sw-1", Children: textKids("c")})
		if !strings.Contains(res.HTML, `qormSwipeActions(document.getElementById("sw-1"))`) {
			t.Errorf("safe id must wire verbatim:\n%s", res.HTML)
		}
		if strings.Contains(res.HTML, jsEscapeLT) {
			t.Errorf("safe id must not be escape-encoded:\n%s", res.HTML)
		}
	})
}

// TestEscapingRadioSegmentedName guards the radio-grouping name= attribute:
// it is the node id in a quoted HTML attribute, so it needs the same entity
// encoding as id= (%q alone leaves the quote-breakout open).
func TestEscapingRadioSegmentedName(t *testing.T) {
	opts := map[string]any{"options": []any{map[string]any{"value": "a", "label": "A"}}}
	cases := []struct {
		name string
		node *model.Node
	}{
		{"radio-name", &model.Node{Type: "radio", ID: xssAttr, Props: opts}},
		{"segmented-name", &model.Node{Type: "segmented", ID: xssAttr, Props: opts}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, tc.node)
			if !strings.Contains(res.HTML, `name="&#34;`) {
				t.Errorf("%s: name= must entity-encode the quote breakout:\n%s", tc.name, res.HTML)
			}
			if strings.Contains(res.HTML, "<script>QORMXSS") {
				t.Errorf("%s: name= breakout leaked a raw script tag:\n%s", tc.name, res.HTML)
			}
		})
	}

	t.Run("normal-name-unchanged", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "radio", ID: "rd", Props: opts})
		if !strings.Contains(res.HTML, `name="rd"`) {
			t.Errorf("safe id must render as the radio name verbatim:\n%s", res.HTML)
		}
	})
}

// TestEscapingDataScene guards the data-scene attribute RenderScene tags onto
// the scene root: the key comes from the sceneID parameter (caller/author
// input), so it is escaped like every other attribute value.
func TestEscapingDataScene(t *testing.T) {
	app := &model.App{
		Entry:  "home",
		Scenes: map[string]*model.Node{"home": {Type: "column", ID: "h", Children: textKids("HOME")}},
	}
	rt := runtime.New(app)

	got := RenderScene(rt, xssAttr).HTML
	if strings.Contains(got, "<script>QORMXSS") {
		t.Errorf("data-scene breakout leaked a raw script tag:\n%s", got)
	}
	if !strings.Contains(got, `data-scene="&#34;`) {
		t.Errorf("data-scene must entity-encode the quote breakout:\n%s", got)
	}

	// A safe scene id still tags verbatim (the client's transition lookup is
	// keyed on it) — see also TestSceneTagging for the entry/named scenes.
	if safe := RenderScene(rt, "details").HTML; !strings.Contains(safe, `data-scene="details"`) {
		t.Errorf("safe scene id must tag verbatim:\n%s", safe)
	}
}

// TestSafeURL pins the href scheme allowlist: relative/#/protocol-relative
// and http/https/mailto/tel (plus the app's asset scheme) pass through;
// everything else collapses to an inert "#". The adversarial cases exercise
// the parser-equivalent normalisation (case, leading/embedded whitespace and
// control characters) that a naive prefix check would miss.
func TestSafeURL(t *testing.T) {
	cases := []struct{ in, want string }{
		// allowed: safe schemes
		{"https://example.com/a?b=c#d", "https://example.com/a?b=c#d"},
		{"http://example.com", "http://example.com"},
		{"HTTPS://EXAMPLE.COM/X", "HTTPS://EXAMPLE.COM/X"},
		{"mailto:user@example.com", "mailto:user@example.com"},
		{"tel:+1-555-0100", "tel:+1-555-0100"},
		{"qormapp://assets/logo.png", "qormapp://assets/logo.png"},
		// allowed: schemeless (relative / fragment / protocol-relative)
		{"/docs/intro", "/docs/intro"},
		{"docs/intro", "docs/intro"},
		{"./x", "./x"},
		{"#top", "#top"},
		{"//cdn.example.com/lib.js", "//cdn.example.com/lib.js"},
		{"page.html#sec:2", "page.html#sec:2"}, // '#' before ':' -> no scheme
		{"a/b:c", "a/b:c"},                     // '/' before ':' -> no scheme
		{"", ""},
		// rejected: dangerous schemes, incl. parser-normalised disguises
		{"javascript:alert(1)", "#"},
		{"JavaScript:alert(1)", "#"},
		{"  javascript:alert(1)", "#"},
		{"javascript\t:alert(1)", "#"},
		{"java\tscript:alert(1)", "#"},
		{"java\nscript:alert(1)", "#"},
		{"java\rscript:alert(1)", "#"},
		{"\x00javascript:alert(1)", "#"},
		{"data:text/html,<b>x</b>", "#"},
		{"DATA:text/html;base64,PHNjcmlwdD4", "#"},
		{"data:image/svg+xml,<svg onload=alert(1)>", "#"},
		{"vbscript:msgbox(1)", "#"},
		{"file:///etc/passwd", "#"},
		{"ftp://example.com/x", "#"},
		{"about:blank", "#"},
		{"blob:https://example.com/uuid", "#"},
	}
	for _, tc := range cases {
		if got := safeURL(tc.in); got != tc.want {
			t.Errorf("safeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestLinkHrefSchemeAllowlist drives the same matrix through the rendered
// link widget: a dangerous href prop must never reach the anchor — neither as
// a live scheme nor as an attribute breakout — while safe URLs pass through.
func TestLinkHrefSchemeAllowlist(t *testing.T) {
	cases := []struct{ href, want string }{
		{"https://example.com/x", `href="https://example.com/x"`},
		{"http://example.com", `href="http://example.com"`},
		{"mailto:user@example.com", `href="mailto:user@example.com"`},
		{"tel:+15550100", `href="tel:+15550100"`},
		{"/docs/intro", `href="/docs/intro"`},
		{"docs/intro", `href="docs/intro"`},
		{"#top", `href="#top"`},
		{"javascript:alert(1)", `href="#"`},
		{"  JavaScript:alert(1)", `href="#"`},
		{"java\tscript:alert(1)", `href="#"`},
		{"data:text/html,<script>alert(1)</script>", `href="#"`},
		{"vbscript:msgbox(1)", `href="#"`},
	}
	for _, tc := range cases {
		res := renderWidget(t, &model.Node{Type: "link", ID: "l", Label: "X", Props: map[string]any{"href": tc.href}})
		if !strings.Contains(res.HTML, tc.want) {
			t.Errorf("link href=%q: want %s in output:\n%s", tc.href, tc.want, res.HTML)
		}
		if strings.Contains(res.HTML, "alert(1)") || strings.Contains(res.HTML, "msgbox") {
			t.Errorf("link href=%q: dangerous payload leaked into output:\n%s", tc.href, res.HTML)
		}
	}

	t.Run("default-href-is-the-renderers-own-noop", func(t *testing.T) {
		// With no href prop the renderer emits its OWN constant placeholder
		// (never author data), so a bare onPress link stays clickable without
		// navigating; safeURL governs only author-supplied values.
		res := renderWidget(t, &model.Node{Type: "link", ID: "l", Label: "X"})
		if !strings.Contains(res.HTML, `href="javascript:void(0)"`) {
			t.Errorf("bare link must keep the renderer's no-navigation default:\n%s", res.HTML)
		}
	})
}

// TestEscapingBoundAttributeBreakout closes the same %q-in-attribute class as
// round 6's id fix on the state-bound emitters round 6 left raw: the recorder/
// camera/biometric/location hidden values and the camera/recorder src (the
// value rides straight from a {{ state.* }} binding into a quoted attribute).
// The inputType cases cover the analogous author-prop attribute on inputs.
func TestEscapingBoundAttributeBreakout(t *testing.T) {
	state := map[string]any{"v": xssAttr}
	cases := []struct {
		name string
		node *model.Node
	}{
		{"biometric-hidden-value", &model.Node{Type: "biometric", ID: "e", Value: "{{ state.v }}"}},
		{"location-hidden-value", &model.Node{Type: "location", ID: "e", Value: "{{ state.v }}"}},
		{"recorder-audio-src-and-hidden-value", &model.Node{Type: "recorder", ID: "e", Value: "{{ state.v }}"}},
		{"camera-preview-src-and-hidden-value", &model.Node{Type: "camera", ID: "e", Value: "{{ state.v }}"}},
		{"input-inputtype", &model.Node{Type: "input", ID: "e", Props: map[string]any{"inputType": xssAttr}}},
		{"textformfield-inputtype", &model.Node{Type: "textformfield", ID: "e", Props: map[string]any{"inputType": xssAttr}}},
		{"radiolisttile-value", &model.Node{Type: "radiolisttile", ID: "e", Label: "L", Value: "{{ state.v }}", Props: map[string]any{"value": xssAttr}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertAttrSafe(t, renderWidgetState(t, tc.node, state).HTML, tc.name)
		})
	}

	t.Run("recorder-data-url-passes-through", func(t *testing.T) {
		// The recorder/camera transport recorded media as data: URLs BY
		// DESIGN: media src is a non-navigating context (no scheme executes
		// script off an <img>/<audio>), so only the attribute is escaped and
		// a legitimate data: URL must survive intact.
		res := renderWidgetState(t, &model.Node{Type: "recorder", ID: "e", Value: "{{ state.v }}"},
			map[string]any{"v": "data:audio/webm;base64,T2s"})
		if !strings.Contains(res.HTML, `src="data:audio/webm;base64,T2s"`) {
			t.Errorf("legitimate data: URL must render verbatim:\n%s", res.HTML)
		}
	})
}
