package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
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
