package canvas

import (
	"image/color"
	"path/filepath"
	"testing"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
	"github.com/qorm/qorm/internal/theme"
)

// styleRuntime builds a runtime carrying the given stylesheet rules (plus an
// optional theme), so parseStyle's cascade can be exercised directly.
func styleRuntime(rules []model.StyleRule, th *theme.Theme, state map[string]any) *runtime.Runtime {
	if state == nil {
		state = map[string]any{}
	}
	return &runtime.Runtime{
		App:   &model.App{Styles: rules},
		Theme: th,
		State: state,
	}
}

func intp(v int) *int { return &v }

// The cascade, low to high: theme component default → type rule → class rule
// (declaration order) → id rule → inline style.
func TestStyleRuleCascadePriority(t *testing.T) {
	th := &theme.Theme{Components: map[string]theme.ComponentStyles{
		"text": {FontSize: intp(11)},
		"box":  {FontSize: intp(9)},
	}}
	rules := []model.StyleRule{
		{Kind: model.StyleRuleType, Name: "text", Style: map[string]any{"fontSize": float64(20)}},
		{Kind: model.StyleRuleClass, Name: "big", Style: map[string]any{"fontSize": float64(22)}},
		{Kind: model.StyleRuleID, Name: "hero", Style: map[string]any{"fontSize": float64(24)}},
	}
	rt := styleRuntime(rules, th, nil)

	mk := func(class, id string, inline map[string]any) *model.Node {
		n := &model.Node{Type: "text", ID: id, Style: inline, Props: map[string]any{}}
		if class != "" {
			n.Props["class"] = class
		}
		return n
	}
	themeOnly := &model.Node{Type: "box", Props: map[string]any{}} // no rule matches "box"
	cases := []struct {
		name string
		node *model.Node
		want int
	}{
		{"theme only", themeOnly, 9},
		{"theme under type", mk("other", "", nil), 20},
		{"class beats type", mk("big", "", nil), 22},
		{"id beats class", mk("big", "hero", nil), 24},
		{"inline beats id", mk("big", "hero", map[string]any{"fontSize": float64(26)}), 26},
		{"id without class", mk("", "hero", nil), 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseStyle(tc.node, rt).FontSize; got != tc.want {
				t.Fatalf("FontSize = %d, want %d", got, tc.want)
			}
		})
	}
}

// Class rules apply in the node's own `class` list order (a class named later
// wins); within one class name, declaration order wins.
func TestStyleRuleClassOrder(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "a", Style: map[string]any{"fontSize": float64(10), "fontWeight": float64(400)}},
		{Kind: model.StyleRuleClass, Name: "a", Style: map[string]any{"fontWeight": float64(700)}},
		{Kind: model.StyleRuleClass, Name: "b", Style: map[string]any{"fontSize": float64(20)}},
	}
	rt := styleRuntime(rules, nil, nil)
	node := &model.Node{Type: "text", Props: map[string]any{"class": "a b"}}
	st := parseStyle(node, rt)
	if st.FontSize != 20 {
		t.Fatalf("FontSize = %d, want 20 (class b is named later in the prop)", st.FontSize)
	}
	if st.FontWeight != 700 {
		t.Fatalf("FontWeight = %d, want 700 (later declaration of .a wins)", st.FontWeight)
	}
	// Reversing the prop order reverses the winner.
	node2 := &model.Node{Type: "text", Props: map[string]any{"class": "b a"}}
	if st := parseStyle(node2, rt); st.FontSize != 10 {
		t.Fatalf("FontSize = %d, want 10 with class list \"b a\"", st.FontSize)
	}
}

// A {{binding}} in a rule body evaluates at the same moment an inline style
// value would — against live state.
func TestStyleRuleBindingEvaluates(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleClass, Name: "dyn", Style: map[string]any{"fontSize": "{{ state.fs }}"}},
	}
	rt := styleRuntime(rules, nil, map[string]any{"fs": float64(17)})
	node := &model.Node{Type: "text", Props: map[string]any{"class": "dyn"}}
	if st := parseStyle(node, rt); st.FontSize != 17 {
		t.Fatalf("FontSize = %d, want 17 from the bound rule value", st.FontSize)
	}
	rt.State["fs"] = float64(23)
	if st := parseStyle(node, rt); st.FontSize != 23 {
		t.Fatalf("FontSize = %d after the state change, want 23", st.FontSize)
	}
}

// No stylesheets (or no match) is exactly the old behavior — and a rule for
// another type/class/id must not leak onto a node.
func TestStyleRuleZeroMatchNoOp(t *testing.T) {
	plain := &model.Node{Type: "text", ID: "x", Props: map[string]any{}}
	if got := matchingStyleRules(plain, styleRuntime(nil, nil, nil)); got != nil {
		t.Fatalf("no stylesheets: matched %v, want nil", got)
	}
	rules := []model.StyleRule{
		{Kind: model.StyleRuleType, Name: "button", Style: map[string]any{"fontSize": float64(1)}},
		{Kind: model.StyleRuleClass, Name: "other", Style: map[string]any{"fontSize": float64(2)}},
		{Kind: model.StyleRuleID, Name: "other", Style: map[string]any{"fontSize": float64(3)}},
	}
	rt := styleRuntime(rules, nil, nil)
	if got := matchingStyleRules(plain, rt); got != nil {
		t.Fatalf("non-matching rules leaked: %v", got)
	}
	if st := parseStyle(plain, rt); st.FontSize != 0 {
		t.Fatalf("FontSize = %d, want 0 (no rule matched)", st.FontSize)
	}
}

// Colors flow through the same resolveColor path as inline styles.
func TestStyleRuleColors(t *testing.T) {
	rules := []model.StyleRule{
		{Kind: model.StyleRuleType, Name: "button", Style: map[string]any{"background": "#007AFF", "color": "var(--text)"}},
	}
	rt := styleRuntime(rules, nil, nil)
	st := parseStyle(&model.Node{Type: "button", Props: map[string]any{}}, rt)
	if st.Background != (color.RGBA{0, 122, 255, 255}) {
		t.Fatalf("Background = %v, want #007AFF", st.Background)
	}
	if st.Color != (color.RGBA{29, 29, 31, 255}) {
		t.Fatalf("Color = %v, want var(--text) resolved", st.Color)
	}
}

// End to end: examples/tetris keeps its styles in styles/app.qss — the scene
// nodes carry only `class` — and the cascade resolves them to exactly the
// values the inline styles held before the migration.
func TestTetrisStylesheetMatchesPreMigrationStyles(t *testing.T) {
	app, err := loader.LoadDir(filepath.Join("..", "..", "..", "examples", "tetris"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range app.Diagnostics {
		t.Fatalf("loader diagnostic for examples/tetris: %s", d)
	}
	if len(app.Stylesheets) != 1 || app.Stylesheets[0].ID != "app" {
		t.Fatalf("tetris Stylesheets = %+v, want the one styles/app.qss sheet", app.Stylesheets)
	}
	root := app.Scenes["main"]
	if root == nil {
		t.Fatal("tetris has no main scene")
	}
	rt := &runtime.Runtime{App: app, State: map[string]any{}}

	primary := color.RGBA{0, 122, 255, 255}     // var(--primary)
	secondary := color.RGBA{134, 134, 139, 255} // var(--textSecondary)
	find := func(text string) *model.Node {
		var hit *model.Node
		var walk func(n *model.Node)
		walk = func(n *model.Node) {
			if n == nil || hit != nil {
				return
			}
			if n.Text == text {
				hit = n
				return
			}
			for _, c := range n.Children {
				walk(c)
			}
		}
		walk(root)
		if hit == nil {
			t.Fatalf("no node with text %q", text)
		}
		return hit
	}
	cases := []struct {
		text                 string
		fontSize, fontWeight int
		color                color.RGBA
	}{
		{"NEXT", 11, 700, secondary},
		{"SCORE", 11, 700, secondary},
		{"PAUSED", 13, 700, primary},
		{"P: resume", 10, 0, secondary},
		{"Left/Right/Down: move   Up/X: rotate   Z: rotate ccw", 12, 0, secondary},
	}
	for _, tc := range cases {
		n := find(tc.text)
		st := parseStyle(n, rt)
		if st.FontSize != tc.fontSize {
			t.Errorf("%q: FontSize = %d, want %d", tc.text, st.FontSize, tc.fontSize)
		}
		if st.FontWeight != tc.fontWeight {
			t.Errorf("%q: FontWeight = %d, want %d", tc.text, st.FontWeight, tc.fontWeight)
		}
		if st.Color != tc.color {
			t.Errorf("%q: Color = %v, want %v", tc.text, st.Color, tc.color)
		}
	}
	// The stat values bind their text; find one by class instead.
	var statValue *model.Node
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil || statValue != nil {
			return
		}
		if c, _ := n.Props["class"].(string); c == "statValue" {
			statValue = n
			return
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(root)
	if statValue == nil {
		t.Fatal("no statValue node in tetris scene")
	}
	st := parseStyle(statValue, rt)
	if st.FontSize != 16 || st.FontWeight != 700 || st.Color != primary {
		t.Fatalf("statValue resolved to %+v, want fontSize 16 / weight 700 / primary", st)
	}
	// The overlay panel keeps its inline padding/margin objects while the
	// class carries background/radius/gap.
	var panel *model.Node
	var walk2 func(n *model.Node)
	walk2 = func(n *model.Node) {
		if n == nil || panel != nil {
			return
		}
		if c, _ := n.Props["class"].(string); c == "overlayPanel" {
			panel = n
			return
		}
		for _, ch := range n.Children {
			walk2(ch)
		}
	}
	walk2(root)
	if panel == nil {
		t.Fatal("no overlayPanel node in tetris scene")
	}
	ps := parseStyle(panel, rt)
	if ps.Gap != 2 || ps.BorderRadius != 8 || ps.Padding != 0 || ps.MarginTop != 130 {
		t.Fatalf("overlayPanel resolved to gap %d radius %v padding %d marginTop %d, want 2/8/0/130 (padding is a per-side inline object)", ps.Gap, ps.BorderRadius, ps.Padding, ps.MarginTop)
	}
}
