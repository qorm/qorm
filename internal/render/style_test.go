package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
	qrt "github.com/qorm/platform/internal/runtime"
)

// styleHTML renders a single text node carrying the given style map and returns
// the HTML. A text node runs both boxCSS and textCSS over n.Style, so it is a
// compact way to assert on the emitted CSS declarations.
func styleHTML(t *testing.T, style map[string]any, layout map[string]any) string {
	t.Helper()
	res := renderWidget(t, &model.Node{Type: "text", ID: "s", Text: "x", Style: style, Layout: layout})
	return res.HTML
}

// TestBoxCSSDeclarations exercises every box-model declaration boxCSS can emit.
// Each case sets one style key (plus a collaborator where a declaration depends
// on two, e.g. borderWidth+borderColor) and asserts the exact CSS appears.
func TestBoxCSSDeclarations(t *testing.T) {
	cases := []struct {
		name  string
		style map[string]any
		want  []string
	}{
		{"min-width", map[string]any{"minWidth": float64(10)}, []string{"min-width:10px;"}},
		{"max-width", map[string]any{"maxWidth": float64(200)}, []string{"max-width:200px;"}},
		{"min-height", map[string]any{"minHeight": float64(12)}, []string{"min-height:12px;"}},
		{"max-height", map[string]any{"maxHeight": float64(240)}, []string{"max-height:240px;"}},
		{"flex-grow", map[string]any{"flexGrow": float64(2)}, []string{"flex-grow:2;flex-basis:0;"}},
		{"aspect-ratio", map[string]any{"aspectRatio": float64(1.5)}, []string{"aspect-ratio:1.5;"}},
		{"background", map[string]any{"background": "red"}, []string{"background:red;"}},
		{"gradient", map[string]any{"gradient": "linear-gradient(1,2)"}, []string{"background:linear-gradient(1,2);"}},
		{"border-radius", map[string]any{"borderRadius": float64(8)}, []string{"border-radius:8px;"}},
		{"border-width-default-color", map[string]any{"borderWidth": float64(2)}, []string{"border:2px solid var(--sep);"}},
		{"border-width-color", map[string]any{"borderWidth": float64(2), "borderColor": "blue"}, []string{"border:2px solid blue;"}},
		{"gap", map[string]any{"gap": float64(12)}, []string{"gap:12px;"}},
		{"opacity", map[string]any{"opacity": float64(0.5)}, []string{"opacity:0.5;"}},
		{"shadow", map[string]any{"shadow": "0 1px 2px #000"}, []string{"box-shadow:0 1px 2px #000;"}},
		{"position-edges", map[string]any{"position": "absolute", "top": float64(5), "left": float64(10)}, []string{"position:absolute;", "top:5px;", "left:10px;"}},
		{"cursor", map[string]any{"cursor": "pointer"}, []string{"cursor:pointer;"}},
		{"transition", map[string]any{"transition": "all .2s"}, []string{"transition:all .2s;"}},
		{"padding-scalar", map[string]any{"padding": float64(8)}, []string{"padding:8px;"}},
		{"padding-edges", map[string]any{"padding": map[string]any{"top": float64(1), "right": float64(2), "bottom": float64(3), "left": float64(4)}}, []string{"padding:1px 2px 3px 4px;"}},
		{"margin-scalar", map[string]any{"margin": float64(6)}, []string{"margin:6px;"}},
		{"margin-edges", map[string]any{"margin": map[string]any{"top": float64(4), "right": float64(3), "bottom": float64(2), "left": float64(1)}}, []string{"margin:4px 3px 2px 1px;"}},
		{"width-fill", map[string]any{"width": "fill"}, []string{"width:100%;"}},
		{"height-px", map[string]any{"height": float64(50)}, []string{"height:50px;"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := styleHTML(t, tc.style, nil)
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("style %v: html lacks %q:\n%s", tc.style, w, html)
				}
			}
		})
	}

	// elevated prop (not a style key) supplies a default shadow only when no
	// explicit shadow is set.
	t.Run("elevated-default-shadow", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "e", Text: "x", Props: map[string]any{"elevated": true}})
		if !strings.Contains(res.HTML, "box-shadow:0 4px 12px rgba(0,0,0,.12);") {
			t.Errorf("elevated should add the default shadow:\n%s", res.HTML)
		}
	})
	t.Run("elevated-defers-to-shadow", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "e2", Text: "x",
			Props: map[string]any{"elevated": true}, Style: map[string]any{"shadow": "none"}})
		if strings.Contains(res.HTML, "box-shadow:0 4px 12px") {
			t.Errorf("explicit shadow must win over the elevated default:\n%s", res.HTML)
		}
	})
}

// TestTextCSSDeclarations exercises every text declaration textCSS can emit.
func TestTextCSSDeclarations(t *testing.T) {
	cases := []struct {
		name    string
		style   map[string]any
		props   map[string]any
		want    []string
		wantNot []string
	}{
		{"color", map[string]any{"color": "red"}, nil, []string{"color:red;"}, nil},
		{"font-size", map[string]any{"fontSize": float64(20)}, nil, []string{"font-size:20px;"}, []string{"font-size:15px;"}},
		{"font-weight", map[string]any{"fontWeight": float64(700)}, nil, []string{"font-weight:700;"}, nil},
		{"font-family", map[string]any{"fontFamily": "Arial"}, nil, []string{"font-family:Arial;"}, nil},
		{"line-height", map[string]any{"lineHeight": float64(1.5)}, nil, []string{"line-height:1.5;"}, nil},
		{"letter-spacing", map[string]any{"letterSpacing": float64(2)}, nil, []string{"letter-spacing:2px;"}, nil},
		{"font-style", map[string]any{"fontStyle": "italic"}, nil, []string{"font-style:italic;"}, nil},
		{"text-decoration", map[string]any{"textDecoration": "underline"}, nil, []string{"text-decoration:underline;"}, nil},
		{"text-transform", map[string]any{"textTransform": "uppercase"}, nil, []string{"text-transform:uppercase;"}, nil},
		{"line-clamp", map[string]any{"lineClamp": float64(2)}, nil, []string{"-webkit-line-clamp:2;", "-webkit-box-orient:vertical"}, nil},
		{"text-align", map[string]any{"textAlign": "center"}, nil, []string{"text-align:center;", "justify-content:center;"}, nil},
		{"ellipsis-prop", nil, map[string]any{"ellipsis": true}, []string{"white-space:nowrap;", "text-overflow:ellipsis;"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: "text", ID: "t", Text: "x", Style: tc.style, Props: tc.props})
			for _, w := range tc.want {
				if !strings.Contains(res.HTML, w) {
					t.Errorf("style %v: html lacks %q:\n%s", tc.style, w, res.HTML)
				}
			}
			for _, w := range tc.wantNot {
				if strings.Contains(res.HTML, w) {
					t.Errorf("style %v: html should not contain %q:\n%s", tc.style, w, res.HTML)
				}
			}
		})
	}

	// The default font size is emitted only when fontSize is absent.
	t.Run("default-font-size", func(t *testing.T) {
		res := renderWidget(t, &model.Node{Type: "text", ID: "d", Text: "x"})
		if !strings.Contains(res.HTML, "font-size:15px;") {
			t.Errorf("text without fontSize should default to 15px:\n%s", res.HTML)
		}
	})
}

// TestContainerCSSLayout covers the containerCSS branches: grid, stack, wrap and
// the layout align/justify mapping (flexAlign) for containers.
func TestContainerCSSLayout(t *testing.T) {
	cases := []struct {
		name string
		node *model.Node
		want []string
	}{
		{"stack", &model.Node{Type: "stack", ID: "st", Children: textKids("x")}, []string{"position:relative;flex-direction:column;"}},
		{"absolute", &model.Node{Type: "absolute", ID: "ab", Children: textKids("x")}, []string{"position:relative;flex-direction:column;"}},
		{"wrap-prop", &model.Node{Type: "row", ID: "wrp", Props: map[string]any{"wrap": true}, Children: textKids("x")}, []string{"flex-wrap:wrap;"}},
		{"align-between", &model.Node{Type: "row", ID: "al", Layout: map[string]any{"align": "center", "justify": "between"}, Children: textKids("x")}, []string{"align-items:center;", "justify-content:space-between;"}},
		{"align-around", &model.Node{Type: "row", ID: "al2", Layout: map[string]any{"justify": "around"}, Children: textKids("x")}, []string{"justify-content:space-around;"}},
		{"align-evenly", &model.Node{Type: "row", ID: "al3", Layout: map[string]any{"justify": "evenly"}, Children: textKids("x")}, []string{"justify-content:space-evenly;"}},
		{"align-stretch", &model.Node{Type: "row", ID: "al4", Layout: map[string]any{"align": "stretch"}, Children: textKids("x")}, []string{"align-items:stretch;"}},
		{"align-end-keywords", &model.Node{Type: "row", ID: "al5", Layout: map[string]any{"align": "bottom", "justify": "right"}, Children: textKids("x")}, []string{"align-items:flex-end;", "justify-content:flex-end;"}},
		{"align-start-keywords", &model.Node{Type: "row", ID: "al6", Layout: map[string]any{"align": "top", "justify": "left"}, Children: textKids("x")}, []string{"align-items:flex-start;", "justify-content:flex-start;"}},
		{"align-baseline", &model.Node{Type: "row", ID: "al7", Layout: map[string]any{"align": "baseline"}, Children: textKids("x")}, []string{"align-items:baseline;"}},
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

// TestFlexAlign is a direct table test of the layout-keyword → CSS mapping,
// including the documented flex-start fallback for an unknown keyword.
func TestFlexAlign(t *testing.T) {
	cases := map[string]string{
		"center":   "center",
		"baseline": "baseline",
		"start":    "flex-start",
		"left":     "flex-start",
		"top":      "flex-start",
		"end":      "flex-end",
		"right":    "flex-end",
		"bottom":   "flex-end",
		"between":  "space-between",
		"around":   "space-around",
		"evenly":   "space-evenly",
		"stretch":  "stretch",
		"nonsense": "flex-start", // unknown keyword falls back
		"":         "flex-start",
	}
	for in, want := range cases {
		if got := flexAlign(in); got != want {
			t.Errorf("flexAlign(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolveStyleArrayBinding guards that resolveStyle/resolveStyleVal recurse
// into nested arrays (and odd, unused style shapes) without breaking the render
// — a malformed style value must never panic or leak a raw {{ }} binding.
func TestResolveStyleArrayBinding(t *testing.T) {
	res := renderWidgetState(t,
		&model.Node{Type: "text", ID: "rb", Text: "x", Style: map[string]any{
			"opacity": "{{ state.o }}",
			"weird":   []any{"{{ state.o }}", float64(1)},
			"nested":  map[string]any{"deep": []any{"{{ state.o }}"}},
		}},
		map[string]any{"o": float64(0.25)})
	if !strings.Contains(res.HTML, "opacity:0.25;") {
		t.Errorf("bound opacity should resolve:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "{{") {
		t.Errorf("unresolved binding leaked into output:\n%s", res.HTML)
	}
}

// ---- direct unit tests of the pure CSS/value helpers ----

func TestCssWriter(t *testing.T) {
	var b strings.Builder
	css(&b, "opacity", 0.5, ";")
	css(&b, "font-size", 20, "px;")
	if got := b.String(); got != "opacity:0.5;font-size:20px;" {
		t.Errorf("css writer = %q", got)
	}
}

func TestWriteSize(t *testing.T) {
	var b strings.Builder
	writeSize(&b, "width", nil, "fill") // nil skipped, then "fill"
	writeSize(&b, "height", float64(50))
	writeSize(&b, "width", "wrap") // no match -> nothing
	if got := b.String(); got != "width:100%;height:50px;" {
		t.Errorf("writeSize = %q", got)
	}
}

func TestWriteEdges(t *testing.T) {
	var b strings.Builder
	writeEdges(&b, "padding", float64(8))
	writeEdges(&b, "margin", map[string]any{"top": float64(1), "right": float64(2), "bottom": float64(3), "left": float64(4)})
	writeEdges(&b, "padding", "ignored") // wrong type -> nothing
	if got := b.String(); got != "padding:8px;margin:1px 2px 3px 4px;" {
		t.Errorf("writeEdges = %q", got)
	}
}

func TestAsFloat(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{float64(1.5), 1.5},
		{int(3), 3},
		{true, 1},
		{false, 0},
		{"2.5", 2.5},
		{"abc", 0},
		{nil, 0},
	}
	for _, tc := range cases {
		if got := asFloat(tc.in); got != tc.want {
			t.Errorf("asFloat(%v) = %g, want %g", tc.in, got, tc.want)
		}
	}
}

func TestAsBool(t *testing.T) {
	cases := []struct {
		in   any
		want bool
	}{
		{true, true},
		{false, false},
		{float64(1), true},
		{float64(0), false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"yes", false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := asBool(tc.in); got != tc.want {
			t.Errorf("asBool(%v) = %t, want %t", tc.in, got, tc.want)
		}
	}
}

func TestClampPct(t *testing.T) {
	if clampPct(-5) != 0 || clampPct(50) != 50 || clampPct(150) != 100 {
		t.Errorf("clampPct clamping wrong: %g %g %g", clampPct(-5), clampPct(50), clampPct(150))
	}
}

func TestOptionList(t *testing.T) {
	got := optionList([]any{
		"plain",
		map[string]any{"value": "v", "label": "L"},
		map[string]any{"value": "nolabel"},
		42, // unsupported element shape is skipped
	})
	want := []option{{"plain", "plain"}, {"v", "L"}, {"nolabel", "nolabel"}}
	if len(got) != len(want) {
		t.Fatalf("optionList len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("optionList[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if optionList("not-an-array") != nil {
		t.Errorf("optionList(non-array) should be nil")
	}
}

func TestStringList(t *testing.T) {
	got := stringList([]any{"a", 1, true})
	want := []string{"a", "1", "true"}
	if len(got) != len(want) {
		t.Fatalf("stringList len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stringList[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if stringList(123) != nil {
		t.Errorf("stringList(non-array) should be nil")
	}
}

func TestMergeArgs(t *testing.T) {
	got := mergeArgs(map[string]string{"a": "1", "b": "2"}, "a", "9")
	if got["a"] != "9" || got["b"] != "2" || len(got) != 2 {
		t.Errorf("mergeArgs = %v, want map[a:9 b:2]", got)
	}
}

func TestAlertColors(t *testing.T) {
	cases := []struct {
		variant string
		fg      string
	}{
		{"success", "var(--success)"},
		{"warning", "var(--warning)"},
		{"error", "var(--danger)"},
		{"danger", "var(--danger)"},
		{"info", "var(--accent)"},
		{"", "var(--accent)"},
		{"bogus", "var(--accent)"},
	}
	for _, tc := range cases {
		_, fg, icon := alertColors(tc.variant)
		if fg != tc.fg {
			t.Errorf("alertColors(%q) fg = %q, want %q", tc.variant, fg, tc.fg)
		}
		if !strings.Contains(icon, "<svg") {
			t.Errorf("alertColors(%q) icon should be an svg, got %q", tc.variant, icon)
		}
	}
}

func TestCheckboxCell(t *testing.T) {
	checked := checkboxCell(true)
	if !strings.Contains(checked, "background:var(--accent)") || !strings.Contains(checked, iconSVG("check", 11)) {
		t.Errorf("checked cell should be accent-filled with a check:\n%s", checked)
	}
	unchecked := checkboxCell(false)
	if !strings.Contains(unchecked, "border:1.5px solid var(--sep)") || strings.Contains(unchecked, iconSVG("check", 11)) {
		t.Errorf("unchecked cell should be an empty bordered box:\n%s", unchecked)
	}
}

func TestIconOrText(t *testing.T) {
	if svg := iconOrText("check", 16); !strings.Contains(svg, "<svg") {
		t.Errorf("known icon should resolve to svg, got %q", svg)
	}
	if got := iconOrText("hello", 16); got != "hello" {
		t.Errorf("unknown name should pass through, got %q", got)
	}
	if got := iconOrText("<b>", 16); got != "&lt;b&gt;" {
		t.Errorf("unknown name must be escaped, got %q", got)
	}
}

func TestBoundPath(t *testing.T) {
	cases := map[string]string{
		"{{ state.email }}":     "email",
		"{{state.a.b}}":         "a.b",
		"{{ state.x }} extra":   "", // not a pure binding
		"plain":                 "",
		"":                      "",
		"{{ state.user.name }}": "user.name",
	}
	for in, want := range cases {
		if got := boundPath(in); got != want {
			t.Errorf("boundPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseInvokeProp(t *testing.T) {
	n := &model.Node{Type: "x", Props: map[string]any{
		"good":   map[string]any{"name": "run", "args": map[string]any{"id": 7, "s": "v"}},
		"noname": map[string]any{"args": map[string]any{"id": 1}},
		"notmap": "just-a-string",
		"noargs": map[string]any{"name": "bare"},
	}}
	inv := parseInvokeProp(n, "good")
	if inv == nil || inv.Name != "run" || inv.Args["id"] != "7" || inv.Args["s"] != "v" {
		t.Errorf("parseInvokeProp(good) = %+v, want name=run args{id:7,s:v}", inv)
	}
	if parseInvokeProp(n, "missing") != nil {
		t.Errorf("absent prop should be nil")
	}
	if parseInvokeProp(n, "notmap") != nil {
		t.Errorf("non-map prop should be nil")
	}
	if parseInvokeProp(n, "noname") != nil {
		t.Errorf("map without name should be nil")
	}
	if inv := parseInvokeProp(n, "noargs"); inv == nil || inv.Name != "bare" || len(inv.Args) != 0 {
		t.Errorf("parseInvokeProp(noargs) = %+v, want name=bare empty args", inv)
	}
}

func TestTruthyStrHelpers(t *testing.T) {
	for _, s := range []string{"", "false", "0"} {
		if truthyStrCT(s) {
			t.Errorf("truthyStrCT(%q) should be false", s)
		}
		if truthyStrChip(s) {
			t.Errorf("truthyStrChip(%q) should be false", s)
		}
	}
	for _, s := range []string{"true", "1", "yes"} {
		if !truthyStrCT(s) || !truthyStrChip(s) {
			t.Errorf("truthyStr*(%q) should be true", s)
		}
	}
}

func TestNumOrDefault(t *testing.T) {
	m := map[string]any{"a": float64(5)}
	if numOrDefault(m, "a", 9) != 5 {
		t.Errorf("numOrDefault present = %g, want 5", numOrDefault(m, "a", 9))
	}
	if numOrDefault(m, "b", 9) != 9 {
		t.Errorf("numOrDefault missing should fall back to default")
	}
	if numOrDefault(nil, "a", 9) != 9 {
		t.Errorf("numOrDefault(nil map) should fall back to default")
	}
}

// ---- Pseudo-state style keys -------------------------------------------------

// TestPseudoStateVariables pins the emission contract for every pseudo-state
// key: the renderer's whole job is to publish a CSS custom property on the node
// (the shell's fixed :hover/:active/:focus-within/disabled rules consume it —
// see TestShellPseudoStateRules in internal/server), so the variable name and
// value are the API. A rename on either side breaks a state visual silently,
// which is exactly what these assertions prevent.
func TestPseudoStateVariables(t *testing.T) {
	cases := []struct {
		name  string
		style map[string]any
		want  []string
	}{
		{"hover-background", map[string]any{"hoverBackground": "var(--fill)"}, []string{"--qorm-hov-bg:var(--fill);"}},
		{"hover-color", map[string]any{"hoverColor": "#ff0000"}, []string{"--qorm-hov-fg:#ff0000;"}},
		{"hover-opacity", map[string]any{"hoverOpacity": float64(0.8)}, []string{"--qorm-hov-op:0.8;"}},
		{"pressed-scale", map[string]any{"pressedScale": float64(0.94)}, []string{"--qorm-prs-sc:0.94;"}},
		{"pressed-opacity", map[string]any{"pressedOpacity": float64(0.6)}, []string{"--qorm-prs-op:0.6;"}},
		{"focus-border-color", map[string]any{"focusBorderColor": "var(--accent)"}, []string{"--qorm-foc-bc:var(--accent);"}},
		{"disabled", map[string]any{"disabled": true}, []string{"--qorm-dis:1;", ` aria-disabled="true"`}},
		{"disabled-opacity", map[string]any{"disabled": true, "disabledOpacity": float64(0.25)}, []string{"--qorm-dop:0.25;", "--qorm-dis:1;"}},
		{"all-four-states", map[string]any{
			"hoverBackground": "#111", "pressedScale": float64(0.9),
			"focusBorderColor": "#222", "disabled": true,
		}, []string{"--qorm-hov-bg:#111;", "--qorm-prs-sc:0.9;", "--qorm-foc-bc:#222;", "--qorm-dis:1;"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := styleHTML(t, tc.style, nil)
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("style %v: html lacks %q:\n%s", tc.style, w, html)
				}
			}
		})
	}
}

// TestPseudoStateVariableNamesDoNotPrefix guards the shell's matching strategy:
// it selects on a SUBSTRING of the style attribute, so one variable name being
// a prefix of another would make an unrelated key trigger the wrong rule (the
// concrete trap: --qorm-dis vs a hypothetical --qorm-dis-op). Nothing but this
// test enforces the invariant, and it is invisible at the call site.
func TestPseudoStateVariableNamesDoNotPrefix(t *testing.T) {
	names := []string{
		varHoverBG, varHoverFG, varHoverOpacity,
		varPressScale, varPressOpacity, varFocusBorder,
		varDisabled, varDisabledOp,
	}
	for i, a := range names {
		for j, b := range names {
			if i != j && strings.HasPrefix(b, a) {
				t.Errorf("pseudo-state var %q is a prefix of %q: the shell's [style*=…] match would collide", a, b)
			}
		}
	}
}

// TestPseudoStateOmittedWhenUnset: a node without pseudo-state keys must render
// byte-identically to before the feature — no stray variables, no ARIA state.
func TestPseudoStateOmittedWhenUnset(t *testing.T) {
	html := styleHTML(t, map[string]any{"background": "red"}, nil)
	for _, v := range []string{"--qorm-hov-", "--qorm-prs-", "--qorm-foc-", "--qorm-dis", "--qorm-dop", "aria-disabled"} {
		if strings.Contains(html, v) {
			t.Errorf("a node with no pseudo-state keys must not emit %q:\n%s", v, html)
		}
	}
	// disabled:false is an explicit opt-out, not a marker.
	if h := styleHTML(t, map[string]any{"disabled": false}, nil); strings.Contains(h, "--qorm-dis") || strings.Contains(h, "aria-disabled") {
		t.Errorf("disabled:false must not mark the node:\n%s", h)
	}
}

// TestPseudoStateInjectionClosure: the pseudo-state values ride from author (or
// bound) input straight into the inline style attribute, the same position as
// `background`. A raw double quote there would TERMINATE the quoted attribute
// and let the value inject arbitrary attributes (the round-6 breakout class).
// Since the CSS-declaration-injection fix these values are validated by
// cssStyleValue at the colorStr choke point, so a payload carrying attribute or
// tag punctuation is DROPPED outright rather than merely entity-encoded — the
// variable is never emitted at all. styleAttr still encodes what survives.
func TestPseudoStateInjectionClosure(t *testing.T) {
	evil := `red" onmouseover="alert(1)`
	vars := map[string]string{
		"hoverBackground":  varHoverBG,
		"hoverColor":       varHoverFG,
		"focusBorderColor": varFocusBorder,
	}
	for _, key := range []string{"hoverBackground", "hoverColor", "focusBorderColor"} {
		t.Run(key, func(t *testing.T) {
			html := styleHTML(t, map[string]any{key: evil + "<script>"}, nil)
			if strings.Contains(html, `onmouseover="`) {
				t.Errorf("%s value broke out of the style attribute:\n%s", key, html)
			}
			if strings.Contains(html, "<script>") {
				t.Errorf("%s value emitted a raw tag:\n%s", key, html)
			}
			if strings.Contains(html, vars[key]) {
				t.Errorf("%s: a value that is not a CSS value must be dropped, not emitted:\n%s", key, html)
			}
		})
	}

	// The declaration-injection payload proper: no attribute breakout at all
	// (so entity encoding would have "passed"), just a second declaration that
	// turns the node into a full-screen click-jacking overlay with an outbound
	// beacon. It must not reach the style attribute in any form.
	t.Run("declaration-injection", func(t *testing.T) {
		const payload = `#fff;position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999;background-image:url(//attacker/beacon.png)`
		html := styleHTML(t, map[string]any{"hoverBackground": payload}, nil)
		for _, frag := range []string{"position:fixed", "z-index:99999", "background-image", "attacker"} {
			if strings.Contains(html, frag) {
				t.Errorf("declaration injection leaked %q into the style attribute:\n%s", frag, html)
			}
		}
	})
}

// TestPseudoStateKeysAreKnown: an unlisted key is a load-time warning and the
// renderer never sees it, so every key consumed above must be whitelisted.
func TestPseudoStateKeysAreKnown(t *testing.T) {
	for _, k := range []string{
		"hoverBackground", "hoverColor", "hoverOpacity",
		"pressedScale", "pressedOpacity", "focusBorderColor",
		"disabled", "disabledOpacity",
	} {
		if !KnownStyleKeys[k] {
			t.Errorf("KnownStyleKeys must include %q (loader whitelist)", k)
		}
	}
}

// ---- Semantic alias containers ----------------------------------------------

// TestAliasContainerDefaultAlignment: `center`/`start`/`end`/`between`/… used
// to fall through to a plain column, so `{"type":"center"}` centered nothing
// unless the author ALSO wrote layout.align — a name that lied. Each alias now
// carries its namesake alignment as the default.
func TestAliasContainerDefaultAlignment(t *testing.T) {
	cases := []struct {
		typ  string
		want []string
		not  []string
	}{
		{"center", []string{"align-items:center;", "justify-content:center;"}, nil},
		{"start", []string{"align-items:flex-start;", "justify-content:flex-start;"}, nil},
		{"end", []string{"align-items:flex-end;", "justify-content:flex-end;"}, nil},
		{"between", []string{"justify-content:space-between;"}, []string{"align-items:"}},
		{"around", []string{"justify-content:space-around;"}, []string{"align-items:"}},
		{"evenly", []string{"justify-content:space-evenly;"}, []string{"align-items:"}},
		{"stretch", []string{"align-items:stretch;"}, []string{"justify-content:"}},
	}
	for _, tc := range cases {
		t.Run(tc.typ, func(t *testing.T) {
			res := renderWidget(t, &model.Node{Type: tc.typ, ID: "al", Children: []*model.Node{
				{Type: "text", ID: "t", Text: "x"},
			}})
			seg := nodeStyleOf(t, res.HTML, "al")
			for _, w := range tc.want {
				if !strings.Contains(seg, w) {
					t.Errorf("type %q: style %q lacks %q", tc.typ, seg, w)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(seg, w) {
					t.Errorf("type %q: distribution/stretch alias should not set %q, got %q", tc.typ, w, seg)
				}
			}
		})
	}
	// A non-alias container keeps the old behaviour: no implicit alignment.
	res := renderWidget(t, &model.Node{Type: "column", ID: "al"})
	if seg := nodeStyleOf(t, res.HTML, "al"); strings.Contains(seg, "align-items:") || strings.Contains(seg, "justify-content:") {
		t.Errorf("a plain column must not gain implicit alignment, got %q", seg)
	}
}

// TestAliasContainerExplicitLayoutWins: the alias alignment is a DEFAULT. An
// explicit layout.align / layout.justify replaces it — and replaces it rather
// than being appended after it, so exactly one declaration is emitted (a
// duplicated property would be a silent cascade puzzle for the author).
func TestAliasContainerExplicitLayoutWins(t *testing.T) {
	res := renderWidget(t, &model.Node{
		Type: "center", ID: "al",
		Layout: map[string]any{"align": "start", "justify": "between"},
	})
	seg := nodeStyleOf(t, res.HTML, "al")
	for _, w := range []string{"align-items:flex-start;", "justify-content:space-between;"} {
		if !strings.Contains(seg, w) {
			t.Errorf("explicit layout should win: style %q lacks %q", seg, w)
		}
	}
	if strings.Count(seg, "align-items:") != 1 || strings.Count(seg, "justify-content:") != 1 {
		t.Errorf("alias default must be replaced, not appended: %q", seg)
	}
	// Overriding one axis leaves the alias default on the other.
	res = renderWidget(t, &model.Node{Type: "center", ID: "al", Layout: map[string]any{"justify": "end"}})
	seg = nodeStyleOf(t, res.HTML, "al")
	if !strings.Contains(seg, "align-items:center;") || !strings.Contains(seg, "justify-content:flex-end;") {
		t.Errorf("partial override should keep the other axis default: %q", seg)
	}
}

// nodeStyleOf extracts the style attribute of the element carrying the given id.
func nodeStyleOf(t *testing.T, html, id string) string {
	t.Helper()
	i := strings.Index(html, `id="`+id+`"`)
	if i < 0 {
		t.Fatalf("no element with id %q in:\n%s", id, html)
	}
	rest := html[i:]
	j := strings.Index(rest, `style="`)
	if j < 0 {
		t.Fatalf("element %q has no style attribute in:\n%s", id, html)
	}
	rest = rest[j+len(`style="`):]
	k := strings.Index(rest, `"`)
	if k < 0 {
		t.Fatalf("unterminated style attribute for %q", id)
	}
	return rest[:k]
}

// TestCSSURLToken covers the one place the renderer builds a url() ON PURPOSE
// (the image `placeholder`). The character that matters inside an unquoted
// url-token is ")": the CSS tokenizer consumes everything up to it, so a ";" or
// a ":" is inert there — which is why a data: URI keeps working — but a value
// carrying its own ")" closes the token early and the rest is parsed as fresh
// declarations, the full-screen-overlay payload again.
func TestCSSURLToken(t *testing.T) {
	for _, ok := range []string{
		"blur.png", "/static/thumbs/a_1.jpg?v=2", "//cdn.example.com/x.webp",
		"data:image/png;base64,iVBORw0KGgo=", "img/a%20b.png", "a.png#frag",
	} {
		if got := cssURLToken(ok); got != ok {
			t.Errorf("legitimate url payload %q must pass through, got %q", ok, got)
		}
	}
	for _, bad := range []string{
		"", // nothing to paint
		"a.png);position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:99999;background:url(b.png",
		`a.png" onload="alert(1)`, "a.png'", "a b.png", "a(b).png", `a\.png`,
		"a.png<script>", "a/*.png",
	} {
		if got := cssURLToken(bad); got != "" {
			t.Errorf("cssURLToken(%q) = %q, want rejected", bad, got)
		}
	}
}

// TestCSSRawDecls pins the ONE deliberate raw-CSS passthrough: `menuStyle` is a
// declaration list by contract, so ";" and ":" are its grammar and it cannot be
// value-filtered without breaking the feature. It is safe because propStr never
// evaluates a binding, so no state / http / MCP-set-state value can reach it —
// but the rule that reaches OFF the page still applies.
func TestCSSRawDecls(t *testing.T) {
	for _, ok := range []string{
		"background:hotpink;", "min-width:320px;padding:12px;",
		"background:var(--surface);border:1px solid var(--sep);",
		// a declaration list is allowed to reposition the panel: that is the
		// documented purpose, and the author already controls the whole scene
		"position:absolute;top:0;left:0;",
	} {
		if got := cssRawDecls(ok); got != ok {
			t.Errorf("legitimate menuStyle %q must pass through, got %q", ok, got)
		}
	}
	for _, bad := range []string{
		"background:url(//attacker/beacon.png);",
		"background:IMAGE-SET(//attacker/x.png 1x);",
		"padding:6px;/*",
	} {
		if got := cssRawDecls(bad); got != "" {
			t.Errorf("cssRawDecls(%q) = %q, want rejected", bad, got)
		}
	}
}

// TestCSSFetchOrComment is the shared rule under cssValue, cssStyleValue and
// cssRawDecls: whatever the syntax around it, no CSS this renderer emits may
// make the browser fetch a third-party resource or open a comment. Case is
// irrelevant, and the surrounding allowlists reject "\", so a CSS escape cannot
// spell any of these past the charset gate.
func TestCSSFetchOrComment(t *testing.T) {
	for _, bad := range []string{
		"url(x)", "URL(x)", "Url(x)", "image-set(x)", "-webkit-image-set(x)",
		"src(x)", "expression(alert(1))", "a/*b",
	} {
		if !cssFetchOrComment(bad) {
			t.Errorf("cssFetchOrComment(%q) = false, want true", bad)
		}
	}
	for _, ok := range []string{
		"var(--accent)", "rgb(0 0 0 / 50%)", "color-mix(in srgb,red 20%,blue)",
		"linear-gradient(135deg,#007aff,#5e5ce6)", "cubic-bezier(.34,1.2,.64,1)",
		"all .2s ease", "curl-ish", "resource(x)",
	} {
		if cssFetchOrComment(ok) {
			t.Errorf("cssFetchOrComment(%q) = true, want false", ok)
		}
	}
}

// TestHTMLQSSClassRule proves a styles/*.qss class rule reaches the HTML
// render path: a node with only `class` (no inline style) emits the rule's
// CSS declarations.
func TestHTMLQSSClassRule(t *testing.T) {
	node := &model.Node{
		Type: "text", ID: "t", Text: "hi",
		Props: map[string]any{"class": "accent"},
	}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{node}}
	app := &model.App{
		Entry:  "main",
		Scenes: map[string]*model.Node{"main": root},
		Styles: []model.StyleRule{
			{Kind: model.StyleRuleClass, Name: "accent", Style: map[string]any{
				"background": "#007AFF",
				"fontSize":   float64(22),
				"color":      "var(--on-accent)",
			}},
		},
	}
	res := Render(qrt.New(app))
	for _, w := range []string{"background:#007AFF;", "font-size:22px;", "color:var(--on-accent);"} {
		if !strings.Contains(res.HTML, w) {
			t.Errorf("class rule should emit %q:\n%s", w, res.HTML)
		}
	}
}

// TestHTMLQSSCascadePriority: type < class (prop order) < id < inline.
func TestHTMLQSSCascadePriority(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleType, Name: "text", Style: map[string]any{"fontSize": float64(20)}},
		{Kind: model.StyleRuleClass, Name: "big", Style: map[string]any{"fontSize": float64(22)}},
		{Kind: model.StyleRuleID, Name: "hero", Style: map[string]any{"fontSize": float64(24)}},
	}
	mk := func(class, id string, inline map[string]any) *model.Node {
		n := &model.Node{Type: "text", ID: id, Text: "x", Style: inline, Props: map[string]any{}}
		if class != "" {
			n.Props["class"] = class
		}
		if id == "" {
			n.ID = "n"
		}
		return n
	}
	cases := []struct {
		name string
		node *model.Node
		want string
	}{
		{"type only", mk("other", "", nil), "font-size:20px;"},
		{"class beats type", mk("big", "", nil), "font-size:22px;"},
		{"id beats class", mk("big", "hero", nil), "font-size:24px;"},
		{"inline beats id", mk("big", "hero", map[string]any{"fontSize": float64(26)}), "font-size:26px;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{tc.node}}
			app := &model.App{
				Entry:  "main",
				Scenes: map[string]*model.Node{"main": root},
				Styles: rules,
			}
			res := Render(qrt.New(app))
			if !strings.Contains(res.HTML, tc.want) {
				t.Fatalf("html lacks %q:\n%s", tc.want, res.HTML)
			}
		})
	}
}

// TestHTMLQSSClassOrder: later class name in the prop wins; later declaration
// of the same class wins.
func TestHTMLQSSClassOrder(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "a", Style: map[string]any{"fontSize": float64(10), "fontWeight": float64(400)}},
		{Kind: model.StyleRuleClass, Name: "a", Style: map[string]any{"fontWeight": float64(700)}},
		{Kind: model.StyleRuleClass, Name: "b", Style: map[string]any{"fontSize": float64(20)}},
	}
	render := func(class string) string {
		n := &model.Node{Type: "text", ID: "t", Text: "x", Props: map[string]any{"class": class}}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: rules}
		return Render(qrt.New(app)).HTML
	}
	html := render("a b")
	if !strings.Contains(html, "font-size:20px;") {
		t.Fatalf("class b later in prop should win fontSize:\n%s", html)
	}
	if !strings.Contains(html, "font-weight:700;") {
		t.Fatalf("later .a declaration should win fontWeight:\n%s", html)
	}
	html2 := render("b a")
	if !strings.Contains(html2, "font-size:10px;") {
		t.Fatalf("class a later in prop should win fontSize:\n%s", html2)
	}
}

// TestHTMLQSSBindingEvaluates: a {{binding}} in a rule body tracks live state.
func TestHTMLQSSBindingEvaluates(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "dyn", Style: map[string]any{"fontSize": "{{ state.fs }}"}},
	}
	n := &model.Node{Type: "text", ID: "t", Text: "x", Props: map[string]any{"class": "dyn"}}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{
		Entry:       "main",
		Scenes:      map[string]*model.Node{"main": root},
		Styles:      rules,
		GlobalState: model.GlobalState{Initial: map[string]any{"fs": float64(17)}},
	}
	rt := qrt.New(app)
	res := Render(rt)
	if !strings.Contains(res.HTML, "font-size:17px;") {
		t.Fatalf("bound rule should emit 17px:\n%s", res.HTML)
	}
	rt.State["fs"] = float64(23)
	res2 := Render(rt)
	if !strings.Contains(res2.HTML, "font-size:23px;") {
		t.Fatalf("bound rule should track state to 23px:\n%s", res2.HTML)
	}
}

// TestHTMLQSSZeroMatchNoOp: non-matching rules must not leak onto a node.
func TestHTMLQSSZeroMatchNoOp(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleType, Name: "button", Style: map[string]any{"background": "red"}},
		{Kind: model.StyleRuleClass, Name: "other", Style: map[string]any{"background": "blue"}},
		{Kind: model.StyleRuleID, Name: "other", Style: map[string]any{"background": "green"}},
	}
	n := &model.Node{Type: "text", ID: "x", Text: "plain"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: rules}
	res := Render(qrt.New(app))
	for _, bad := range []string{"background:red;", "background:blue;", "background:green;"} {
		if strings.Contains(res.HTML, bad) {
			t.Errorf("non-matching rule leaked %q:\n%s", bad, res.HTML)
		}
	}
}

// TestHTMLQSSResidualDisabledAndSpacer: paths that used to read raw n.Style
// (a11y aria-disabled, spacer size) must honor QSS class rules the same way
// boxCSS/textCSS do via effectiveStyle.
func TestHTMLQSSResidualDisabledAndSpacer(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "off", Style: map[string]any{"disabled": true}},
		{Kind: model.StyleRuleClass, Name: "gap", Style: map[string]any{"size": float64(24)}},
		{Kind: model.StyleRuleType, Name: "chart", Style: map[string]any{"width": float64(100), "height": float64(40)}},
	}
	t.Run("aria-disabled from class", func(t *testing.T) {
		n := &model.Node{Type: "text", ID: "t", Text: "x", Props: map[string]any{"class": "off"}}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: rules}
		html := Render(qrt.New(app)).HTML
		if !strings.Contains(html, `aria-disabled="true"`) {
			t.Fatalf("QSS disabled should set aria-disabled:\n%s", html)
		}
		if !strings.Contains(html, "--qorm-dis:1;") {
			t.Fatalf("QSS disabled should still set visual marker:\n%s", html)
		}
	})
	t.Run("bound disabled resolves for aria", func(t *testing.T) {
		n := &model.Node{Type: "text", ID: "t", Text: "x", Style: map[string]any{"disabled": "{{ state.off }}"}}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
		app := &model.App{
			Entry: "main", Scenes: map[string]*model.Node{"main": root},
			GlobalState: model.GlobalState{Initial: map[string]any{"off": true}},
		}
		html := Render(qrt.New(app)).HTML
		if !strings.Contains(html, `aria-disabled="true"`) {
			t.Fatalf("bound disabled should set aria-disabled:\n%s", html)
		}
	})
	t.Run("spacer size from class", func(t *testing.T) {
		n := &model.Node{Type: "spacer", ID: "sp", Props: map[string]any{"class": "gap"}}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: rules}
		html := Render(qrt.New(app)).HTML
		if !strings.Contains(html, "width:24px;height:24px;") {
			t.Fatalf("QSS size should size the spacer:\n%s", html)
		}
	})
	t.Run("chart dims from type rule", func(t *testing.T) {
		n := &model.Node{Type: "chart", ID: "c", Props: map[string]any{"data": []any{float64(1), float64(2)}}}
		root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{n}}
		app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}, Styles: rules}
		html := Render(qrt.New(app)).HTML
		if !strings.Contains(html, `width="100"`) || !strings.Contains(html, `height="40"`) {
			t.Fatalf("QSS chart width/height should set SVG attrs:\n%s", html)
		}
	})
}

func TestResponsiveBreakpointStyles(t *testing.T) {
	style := map[string]any{
		"color": map[string]any{
			"sm": "#ff0000",
			"lg": "#00ff00",
		},
		"padding": map[string]any{
			"sm": "8px",
			"lg": "24px",
		},
	}
	html := styleHTML(t, style, nil)
	if !strings.Contains(html, "color:#ff0000;") {
		t.Errorf("responsive color style should resolve sm breakpoint fallback: got %q", html)
	}
}

func TestRenderSubtree(t *testing.T) {
	child := &model.Node{Type: "text", ID: "target_child", Text: "Subtree Content"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{child}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := qrt.New(app)

	res := RenderSubtree(rt, "target_child")
	if !strings.Contains(res.HTML, "Subtree Content") {
		t.Errorf("RenderSubtree should render targeted subtree: got %q", res.HTML)
	}

	missing := RenderSubtree(rt, "nonexistent_node")
	if !strings.Contains(missing.HTML, "node not found") {
		t.Errorf("RenderSubtree for missing node should report node not found: got %q", missing.HTML)
	}
}

func TestContainerQueriesDSL(t *testing.T) {
	style := map[string]any{
		"container": true,
		"padding": map[string]any{
			"cq-sm": "8px",
			"cq-lg": "24px",
		},
	}
	html := styleHTML(t, style, nil)
	if !strings.Contains(html, "container-type:inline-size;") {
		t.Errorf("container: true style should emit container-type:inline-size; got %q", html)
	}
	if !strings.Contains(html, "padding:8px;") {
		t.Errorf("container query padding cq-sm fallback should emit padding:8px; got %q", html)
	}
}

func TestDatatableWidget(t *testing.T) {
	cols := []any{
		map[string]any{"key": "id", "title": "ID"},
		map[string]any{"key": "name", "title": "Name"},
	}
	data := []any{
		map[string]any{"id": "1", "name": "Alice"},
		map[string]any{"id": "2", "name": "Bob"},
	}
	n := &model.Node{
		Type: "datatable",
		ID:   "my_table",
		Props: map[string]any{
			"columns":      cols,
			"data":         data,
			"stickyHeader": true,
			"virtual":      true,
		},
	}
	res := renderWidget(t, n)
	if !strings.Contains(res.HTML, "<table") || !strings.Contains(res.HTML, "Alice") || !strings.Contains(res.HTML, "position:sticky;") {
		t.Errorf("datatable widget should render table with data and sticky header: got %q", res.HTML)
	}
}

func TestRenderNodeDiff(t *testing.T) {
	child := &model.Node{Type: "text", ID: "target_child", Text: "Subtree Content"}
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{child}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := qrt.New(app)

	diffRes := RenderNodeDiff(rt, "target_child")
	if !strings.Contains(diffRes.HTML, `data-morph-target="target_child"`) {
		t.Errorf("RenderNodeDiff should wrap target node in morph template: got %q", diffRes.HTML)
	}
}
