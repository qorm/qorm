package render

import (
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/model"
	"github.com/qorm/qorm/internal/runtime"
)

func whenApp() *model.App {
	root := &model.Node{
		Type: "column", ID: "root",
		Children: []*model.Node{
			{Type: "text", ID: "vp", Text: "vp: {{ viewport.width }}x{{ viewport.height }} {{ viewport.orientation }}"},
			{
				Type: "when", ID: "sw",
				Condition: "{{ viewport.width >= 768 }}",
				Then:      &model.Node{Type: "row", ID: "wide", Children: []*model.Node{{Type: "text", ID: "wt", Text: "WIDE"}}},
				Else:      &model.Node{Type: "column", ID: "narrow", Children: []*model.Node{{Type: "text", ID: "nt", Text: "NARROW"}}},
			},
		},
	}
	return &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
}

// TestWhenBranchesByViewport verifies a `when` node renders its then branch at
// a wide viewport, its else branch at a narrow one, and — the documented
// server-first-frame semantics — the else branch while the viewport is unknown.
func TestWhenBranchesByViewport(t *testing.T) {
	cases := []struct {
		name    string
		vp      runtime.Viewport
		want    string
		wantNot string
	}{
		{"wide-1440", runtime.Viewport{W: 1440, H: 900}, "WIDE", "NARROW"},
		{"narrow-375", runtime.Viewport{W: 375, H: 667}, "NARROW", "WIDE"},
		{"exact-breakpoint", runtime.Viewport{W: 768, H: 1024}, "WIDE", "NARROW"},
		{"unknown-viewport-falsy", runtime.Viewport{}, "NARROW", "WIDE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := runtime.New(whenApp())
			rt.Viewport = tc.vp
			html := Render(rt).HTML
			if !strings.Contains(html, tc.want) {
				t.Errorf("viewport %dx%d: html lacks %q:\n%s", tc.vp.W, tc.vp.H, tc.want, html)
			}
			if strings.Contains(html, tc.wantNot) {
				t.Errorf("viewport %dx%d: html should not contain %q (both branches rendered?)", tc.vp.W, tc.vp.H, tc.wantNot)
			}
		})
	}
}

// TestWhenViewportVarsInterpolate checks viewport.width/height/orientation are
// available to ordinary bindings, and the orientation rule (W>=H landscape,
// W<H portrait, "" while unknown).
func TestWhenViewportVarsInterpolate(t *testing.T) {
	rt := runtime.New(whenApp())
	rt.Viewport = runtime.Viewport{W: 1440, H: 900}
	if html := Render(rt).HTML; !strings.Contains(html, "vp: 1440x900 landscape") {
		t.Errorf("landscape interpolation wrong:\n%s", html)
	}
	rt.Viewport = runtime.Viewport{W: 375, H: 667}
	if html := Render(rt).HTML; !strings.Contains(html, "vp: 375x667 portrait") {
		t.Errorf("portrait interpolation wrong:\n%s", html)
	}
	rt.Viewport = runtime.Viewport{}
	if html := Render(rt).HTML; !strings.Contains(html, "vp: 0x0 ") {
		t.Errorf("unknown viewport should read 0x0 with empty orientation:\n%s", html)
	}
}

// TestWhenOrientationCondition drives a when node off viewport.orientation.
func TestWhenOrientationCondition(t *testing.T) {
	root := &model.Node{
		Type: "when", ID: "o",
		Condition: "{{ viewport.orientation == 'portrait' }}",
		Then:      &model.Node{Type: "text", ID: "p", Text: "PORTRAIT"},
		Else:      &model.Node{Type: "text", ID: "l", Text: "LANDSCAPE"},
	}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app)
	rt.Viewport = runtime.Viewport{W: 375, H: 667}
	if html := Render(rt).HTML; !strings.Contains(html, "PORTRAIT") {
		t.Errorf("375x667 should be portrait:\n%s", html)
	}
	rt.Viewport = runtime.Viewport{W: 1024, H: 768}
	if html := Render(rt).HTML; !strings.Contains(html, "LANDSCAPE") {
		t.Errorf("1024x768 should be landscape:\n%s", html)
	}
}

// TestWhenMissingBranches: a when with no matching branch renders nothing (and
// never panics); a missing condition is falsy and renders else.
func TestWhenMissingBranches(t *testing.T) {
	root := &model.Node{Type: "column", ID: "root", Children: []*model.Node{
		{Type: "when", ID: "no-else", Condition: "{{ viewport.width >= 768 }}",
			Then: &model.Node{Type: "text", ID: "t", Text: "ONLY-THEN"}},
		{Type: "when", ID: "no-cond",
			Then: &model.Node{Type: "text", ID: "t2", Text: "CONDLESS-THEN"},
			Else: &model.Node{Type: "text", ID: "e2", Text: "CONDLESS-ELSE"}},
		{Type: "when", ID: "bare"},
	}}
	app := &model.App{Entry: "main", Scenes: map[string]*model.Node{"main": root}}
	rt := runtime.New(app) // unknown viewport
	html := Render(rt).HTML
	if strings.Contains(html, "ONLY-THEN") {
		t.Errorf("no-else when should render nothing at unknown viewport:\n%s", html)
	}
	if !strings.Contains(html, "CONDLESS-ELSE") || strings.Contains(html, "CONDLESS-THEN") {
		t.Errorf("missing condition must be falsy (render else):\n%s", html)
	}
}
