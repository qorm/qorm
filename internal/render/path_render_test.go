package render

import (
	"strings"
	"testing"

	"github.com/qorm/platform/internal/model"
)

// TestPathRenderMarkup renders the demo path node (the canvas-advanced scene
// uses exactly this shape: bound `d`, transparent background, green stroke)
// and asserts the widget emits the inline <svg><path> with the stroke applied
// and no fill — a regression to the "unknown" container would drop the svg.
func TestPathRenderMarkup(t *testing.T) {
	n := &model.Node{
		Type: "path",
		ID:   "morph_path",
		Props: map[string]any{
			"d": "{{ state.morphed ? 'M 50 150 Q 100 50 150 150 T 250 150' : 'M 50 100 Q 100 200 150 100 T 250 100' }}",
		},
		Style: map[string]any{
			"x":           50,
			"y":           200,
			"width":       300,
			"height":      300,
			"background":  "transparent",
			"strokeColor": "#00ff7f",
			"strokeWidth": 6,
		},
	}
	res := renderWidgetState(t, n, map[string]any{"morphed": false})
	html := res.HTML
	// The svg + path leaf exists; the false-branch d ended up in the markup.
	for _, want := range []string{`data-qorm-path="1"`, "<svg", "<path d=\"M 50 100", `stroke="#00ff7f"`, `stroke-width="6"`, "T 250 100", `fill="none"`, `viewBox="0 0 300 300"`} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML lacks %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "data-qorm-unknown") {
		t.Errorf("path rendered as unknown node:\n%s", html)
	}
	// The morphed state flips the emitted d — the binding is re-evaluated per
	// render (no morph interpolation; the swap snaps).
	res2 := renderWidgetState(t, n, map[string]any{"morphed": true})
	if !strings.Contains(res2.HTML, `<path d="M 50 150`) {
		t.Errorf("morphed HTML should carry the true-branch d:\n%s", res2.HTML)
	}
}

// TestPathRenderFillAndRelative exercises the fill (background) and lowercase
// relative commands so the snapshot pins the full SVG-subset surface the
// parser accepts (H/V/Q/T/S relative forms).
func TestPathRenderFillAndRelative(t *testing.T) {
	n := &model.Node{
		Type: "path",
		ID:   "rel",
		Props: map[string]any{
			"d": "m 10 10 h 20 v 20 q 10 10 20 0 t 10 -5 s 5 10 10 0 z",
		},
		Style: map[string]any{
			"width":       60,
			"height":      60,
			"background":  "#7f00ff",
			"strokeColor": "#ff007f",
			"strokeWidth": 2,
		},
	}
	res := renderWidget(t, n)
	for _, want := range []string{`fill="#7f00ff"`, `stroke="#ff007f"`, `stroke-width="2"`, `d="m 10 10 h 20 v 20 q`, "z\""} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("HTML lacks %q:\n%s", want, res.HTML)
		}
	}
}
